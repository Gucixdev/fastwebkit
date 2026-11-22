package content

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func uiCatalog(items ...Item) *http.ServeMux {
	c := &Catalog{Items: items, BySlug: map[string]Item{}}
	for _, it := range items {
		c.BySlug[it.Slug] = it
	}
	mux := http.NewServeMux()
	c.Routes(mux)
	return mux
}

func uiGet(mux *http.ServeMux, path, accept string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", path, nil)
	if accept != "" {
		r.Header.Set("Accept", accept)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func TestCatalogHTMLViews(t *testing.T) {
	mux := uiCatalog(
		Item{Slug: "evil", Title: `<img src=x onerror=alert(1)>`, PriceCents: 1999, Currency: "USD", HTML: "<p>ok</p>"},
		Item{Slug: "free", Title: "Free", HTML: "<p>free</p>"},
	)
	for _, path := range []string{"/", "/items"} {
		w := uiGet(mux, path, "text/html")
		out := w.Body.String()
		if w.Code != 200 || !strings.Contains(out, `href="/items/evil"`) || !strings.Contains(out, "19.99 USD") {
			t.Errorf("%s list wrong: %d %s", path, w.Code, out)
		}
		if strings.Contains(out, "<img src=x") {
			t.Errorf("%s: title not escaped", path)
		}
	}
	d := uiGet(mux, "/items/evil", "text/html").Body.String()
	if !strings.Contains(d, `action="/items/evil/checkout"`) || !strings.Contains(d, "<p>ok</p>") || strings.Contains(d, "<img src=x") {
		t.Errorf("detail wrong: %s", d)
	}
	if f := uiGet(mux, "/items/free", "text/html").Body.String(); strings.Contains(f, "<form") {
		t.Errorf("free item must have no Buy form: %s", f)
	}
	if uiGet(mux, "/items/none", "text/html").Code != 404 {
		t.Errorf("missing item must be 404")
	}
}

func TestCatalogJSONStaysDefault(t *testing.T) {
	mux := uiCatalog(Item{Slug: "a", Title: "A"})
	for _, accept := range []string{"", "application/json", "*/*"} {
		w := uiGet(mux, "/items", accept)
		if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
			t.Errorf("Accept %q: want JSON, got %q", accept, w.Header().Get("Content-Type"))
		}
	}
}
