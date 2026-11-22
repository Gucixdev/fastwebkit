package content

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCatalogRoutes(t *testing.T) {
	dir := t.TempDir()
	writeItem(t, dir, "pi-case.md", "---\ntitle: Pi Case\nprice_cents: 3499\ncurrency: usd\n---\nBody.\n")

	catalog, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	mux := http.NewServeMux()
	catalog.Routes(mux)

	t.Run("list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if !strings.Contains(w.Body.String(), "Pi Case") {
			t.Errorf("list body missing item title: %s", w.Body.String())
		}
	})

	t.Run("detail", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items/pi-case", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if !strings.Contains(w.Body.String(), "Body.") {
			t.Errorf("detail body missing rendered markdown: %s", w.Body.String())
		}
	})

	t.Run("unknown slug 404s", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items/does-not-exist", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})
}

func TestLoadBuildsBySlugIndex(t *testing.T) {
	dir := t.TempDir()
	writeItem(t, dir, "a.md", "A\n")
	writeItem(t, dir, "b.md", "B\n")

	catalog, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(catalog.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(catalog.Items))
	}
	if _, ok := catalog.BySlug["a"]; !ok {
		t.Error("BySlug missing \"a\"")
	}
	if _, ok := catalog.BySlug["b"]; !ok {
		t.Error("BySlug missing \"b\"")
	}
}
