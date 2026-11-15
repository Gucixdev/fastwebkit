// Package content loads catalog items from plain markdown files: YAML front matter for
// structured fields (price, currency, slug), the rest of the file rendered to HTML as the
// description. Raw HTML in the body (a YouTube <iframe>, an <img>) passes straight through -
// goldmark allows it by default, so embedding media is just writing it into the .md file.

package kit

import (
	"bytes"
	"fmt"
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
