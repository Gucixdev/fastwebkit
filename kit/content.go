// Package content loads catalog items from plain markdown files: YAML front matter for
// structured fields (price, currency, slug), the rest of the file rendered to HTML as the
// description. Raw HTML in the body (a YouTube <iframe>, an <img>) passes straight through -
// goldmark allows it by default, so embedding media is just writing it into the .md file.

package kit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"gopkg.in/yaml.v3"
)

type Item struct {
	Slug       string // derived from the filename, not front matter - one less thing to keep in sync
	Title      string `yaml:"title"`
	PriceCents int64  `yaml:"price_cents"`
	Currency   string `yaml:"currency"`
	HTML       string // rendered body, safe to drop straight into a template with no escaping
}

// WithUnsafe is required for raw HTML in the body (an <iframe>, an <img>) to pass through -
// without it goldmark drops it as "<!-- raw HTML omitted -->". Item files are authored by
// the shop owner, not visitors, so this is not a stored-XSS surface.
var md = goldmark.New(
	goldmark.WithExtensions(meta.Meta),
	goldmark.WithRendererOptions(html.WithUnsafe()),
)

// LoadDir reads every *.md file in dir as one Item. A file with no front matter still loads -
// Title falls back to the filename, PriceCents stays 0 (treat as "contact for price" upstream).
func LoadDir(dir string) ([]Item, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read items dir %s: %w", dir, err)
	}

	items := make([]Item, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		item, err := loadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("load item %s: %w", e.Name(), err)
		}
		item.Slug = strings.TrimSuffix(e.Name(), ".md")
		items = append(items, item)
	}
	return items, nil
}

func loadFile(path string) (Item, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return Item{}, err
	}

	ctx := parser.NewContext()
	var buf bytes.Buffer
	if err := md.Convert(src, &buf, parser.WithContext(ctx)); err != nil {
		return Item{}, fmt.Errorf("render markdown: %w", err)
	}

	item := Item{HTML: buf.String()}
	if raw := meta.Get(ctx); raw != nil {
		b, err := yaml.Marshal(raw)
		if err != nil {
			return Item{}, fmt.Errorf("re-marshal front matter: %w", err)
		}
		if err := yaml.Unmarshal(b, &item); err != nil {
			return Item{}, fmt.Errorf("decode front matter: %w", err)
		}
	}
	if item.Title == "" {
		item.Title = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	return item, nil
}

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
