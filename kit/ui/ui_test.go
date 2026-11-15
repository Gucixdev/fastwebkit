package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUIMoney(t *testing.T) {
	for _, c := range []struct {
		cents int64
		want  string
	}{{0, "0.00 USD"}, {5, "0.05 USD"}, {1999, "19.99 USD"}, {100000, "1000.00 USD"}, {-250, "-2.50 USD"}} {
		if got := Money(c.cents, "usd"); got != c.want {
			t.Errorf("Money(%d) = %q, want %q", c.cents, got, c.want)
		}
	}
}

func TestWantsHTML(t *testing.T) {
	for accept, want := range map[string]bool{
		"": false, "application/json": false, "*/*": false,
		"text/html,application/xhtml+xml,*/*;q=0.8": true,
		"application/json, text/html":               false,
	} {
		r := httptest.NewRequest("GET", "/", nil)
		if accept != "" {
			r.Header.Set("Accept", accept)
		}
		if got := WantsHTML(r); got != want {
			t.Errorf("WantsHTML(%q) = %v, want %v", accept, got, want)
		}
	}
}

func TestPageEscapesTitle(t *testing.T) {
	w := httptest.NewRecorder()
	Page(w, http.StatusTeapot, `<script>alert(1)</script>`, "<p>body</p>")
	out := w.Body.String()
	if w.Code != http.StatusTeapot || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("status/type wrong: %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	if strings.Contains(out, "<script>alert(1)</script>") || !strings.Contains(out, "&lt;script&gt;") {
		t.Errorf("title not escaped: %s", out)
	}
	if !strings.Contains(out, "<p>body</p>") || !strings.Contains(out, "prefers-color-scheme:dark") {
		t.Errorf("body or stylesheet missing")
	}
}
