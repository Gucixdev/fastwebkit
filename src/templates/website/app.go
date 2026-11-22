package main

import (
	"encoding/xml"
	"net/http"
	"strings"

	"fastwebapp/kit/content"
)

// newApp composes one module: content (Markdown folder -> list and detail pages, wrapped by ui).
// Pages with a price would show a Buy form, so this template is for pages without prices; the saas
// template wires payments to the same catalog.
func newApp(dir, baseURL string) (http.Handler, error) {
	cat, err := content.Load(dir)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	cat.Routes(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("User-agent: *\nAllow: /\nSitemap: " + strings.TrimRight(baseURL, "/") + "/sitemap.xml\n"))
	})
	mux.HandleFunc("GET /sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		type url struct {
			Loc string `xml:"loc"`
		}
		type urlset struct {
			XMLName xml.Name `xml:"urlset"`
			Xmlns   string   `xml:"xmlns,attr"`
			URLs    []url    `xml:"url"`
		}
		base := strings.TrimRight(baseURL, "/")
		out := urlset{Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9", URLs: []url{{base + "/"}}}
		for _, it := range cat.Items {
			out.URLs = append(out.URLs, url{base + "/items/" + it.Slug})
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Write([]byte(xml.Header))
		xml.NewEncoder(w).Encode(out)
	})
	return mux, nil
}
