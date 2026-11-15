package kit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

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
