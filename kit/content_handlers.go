package kit

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Catalog serves a directory of markdown files as a browsable list + detail pages: pure
// markdown-to-HTML, no payment or checkout logic here - that's app-level wiring that uses a
// Catalog's Items/ByolSlug alongside payments, not something baked into this module.
type Catalog struct {
	Items  []Item
	BySlug map[string]Item
}

func Load(dir string) (*Catalog, error) {
	items, err := LoadDir(dir)
	if err != nil {
		return nil, err
	}
	bySlug := make(map[string]Item, len(items))
	for _, it := range items {
		bySlug[it.Slug] = it
	}
	return &Catalog{Items: items, BySlug: bySlug}, nil
}

// Routes registers read-only listing/detail endpoints. Mount it under whatever prefix the app
// wants; a mutating e-commerce flow (checkout, webhooks) lives in the app, not here.
func (c *Catalog) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /items", c.handleList)
	mux.HandleFunc("GET /items/{slug}", c.handleDetail)
}

func (c *Catalog) handleList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(c.Items)
}

func (c *Catalog) handleDetail(w http.ResponseWriter, r *http.Request) {
	item, ok := c.BySlug[r.PathValue("slug")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<h1>%s</h1>%s", item.Title, item.HTML)
}
