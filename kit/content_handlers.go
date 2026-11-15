package kit

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strings"
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
	mux.HandleFunc("GET /{$}", c.handleHome)
	mux.HandleFunc("GET /items", c.handleList)
	mux.HandleFunc("GET /items/{slug}", c.handleDetail)
}

var contentListTpl = template.Must(template.New("list").Parse(`<h1>Items</h1><ul class="cards">{{range .}}` +
	`<li><a href="/items/{{.Slug}}">{{.Title}}</a><span class="price">{{.Price}}</span></li>{{else}}<li>Nothing here yet.</li>{{end}}</ul>`))

var contentDetailTpl = template.Must(template.New("detail").Parse(`<h1>{{.Title}}</h1>{{.Body}}` +
	`{{if .Price}}<p class="price">{{.Price}}</p><form method="post" action="/items/{{.Slug}}/checkout"><button>Buy</button></form>{{end}}`))

func (c *Catalog) handleHome(w http.ResponseWriter, r *http.Request) { c.handleList(w, r) }

func (c *Catalog) handleList(w http.ResponseWriter, r *http.Request) {
	if WantsHTML(r) {
		type row struct{ Slug, Title, Price string }
		rows := make([]row, 0, len(c.Items))
		for _, it := range c.Items {
			rows = append(rows, row{it.Slug, it.Title, contentPrice(it)})
		}
		var b strings.Builder
		_ = contentListTpl.Execute(&b, rows)
		Page(w, http.StatusOK, "Items", template.HTML(b.String()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(c.Items)
}

func (c *Catalog) handleDetail(w http.ResponseWriter, r *http.Request) {
	item, ok := c.BySlug[r.PathValue("slug")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	var b strings.Builder
	_ = contentDetailTpl.Execute(&b, struct {
		Slug, Title, Price string
		Body               template.HTML
	}{item.Slug, item.Title, contentPrice(item), template.HTML(item.HTML)})
	Page(w, http.StatusOK, item.Title, template.HTML(b.String()))
}

func contentPrice(it Item) string {
	if it.PriceCents <= 0 {
		return ""
	}
	return uiMoney(it.PriceCents, it.Currency)
}
