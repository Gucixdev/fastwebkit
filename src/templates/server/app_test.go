package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"fastwebapp/kit/database"
)

func testApp(t *testing.T, maxUpload int64) http.Handler {
	t.Helper()
	db := database.OpenTest(t, "users", "downloads")
	h, err := newApp(db, []byte("test-secret"), filepath.Join(t.TempDir(), "files"), maxUpload)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func do(h http.Handler, method, path, ctype string, body io.Reader, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, body)
	if ctype != "" {
		r.Header.Set("Content-Type", ctype)
	}
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func register(t *testing.T, h http.Handler, email string) *http.Cookie {
	t.Helper()
	w := do(h, "POST", "/register", "application/json", strings.NewReader(`{"email":"`+email+`","password":"hunter22"}`))
	if w.Code != 200 {
		t.Fatalf("register: %d %s", w.Code, w.Body)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "session" {
			return c
		}
	}
	t.Fatal("no session cookie")
	return nil
}

func upload(t *testing.T, name, content string) (*bytes.Buffer, string) {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	fw, _ := mw.CreateFormFile("file", name)
	fw.Write([]byte(content))
	mw.Close()
	return &b, mw.FormDataContentType()
}

func TestHealthz(t *testing.T) {
	w := do(testApp(t, 1<<20), "GET", "/healthz", "", nil)
	if w.Code != 200 || w.Body.String() != "ok" {
		t.Fatalf("healthz: %d %q", w.Code, w.Body)
	}
}

func TestMeNeedsSession(t *testing.T) {
	h := testApp(t, 1<<20)
	if w := do(h, "GET", "/me", "", nil); w.Code != 401 {
		t.Fatalf("anonymous /me = %d, want 401", w.Code)
	}
	c := register(t, h, "a@example.com")
	w := do(h, "GET", "/me", "", nil, c)
	var out map[string]string
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out["id"] == "" {
		t.Fatalf("/me with session = %d %s", w.Code, w.Body)
	}
}

func TestUploadNeedsSessionThenServesFile(t *testing.T) {
	h := testApp(t, 1<<20)
	body, ct := upload(t, "hello.txt", "hello world")
	if w := do(h, "POST", "/downloads", ct, body); w.Code != 401 {
		t.Fatalf("anonymous upload = %d, want 401", w.Code)
	}

	c := register(t, h, "b@example.com")
	body, ct = upload(t, "hello.txt", "hello world")
	w := do(h, "POST", "/downloads", ct, body, c)
	if w.Code != 200 {
		t.Fatalf("upload = %d %s", w.Code, w.Body)
	}
	var l struct{ ID, FileURL, TorrentURL, Magnet string }
	if err := json.Unmarshal(w.Body.Bytes(), &l); err != nil || l.FileURL == "" || !strings.HasPrefix(l.Magnet, "magnet:") {
		t.Fatalf("listing = %s (%v)", w.Body, err)
	}

	// reading is public
	if w := do(h, "GET", "/downloads", "", nil); w.Code != 200 || !strings.Contains(w.Body.String(), "hello.txt") {
		t.Fatalf("list = %d %s", w.Code, w.Body)
	}
	if w := do(h, "GET", l.FileURL, "", nil); w.Code != 200 || w.Body.String() != "hello world" {
		t.Fatalf("file = %d %q", w.Code, w.Body)
	}
	if w := do(h, "GET", l.TorrentURL, "", nil); w.Code != 200 || !strings.HasPrefix(w.Body.String(), "d") {
		t.Fatalf("torrent = %d", w.Code)
	}
}

func TestUploadOverCapIsRejected(t *testing.T) {
	h := testApp(t, 1024)
	c := register(t, h, "c@example.com")
	body, ct := upload(t, "big.bin", strings.Repeat("x", 4096))
	w := do(h, "POST", "/downloads", ct, body, c)
	if w.Code < 400 {
		t.Fatalf("oversize upload = %d, want an error", w.Code)
	}
	if l := do(h, "GET", "/downloads", "", nil); strings.Contains(l.Body.String(), "big.bin") {
		t.Fatalf("oversize file was stored: %s", l.Body)
	}
}

func TestPanicBecomes500(t *testing.T) {
	h := recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	if w := do(h, "GET", "/", "", nil); w.Code != 500 {
		t.Fatalf("panic = %d, want 500", w.Code)
	}
}
