package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fastwebapp/kit/ui"
)

func testApp(t *testing.T) http.Handler {
	t.Helper()
	dir := t.TempDir()
	write := func(name, s string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("welcome.md", "---\ntitle: Welcome\n---\n\nHello **world**.\n")
	write("evil.md", "---\ntitle: <script>alert(1)</script>\n---\n\nbody\n")
	h, err := newApp(dir, "https://example.test/")
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func get(h http.Handler, path, accept string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", path, nil)
	if accept != "" {
		r.Header.Set("Accept", accept)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestBrowserGetsHTMLPages(t *testing.T) {
	ui.Site = "My Site"
	h := testApp(t)
	for _, path := range []string{"/", "/items"} {
		w := get(h, path, "text/html")
		if w.Code != 200 || !strings.Contains(w.Body.String(), `href="/items/welcome"`) || !strings.Contains(w.Body.String(), "My Site") {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
	}
	w := get(h, "/items/welcome", "text/html")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "<strong>world</strong>") {
		t.Fatalf("detail: %d %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "<form") {
		t.Fatalf("a page without a price must have no Buy form")
	}
}

func TestScriptGetsJSON(t *testing.T) {
	w := get(testApp(t), "/items", "application/json")
	var items []struct{ Slug, Title string }
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &items) != nil || len(items) != 2 {
		t.Fatalf("json: %d %s", w.Code, w.Body)
	}
}

func TestTitleIsEscapedAndUnknownIs404(t *testing.T) {
	h := testApp(t)
	if b := get(h, "/items", "text/html").Body.String(); strings.Contains(b, "<script>alert(1)") {
		t.Fatalf("title not escaped: %s", b)
	}
	if w := get(h, "/items/nope", "text/html"); w.Code != 404 {
		t.Fatalf("unknown page = %d", w.Code)
	}
}

func TestSitemapRobotsHealth(t *testing.T) {
	h := testApp(t)
	sm := get(h, "/sitemap.xml", "").Body.String()
	for _, want := range []string{"<loc>https://example.test/</loc>", "<loc>https://example.test/items/welcome</loc>"} {
		if !strings.Contains(sm, want) {
			t.Errorf("sitemap lacks %s: %s", want, sm)
		}
	}
	if r := get(h, "/robots.txt", "").Body.String(); !strings.Contains(r, "Sitemap: https://example.test/sitemap.xml") {
		t.Errorf("robots: %q", r)
	}
	if w := get(h, "/healthz", ""); w.Code != 200 {
		t.Errorf("healthz = %d", w.Code)
	}
}

func TestMissingPagesDirFailsToStart(t *testing.T) {
	if _, err := newApp(filepath.Join(t.TempDir(), "nope"), ""); err == nil {
		t.Fatal("want an error for a missing pages dir")
	}
}
