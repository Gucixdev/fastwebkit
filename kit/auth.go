// auth is the login/register module: users table, bcrypt passwords, signed-cookie
// sessions. A standalone module - it only needs a *sql.DB (from database), nothing shop- or
// app-specific leaks in here.

package kit

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
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
func AuthNew(db *sql.DB, secret []byte) (*Auth, error) {
	if err := Migrate(db, authSchema); err != nil {
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
		Rebind(a.db, `INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, ?, ?)`),
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
		Rebind(a.db, `SELECT id, email, password_hash, created_at FROM users WHERE email = ?`), email,
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
func (a *Auth) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /register", a.handleRegister)
	mux.HandleFunc("POST /login", a.handleLogin)
	mux.HandleFunc("POST /logout", a.handleLogout)
}

func (a *Auth) handleRegister(w http.ResponseWriter, r *http.Request) {
	var creds credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	user, err := a.Register(creds.Email, creds.Password)
	if errors.Is(err, ErrEmailTaken) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	a.setSessionCookie(w, user.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

func (a *Auth) handleLogin(w http.ResponseWriter, r *http.Request) {
	var creds credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	user, err := a.Login(creds.Email, creds.Password)
	if errors.Is(err, ErrInvalidLogin) {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	a.setSessionCookie(w, user.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

func (a *Auth) handleLogout(w http.ResponseWriter, r *http.Request) {
	a.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}
