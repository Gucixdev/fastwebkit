package fastwebkit

import (
	"bufio"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// allowed is docs/modules.md in code: module -> the modules it may import. ui and database are the base
// modules; nothing else is imported by another module.
var allowed = map[string][]string{
	"ui":        {},
	"database":  {},
	"mongo":     {},
	"content":   {"ui"},
	"auth":      {"database", "ui"},
	"downloads": {"database", "ui"},
	"payments":  {"database"},
}

const modPrefix = "fastwebapp/kit/"

func TestGoModulesImportOnlyTheirDocumentedNeighbours(t *testing.T) {
	entries, err := os.ReadDir("kit")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() {
			t.Errorf("kit/%s: a loose file; every piece of code belongs to a module folder", e.Name())
			continue
		}
		mod := e.Name()
		seen[mod] = true
		ok, known := allowed[mod]
		if !known {
			t.Errorf("kit/%s is not in the module table (docs/modules.md and this test)", mod)
			continue
		}
		files, _ := filepath.Glob(filepath.Join("kit", mod, "*.go"))
		for _, f := range files {
			af, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range af.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				other, isKit := strings.CutPrefix(path, modPrefix)
				if !isKit || other == mod {
					continue
				}
				if !contains(ok, other) {
					t.Errorf("%s imports %s: kit/%s may import only %v", f, path, mod, ok)
				}
			}
		}
	}
	for mod := range allowed {
		if !seen[mod] {
			t.Errorf("module %s is in the table but kit/%s does not exist", mod, mod)
		}
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestEveryModuleAndTemplateHasAShortReadme(t *testing.T) {
	var dirs []string
	for _, pat := range []string{"kit/*", "fastwebapp/web/*", "templates/*"} {
		m, _ := filepath.Glob(pat)
		for _, d := range m {
			if st, err := os.Stat(d); err == nil && st.IsDir() {
				dirs = append(dirs, d)
			}
		}
	}
	if len(dirs) < 19 {
		t.Fatalf("found only %d module/template folders: %v", len(dirs), dirs)
	}
	for _, d := range dirs {
		f, err := os.Open(filepath.Join(d, "README.md"))
		if err != nil {
			t.Errorf("%s has no README.md", d)
			continue
		}
		n := 0
		for sc := bufio.NewScanner(f); sc.Scan(); {
			n++
		}
		f.Close()
		if n == 0 || n > 100 {
			t.Errorf("%s/README.md has %d lines, want 1..100", d, n)
		}
	}
}

var tsImport = regexp.MustCompile(`(?m)^\s*(?:import|export)\b[^;]*?from\s+["']([^"']+)["']`)

func TestTypeScriptModulesDoNotReachIntoEachOther(t *testing.T) {
	files, _ := filepath.Glob("fastwebapp/web/*/*.ts")
	if len(files) < 20 {
		t.Fatalf("found %d ts files", len(files))
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range tsImport.FindAllStringSubmatch(string(b), -1) {
			if strings.HasPrefix(m[1], "..") {
				t.Errorf("%s imports %q: a module may only import files of its own folder (or npm packages)", f, m[1])
			}
		}
	}
}

func TestEveryTemplateHasARunScriptThatTakesNoArguments(t *testing.T) {
	m, _ := filepath.Glob("templates/*/run")
	if len(m) != 5 {
		t.Fatalf("want run scripts for 5 templates, found %v", m)
	}
	for _, f := range m {
		st, _ := os.Stat(f)
		if st.Mode()&0o111 == 0 {
			t.Errorf("%s is not executable", f)
		}
		b, _ := os.ReadFile(f)
		if !strings.Contains(string(b), "$# -eq 0") {
			t.Errorf("%s does not refuse arguments", f)
		}
	}
}
