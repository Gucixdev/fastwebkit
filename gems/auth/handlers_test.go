package auth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlersRegisterLoginLogoutFlow(t *testing.T) {
	a := newTestAuth(t)
	mux := http.NewServeMux()
	a.Routes(mux)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}

	t.Run("register sets a session cookie", func(t *testing.T) {
		w := do(http.MethodPost, "/register", `{"email":"eve@example.com","password":"hunter22"}`)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
		}
		resp := http.Response{Header: w.Header()}
		if len(resp.Cookies()) != 1 {
			t.Fatalf("expected a session cookie, got %v", resp.Cookies())
		}
	})

	t.Run("duplicate register is a 409", func(t *testing.T) {
		w := do(http.MethodPost, "/register", `{"email":"eve@example.com","password":"other"}`)
		if w.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409", w.Code)
		}
	})

	t.Run("login with correct credentials succeeds", func(t *testing.T) {
		w := do(http.MethodPost, "/login", `{"email":"eve@example.com","password":"hunter22"}`)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
		}
	})

	t.Run("login with wrong password is a 401", func(t *testing.T) {
		w := do(http.MethodPost, "/login", `{"email":"eve@example.com","password":"wrong"}`)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("logout clears the cookie with a 204", func(t *testing.T) {
		w := do(http.MethodPost, "/logout", "")
		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", w.Code)
		}
		resp := http.Response{Header: w.Header()}
		cookies := resp.Cookies()
		if len(cookies) != 1 || cookies[0].MaxAge >= 0 {
			t.Fatalf("expected a clearing cookie, got %v", cookies)
		}
	})

	t.Run("malformed JSON is a 400", func(t *testing.T) {
		w := do(http.MethodPost, "/register", `not json`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})
}
