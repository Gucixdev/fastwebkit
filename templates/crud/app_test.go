package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"fastwebapp/kit/database"
)

func testApp(t *testing.T) http.Handler {
	t.Helper()
	db := database.OpenTest(t, "users", "notes")
	h, err := newApp(db, []byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func do(h http.Handler, method, path, ctype, accept string, body string, c *http.Cookie) *httptest.ResponseRecorder {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	r := httptest.NewRequest(method, path, rd)
	if ctype != "" {
		r.Header.Set("Content-Type", ctype)
	}
	if accept != "" {
		r.Header.Set("Accept", accept)
	}
	if c != nil {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func signUp(t *testing.T, h http.Handler, email string) *http.Cookie {
	t.Helper()
	w := do(h, "POST", "/register", "application/json", "", `{"email":"`+email+`","password":"hunter22"}`, nil)
	for _, c := range w.Result().Cookies() {
		if c.Name == "session" && w.Code == 200 {
			return c
		}
	}
	t.Fatalf("register %s: %d %s", email, w.Code, w.Body)
	return nil
}

func createNote(t *testing.T, h http.Handler, c *http.Cookie, title string) note {
	t.Helper()
	w := do(h, "POST", "/notes", "application/json", "", `{"title":`+jsonStr(title)+`,"body":"b"}`, c)
	var n note
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &n) != nil || n.ID == "" {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	return n
}

func jsonStr(s string) string { b, _ := json.Marshal(s); return string(b) }

func TestJSONLifecycle(t *testing.T) {
	h := testApp(t)
	c := signUp(t, h, "a@example.com")
	n := createNote(t, h, c, "first")

	w := do(h, "GET", "/notes/"+n.ID, "", "", "", c)
	var got note
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || got.Title != "first" || got.Body != "b" {
		t.Fatalf("get: %d %s", w.Code, w.Body)
	}

	w = do(h, "PUT", "/notes/"+n.ID, "application/json", "", `{"title":"renamed","body":"new"}`, c)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"renamed"`) {
		t.Fatalf("update: %d %s", w.Code, w.Body)
	}

	w = do(h, "GET", "/notes", "", "", "", c)
	var list []note
	if json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list) != 1 || list[0].Title != "renamed" {
		t.Fatalf("list: %s", w.Body)
	}

	if w = do(h, "DELETE", "/notes/"+n.ID, "", "", "", c); w.Code != 204 {
		t.Fatalf("delete: %d", w.Code)
	}
	if w = do(h, "GET", "/notes/"+n.ID, "", "", "", c); w.Code != 404 {
		t.Fatalf("get after delete = %d, want 404", w.Code)
	}
	if w = do(h, "DELETE", "/notes/"+n.ID, "", "", "", c); w.Code != 404 {
		t.Fatalf("second delete = %d, want 404", w.Code)
	}
}

func TestOwnerIsolation(t *testing.T) {
	h := testApp(t)
	alice, bob := signUp(t, h, "alice@example.com"), signUp(t, h, "bob@example.com")
	n := createNote(t, h, alice, "secret")

	if w := do(h, "GET", "/notes/"+n.ID, "", "", "", bob); w.Code != 404 {
		t.Errorf("bob reads alice's note: %d", w.Code)
	}
	if w := do(h, "PUT", "/notes/"+n.ID, "application/json", "", `{"title":"hacked"}`, bob); w.Code != 404 {
		t.Errorf("bob updates alice's note: %d", w.Code)
	}
	if w := do(h, "DELETE", "/notes/"+n.ID, "", "", "", bob); w.Code != 404 {
		t.Errorf("bob deletes alice's note: %d", w.Code)
	}
	if w := do(h, "GET", "/notes", "", "", "", bob); strings.TrimSpace(w.Body.String()) != "[]" {
		t.Errorf("bob's list should be empty: %s", w.Body)
	}
	if w := do(h, "GET", "/notes/"+n.ID, "", "", "", alice); !strings.Contains(w.Body.String(), "secret") {
		t.Errorf("alice lost her note: %s", w.Body)
	}
}

func TestAnonymous(t *testing.T) {
	h := testApp(t)
	if w := do(h, "GET", "/notes", "", "", "", nil); w.Code != 401 {
		t.Errorf("script without session = %d, want 401", w.Code)
	}
	w := do(h, "GET", "/notes", "", "text/html", "", nil)
	if w.Code != 303 || w.Header().Get("Location") != "/login?next="+url.QueryEscape("/notes") {
		t.Errorf("browser without session = %d -> %q", w.Code, w.Header().Get("Location"))
	}
	if w := do(h, "POST", "/notes", "application/json", "", `{"title":"x"}`, nil); w.Code != 401 {
		t.Errorf("anonymous create = %d, want 401", w.Code)
	}
}

func TestValidation(t *testing.T) {
	h := testApp(t)
	c := signUp(t, h, "v@example.com")
	for name, body := range map[string]string{
		"empty title":   `{"title":"  ","body":"x"}`,
		"title too big": `{"title":"` + strings.Repeat("a", 201) + `"}`,
		"not json":      `nope`,
	} {
		if w := do(h, "POST", "/notes", "application/json", "", body, c); w.Code != 400 {
			t.Errorf("%s: %d, want 400", name, w.Code)
		}
	}
}

func TestHTMLFormFlowEscapes(t *testing.T) {
	h := testApp(t)
	c := signUp(t, h, "h@example.com")
	form := url.Values{"title": {`<b>bold</b>`}, "body": {"x"}}.Encode()
	w := do(h, "POST", "/notes", "application/x-www-form-urlencoded", "text/html", form, c)
	if w.Code != 303 || w.Header().Get("Location") != "/notes" {
		t.Fatalf("form create: %d %q", w.Code, w.Header().Get("Location"))
	}
	page := do(h, "GET", "/notes", "", "text/html", "", c).Body.String()
	if strings.Contains(page, "<b>bold</b>") || !strings.Contains(page, "&lt;b&gt;bold&lt;/b&gt;") {
		t.Fatalf("title not escaped in page: %s", page)
	}
	var id string
	var list []note
	json.Unmarshal(do(h, "GET", "/notes", "", "", "", c).Body.Bytes(), &list)
	id = list[0].ID
	if w := do(h, "POST", "/notes/"+id+"/delete", "application/x-www-form-urlencoded", "text/html", "", c); w.Code != 303 {
		t.Fatalf("form delete: %d", w.Code)
	}
	if page := do(h, "GET", "/notes", "", "text/html", "", c).Body.String(); !strings.Contains(page, "No notes yet.") {
		t.Fatalf("note still listed: %s", page)
	}
}

func TestRootRedirectsToNotes(t *testing.T) {
	if w := do(testApp(t), "GET", "/", "", "", "", nil); w.Code != 303 || w.Header().Get("Location") != "/notes" {
		t.Fatalf("/ = %d %q", w.Code, w.Header().Get("Location"))
	}
}
