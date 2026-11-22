# content

A folder of Markdown files as a catalog: YAML front matter for the structured fields (title, price, currency), the
rest of the file rendered to HTML. List and detail pages come with it. No payment logic: a priced item shows a Buy
form that posts to `/items/{slug}/checkout`, and whoever composes the app (see `payments`, the saas template)
decides what that route does.

Depends on: `ui` (pages, money format). Used by: the website and saas templates.

## An item

    ---
    title: Raspberry Pi 5 Aluminum Case
    price_cents: 3499
    currency: usd
    ---
    Markdown body. Raw HTML (an <img>, a YouTube <iframe>) passes straight through.

The slug is the file name without `.md`. Without front matter the title is the file name and the price is 0
(no Buy form). Item files are written by the site owner, not visitors, so raw HTML is allowed.

## Interface

    content.Load(dir) (*Catalog, error)       // Catalog{Items []Item, BySlug map[string]Item}
    content.LoadDir(dir) ([]Item, error)
    c.Routes(mux)                              // GET /{$}, GET /items, GET /items/{slug}
    Item{Slug, Title, PriceCents, Currency, HTML}

`/items` answers HTML to a browser and JSON to a script.

## Files

    content.go      Load, Routes, rendering
    items/          one sample item, used by the full demo (fastwebapp/)
    *_test.go       loading, handlers, escaping of titles in the pages

## Not done

- No pagination, no tags, no search; the whole folder is read once at startup (restart to pick up edits).

## Test

    go test ./kit/content
