package auth

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSignedCookieRoundTrip(t *testing.T) {
	cookie := signedCookie{secret: []byte("secret")}
	encoded := cookie.encode("user-123")

	userID, ok := cookie.decode(encoded)
	if !ok {
		t.Fatal("decode() failed on a value it just encoded")
	}
	if userID != "user-123" {
		t.Errorf("userID = %q, want %q", userID, "user-123")
	}
}

func TestSignedCookieRejectsTamperedPayload(t *testing.T) {
	cookie := signedCookie{secret: []byte("secret")}
	encoded := cookie.encode("user-123")

	// Flip the encoded payload but keep the original signature - must be rejected, not
	// decoded into a different user id.
	parts := strings.SplitN(encoded, ".", 2)
	tampered := "XXXX" + parts[0][4:] + "." + parts[1]

	if _, ok := cookie.decode(tampered); ok {
		t.Fatal("decode() accepted a tampered payload")
	}
}

func TestSignedCookieRejectsWrongSecret(t *testing.T) {
	encoded := signedCookie{secret: []byte("secret-a")}.encode("user-123")
	if _, ok := (signedCookie{secret: []byte("secret-b")}).decode(encoded); ok {
		t.Fatal("decode() accepted a cookie signed with a different secret")
	}
}

func TestSignedCookieRejectsExpired(t *testing.T) {
	cookie := signedCookie{secret: []byte("secret")}
	// Build an already-expired payload directly, rather than waiting 30 days.
	expired := time.Now().Add(-time.Hour).Unix()
	payload := "user-123|" + strconv.FormatInt(expired, 10)
	encoded := base64.RawURLEncoding.EncodeToString([]byte(payload))
	value := encoded + "." + cookie.sign(encoded)

	if _, ok := cookie.decode(value); ok {
		t.Fatal("decode() accepted an expired session")
	}
}

func TestRequireUserRejectsMissingCookie(t *testing.T) {
	a := &Auth{cookie: signedCookie{secret: []byte("secret")}}
	called := false
	handler := a.RequireUser(func(w http.ResponseWriter, r *http.Request) { called = true })

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if called {
		t.Fatal("handler ran without a valid session")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestRequireUserAllowsValidSession(t *testing.T) {
	a := &Auth{cookie: signedCookie{secret: []byte("secret")}}
	var seenUserID string
	handler := a.RequireUser(func(w http.ResponseWriter, r *http.Request) {
		seenUserID, _ = a.UserIDFromRequest(r)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: a.cookie.encode("user-123")})
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (default)", w.Code)
	}
	if seenUserID != "user-123" {
		t.Errorf("seenUserID = %q, want %q", seenUserID, "user-123")
	}
}

func TestSetAndClearSessionCookie(t *testing.T) {
	a := &Auth{cookie: signedCookie{secret: []byte("secret")}}

	w := httptest.NewRecorder()
	a.setSessionCookie(w, "user-123")
	resp := http.Response{Header: w.Header()}
	cookies := resp.Cookies()
	if len(cookies) != 1 || cookies[0].Name != cookieName {
		t.Fatalf("expected one %q cookie, got %v", cookieName, cookies)
	}
	if userID, ok := a.cookie.decode(cookies[0].Value); !ok || userID != "user-123" {
		t.Errorf("set cookie decodes to (%q, %v), want (user-123, true)", userID, ok)
	}

	w2 := httptest.NewRecorder()
	a.clearSessionCookie(w2)
	resp2 := http.Response{Header: w2.Header()}
	cleared := resp2.Cookies()
	if len(cleared) != 1 || cleared[0].MaxAge >= 0 {
		t.Fatalf("expected a cookie with a negative MaxAge, got %v", cleared)
	}
}
