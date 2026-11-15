// ui is the page shell for server-rendered HTML: one function, Page, wraps a body in a document with
// a small embedded stylesheet (light and dark by the browser's setting, no JavaScript, no files to serve).
// Everything the other modules show to a person in a browser goes through it, so the look changes in one place.
// Needs nothing else from this repo.
package kit

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"
)

const uiCSS = `:root{color-scheme:light dark;--bg:#fff;--fg:#1b1f23;--mut:#667;--line:#dde;--acc:#1a7f37}
@media(prefers-color-scheme:dark){:root{--bg:#14171a;--fg:#e6e8ea;--mut:#9aa;--line:#2a2f35;--acc:#3fb950}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--fg);font:16px/1.55 system-ui,sans-serif}
header,main{max-width:46rem;margin:0 auto;padding:1rem}header{border-bottom:1px solid var(--line);display:flex;gap:1rem;align-items:baseline}
header a{color:var(--fg);text-decoration:none;font-weight:600}h1{font-size:1.5rem;margin:.5rem 0 1rem}
ul.cards{list-style:none;padding:0;display:grid;gap:.75rem}ul.cards li{border:1px solid var(--line);border-radius:.5rem;padding:.75rem 1rem;display:flex;justify-content:space-between;gap:1rem;align-items:center}
ul.cards a{color:var(--fg);font-weight:600;text-decoration:none}.price{color:var(--mut)}
button,.btn{background:var(--acc);color:#fff;border:0;border-radius:.4rem;padding:.5rem 1rem;font:inherit;cursor:pointer}
img{max-width:100%;height:auto}`

var uiPage = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}}</title><style>` + uiCSS + `</style></head>
<body><header><a href="/">{{.Site}}</a></header><main>{{.Body}}</main></body></html>`))

// UISite is the name shown top left; set it once at startup.
var UISite = "fastwebapp"

// Page writes a complete HTML document with the given status. title is escaped; body is trusted HTML,
// so build it from escaped values or from html/template (see uiMoney for a number formatter).
func Page(w http.ResponseWriter, status int, title string, body template.HTML) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = uiPage.Execute(w, struct {
		Title, Site string
		Body        template.HTML
	}{title, UISite, body})
}

// WantsHTML is true for a browser (Accept lists text/html) and false for curl or a script, which get JSON.
func WantsHTML(r *http.Request) bool {
	a := r.Header.Get("Accept")
	return strings.Contains(a, "text/html") && !strings.HasPrefix(strings.TrimSpace(a), "application/json")
}

// uiMoney formats minor units: 1999 USD -> "19.99 USD". Negative amounts keep the sign.
func uiMoney(cents int64, currency string) string {
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	return fmt.Sprintf("%s%d.%02d %s", sign, cents/100, cents%100, strings.ToUpper(currency))
}
