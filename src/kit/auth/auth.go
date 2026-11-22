// auth is the login/register module: users table, bcrypt passwords, signed-cookie
// sessions. A standalone module - it only needs a *sql.DB (from database), nothing shop- or
// app-specific leaks in here.

package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"fastwebapp/kit/database"
	"fastwebapp/kit/ui"
)

var (
	ErrEmailTaken   = errors.New("auth: email already registered")
	ErrInvalidLogin = errors.New("auth: invalid email or password")
)

type User struct {
	ID        string
	Email     string
	CreatedAt time.Time
}

type Auth struct {
	db     *sql.DB
	cookie signedCookie
}

const authSchema = `CREATE TABLE IF NOT EXISTS users (
	id TEXT PRIMARY KEY,
	email TEXT UNIQUE NOT NULL,
	password_hash TEXT NOT NULL,
	created_at TEXT NOT NULL
)`

// New runs the users table migration and returns an Auth bound to db. Works the same against
// SQLite or PostgreSQL - the schema avoids anything dialect-specific (no SERIAL/AUTOINCREMENT,
// no native TIMESTAMP type). secret signs session cookies - any change to it invalidates every
// existing session.
func New(db *sql.DB, secret []byte) (*Auth, error) {
	if err := database.Migrate(db, authSchema); err != nil {
		return nil, err
	}
	return &Auth{db: db, cookie: signedCookie{secret: secret}}, nil
}

func (a *Auth) Register(email, password string) (User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, fmt.Errorf("auth: hash password: %w", err)
	}

	u := User{ID: uuid.NewString(), Email: email, CreatedAt: time.Now()}
	_, err = a.db.Exec(
		database.Rebind(a.db, `INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, ?, ?)`),
		u.ID, u.Email, string(hash), u.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		// sqlite and pgx report a unique-violation with different driver error types, but both
		// surface the constraint name in the error text - good enough without parsing driver-
		// specific error codes for a single-column UNIQUE check.
		if isUniqueViolation(err) {
			return User{}, ErrEmailTaken
		}
		return User{}, fmt.Errorf("auth: insert user: %w", err)
	}
	return u, nil
}

func (a *Auth) Login(email, password string) (User, error) {
	var u User
	var hash, createdAt string
	err := a.db.QueryRow(
		database.Rebind(a.db, `SELECT id, email, password_hash, created_at FROM users WHERE email = ?`), email,
	).Scan(&u.ID, &u.Email, &hash, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrInvalidLogin
	}
	if err != nil {
		return User{}, fmt.Errorf("auth: query user: %w", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return User{}, ErrInvalidLogin
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return u, nil
}

// ErrNoUser is returned by UserByID for an id that has no account.
var ErrNoUser = errors.New("auth: no such user")

// UserByID looks an account up by the id that UserIDFromRequest returns, for modules that show who did something.
func (a *Auth) UserByID(id string) (User, error) {
	var u User
	var createdAt string
	err := a.db.QueryRow(database.Rebind(a.db, `SELECT id, email, created_at FROM users WHERE id = ?`), id).Scan(&u.ID, &u.Email, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNoUser
	}
	if err != nil {
		return User{}, fmt.Errorf("auth: query user: %w", err)
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return u, nil
}

func isUniqueViolation(err error) bool {
	msg := err.Error()
	return contains(msg, "UNIQUE constraint") || contains(msg, "unique constraint") || contains(msg, "duplicate key") || contains(msg, "Duplicate entry")
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

const cookieName = "session"
const sessionTTL = 30 * 24 * time.Hour

// signedCookie is a self-contained session: userID + expiry, HMAC-signed so it can't be forged
// or extended client-side. No server-side session table - nothing to clean up, nothing to hit
// on every request.
type signedCookie struct {
	secret []byte
}

func (s signedCookie) encode(userID string) string {
	payload := userID + "|" + strconv.FormatInt(time.Now().Add(sessionTTL).Unix(), 10)
	encoded := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return encoded + "." + s.sign(encoded)
}

func (s signedCookie) decode(value string) (string, bool) {
	encoded, sig, ok := strings.Cut(value, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(s.sign(encoded))) {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", false
	}
	userID, expiryStr, ok := strings.Cut(string(raw), "|")
	if !ok {
		return "", false
	}
	expiry, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil || time.Now().Unix() > expiry {
		return "", false
	}
	return userID, true
}

func (s signedCookie) sign(encoded string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(encoded))
	return hex.EncodeToString(mac.Sum(nil))
}

func (a *Auth) setSessionCookie(w http.ResponseWriter, userID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    a.cookie.encode(userID),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

func (a *Auth) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Path: "/", MaxAge: -1})
}

// UserIDFromRequest reads and verifies the session cookie, returning the signed-in user's id.
func (a *Auth) UserIDFromRequest(r *http.Request) (string, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return "", false
	}
	return a.cookie.decode(c.Value)
}

// RequireUser wraps a handler so it 401s without a valid session, otherwise runs next with the
// user id reachable via UserIDFromRequest inside it.
func (a *Auth) RequireUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := a.UserIDFromRequest(r); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Routes registers /register, /login, /logout. Mount it under whatever prefix the app wants.
// A POST with Content-Type application/json answers JSON; a POST from an HTML form
// (application/x-www-form-urlencoded) sets the cookie and redirects to the form's "next" (a path on this
// site, default "/"), or shows the form again with the error. GET /login and GET /register serve the forms.
func (a *Auth) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /register", func(w http.ResponseWriter, r *http.Request) { authForm(w, r, "register", "", http.StatusOK) })
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) { authForm(w, r, "login", "", http.StatusOK) })
	mux.HandleFunc("POST /register", a.handleRegister)
	mux.HandleFunc("POST /login", a.handleLogin)
	mux.HandleFunc("POST /logout", a.handleLogout)
}

var authFormTmpl = template.Must(template.New("form").Parse(`<h1>{{.Title}}</h1>
{{if .Err}}<p role="alert" style="color:#c33">{{.Err}}</p>{{end}}
<form method="post" action="/{{.Kind}}">
<input type="hidden" name="next" value="{{.Next}}">
<p><label>Email<br><input type="email" name="email" required autofocus autocomplete="username"></label></p>
<p><label>Password<br><input type="password" name="password" required autocomplete="{{.Autocomplete}}"></label></p>
<p><button type="submit">{{.Title}}</button></p>
</form>
<p>{{if eq .Kind "login"}}No account? <a href="/register">Register</a>{{else}}Have an account? <a href="/login">Log in</a>{{end}}</p>`))

// authForm renders the login or register page; next survives the round trip only if it is a local path.
func authForm(w http.ResponseWriter, r *http.Request, kind, errMsg string, status int) {
	title, auto := "Log in", "current-password"
	if kind == "register" {
		title, auto = "Register", "new-password"
	}
	var b strings.Builder
	_ = authFormTmpl.Execute(&b, struct{ Title, Kind, Err, Next, Autocomplete string }{title, kind, errMsg, safeNext(r.FormValue("next")), auto})
	ui.Page(w, status, title, template.HTML(b.String()))
}

// safeNext keeps a redirect target on this site: one leading slash, no "//" or backslash tricks, no scheme.
func safeNext(next string) string {
	if next == "" || next[0] != '/' || strings.HasPrefix(next, "//") || strings.ContainsAny(next, "\\\r\n") {
		return "/"
	}
	return next
}

func isFormPost(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded")
}

// readCreds takes the credentials from a form or from JSON.
func readCreds(r *http.Request) (credentials, error) {
	var creds credentials
	if isFormPost(r) {
		if err := r.ParseForm(); err != nil {
			return creds, err
		}
		return credentials{Email: r.PostFormValue("email"), Password: r.PostFormValue("password")}, nil
	}
	err := json.NewDecoder(r.Body).Decode(&creds)
	return creds, err
}

// authFail answers an error as plain text for JSON clients and as the form again for browsers.
func authFail(w http.ResponseWriter, r *http.Request, kind, msg string, status int) {
	if isFormPost(r) {
		authForm(w, r, kind, msg, status)
		return
	}
	http.Error(w, msg, status)
}

// authOK sets the cookie and answers: a redirect for forms, the user as JSON otherwise.
func (a *Auth) authOK(w http.ResponseWriter, r *http.Request, user User) {
	a.setSessionCookie(w, user.ID)
	if isFormPost(r) {
		http.Redirect(w, r, safeNext(r.PostFormValue("next")), http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

func (a *Auth) handleRegister(w http.ResponseWriter, r *http.Request) {
	creds, err := readCreds(r)
	if err != nil {
		authFail(w, r, "register", "bad request", http.StatusBadRequest)
		return
	}
	user, err := a.Register(creds.Email, creds.Password)
	if errors.Is(err, ErrEmailTaken) {
		authFail(w, r, "register", err.Error(), http.StatusConflict)
		return
	}
	if err != nil {
		authFail(w, r, "register", err.Error(), http.StatusInternalServerError)
		return
	}
	a.authOK(w, r, user)
}

func (a *Auth) handleLogin(w http.ResponseWriter, r *http.Request) {
	creds, err := readCreds(r)
	if err != nil {
		authFail(w, r, "login", "bad request", http.StatusBadRequest)
		return
	}
	user, err := a.Login(creds.Email, creds.Password)
	if errors.Is(err, ErrInvalidLogin) {
		authFail(w, r, "login", err.Error(), http.StatusUnauthorized)
		return
	}
	if err != nil {
		authFail(w, r, "login", err.Error(), http.StatusInternalServerError)
		return
	}
	a.authOK(w, r, user)
}

func (a *Auth) handleLogout(w http.ResponseWriter, r *http.Request) {
	a.clearSessionCookie(w)
	if isFormPost(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
