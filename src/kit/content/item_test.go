package content

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeItem(t *testing.T, dir, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", filename, err)
	}
}

func TestLoadDirParsesFrontMatterAndSlug(t *testing.T) {
	dir := t.TempDir()
	writeItem(t, dir, "pi-case.md", "---\ntitle: Pi Case\nprice_cents: 3499\ncurrency: usd\n---\nA nice case.\n")

	items, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	item := items[0]
	if item.Slug != "pi-case" {
		t.Errorf("Slug = %q, want %q", item.Slug, "pi-case")
	}
	if item.Title != "Pi Case" {
		t.Errorf("Title = %q, want %q", item.Title, "Pi Case")
	}
	if item.PriceCents != 3499 {
		t.Errorf("PriceCents = %d, want 3499", item.PriceCents)
	}
	if item.Currency != "usd" {
		t.Errorf("Currency = %q, want %q", item.Currency, "usd")
	}
	if !strings.Contains(item.HTML, "A nice case.") {
		t.Errorf("HTML missing body text: %q", item.HTML)
	}
}

func TestLoadDirFallsBackToFilenameTitle(t *testing.T) {
	dir := t.TempDir()
	writeItem(t, dir, "no-frontmatter.md", "Just a paragraph, no front matter at all.\n")

	items, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if items[0].Title != "no-frontmatter" {
		t.Errorf("Title = %q, want filename fallback %q", items[0].Title, "no-frontmatter")
	}
	if items[0].PriceCents != 0 {
		t.Errorf("PriceCents = %d, want 0 for a file with no price", items[0].PriceCents)
	}
}

func TestLoadDirPassesThroughRawHTML(t *testing.T) {
	dir := t.TempDir()
	writeItem(t, dir, "media.md", "---\ntitle: Media\n---\n<img src=\"/x.jpg\">\n<iframe src=\"https://youtube.com/embed/x\"></iframe>\n")

	items, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	// This is the bug that was only caught by actually running the server and curling it:
	// without html.WithUnsafe(), goldmark drops this as "<!-- raw HTML omitted -->".
	if !strings.Contains(items[0].HTML, `<img src="/x.jpg">`) {
		t.Errorf("raw <img> was stripped: %q", items[0].HTML)
	}
	if !strings.Contains(items[0].HTML, "<iframe") {
		t.Errorf("raw <iframe> was stripped: %q", items[0].HTML)
	}
	if strings.Contains(items[0].HTML, "raw HTML omitted") {
		t.Errorf("goldmark dropped raw HTML: %q", items[0].HTML)
	}
}

func TestLoadDirIgnoresNonMarkdownFiles(t *testing.T) {
	dir := t.TempDir()
	writeItem(t, dir, "item.md", "hello\n")
	writeItem(t, dir, "notes.txt", "not an item\n")

	items, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1 (notes.txt should be ignored)", len(items))
	}
}

func TestLoadDirErrorsOnMissingDir(t *testing.T) {
	_, err := LoadDir(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}
