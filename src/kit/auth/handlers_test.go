package auth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestHTMLFormsRegisterLoginAndRedirect(t *testing.T) {
	a := newTestAuth(t)
	mux := http.NewServeMux()
	a.Routes(mux)
	post := func(path, form string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(form))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}

	for _, p := range []string{"/login", "/register"} {
		w := get(p)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `<form method="post" action="`+p+`">`) {
			t.Fatalf("GET %s: %d %s", p, w.Code, w.Body.String())
		}
	}
	if !strings.Contains(get("/login?next=/shop").Body.String(), `name="next" value="/shop"`) {
		t.Fatal("next not carried into the form")
	}
	if !strings.Contains(get("/login?next=//evil.example").Body.String(), `name="next" value="/"`) {
		t.Fatal("protocol-relative next must fall back to /")
	}

	w := post("/register", "email=form%40example.com&password=hunter22&next=%2Fshop")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/shop" {
		t.Fatalf("register: %d -> %q", w.Code, w.Header().Get("Location"))
	}
	if len((&http.Response{Header: w.Header()}).Cookies()) != 1 {
		t.Fatal("register by form must set the session cookie")
	}
	w = post("/register", "email=form%40example.com&password=again")
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "already registered") || w.Header().Get("Location") != "" {
		t.Fatalf("duplicate: %d %s", w.Code, w.Body.String())
	}
	w = post("/login", "email=form%40example.com&password=wrong")
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "invalid email or password") {
		t.Fatalf("bad login: %d %s", w.Code, w.Body.String())
	}
	w = post("/login", "email=form%40example.com&password=hunter22&next=https%3A%2F%2Fevil.example")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" {
		t.Fatalf("login with absolute next must go to /: %d -> %q", w.Code, w.Header().Get("Location"))
	}
	w = post("/logout", "")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" {
		t.Fatalf("logout: %d", w.Code)
	}
}

func TestSafeNext(t *testing.T) {
	for in, want := range map[string]string{"": "/", "/a/b?c=1": "/a/b?c=1", "//x.example": "/", "/\\x.example": "/", "https://x.example": "/", "x": "/", "/a\r\nSet-Cookie: x": "/"} {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
}
