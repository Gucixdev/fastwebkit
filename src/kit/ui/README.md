# ui

The page shell for server-rendered HTML. One function, `Page`, wraps a body in a complete document with a small
embedded stylesheet (light and dark by the browser's setting, no JavaScript, no files to serve). Every module that
shows something to a person in a browser goes through it, so the look changes in one place.

Depends on: nothing. Used by: `content`, `auth`, `downloads`, and the templates.

## Interface

    ui.Site                                   // name shown top left; set once at startup
    ui.Page(w, status, title, body)           // title is escaped; body is template.HTML you built from escaped values
    ui.WantsHTML(r) bool                      // true for a browser (Accept lists text/html), false for curl and scripts
    ui.Money(cents, currency) string          // 1999, "usd" -> "19.99 USD"

The rule every handler follows: `if ui.WantsHTML(r) { ui.Page(...) } else { write JSON }`. Browsers get pages,
scripts get JSON, on the same route.

## Files

    ui.go        the whole module
    ui_test.go   Money, WantsHTML, title escaping

## Not done

- One stylesheet, no theming hook beyond editing `uiCSS`. No CSP header (the chat template's inline script needs
  `script-src 'unsafe-inline'` if you add one).

## Test

    go test ./kit/ui
