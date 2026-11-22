# website

A folder of Markdown files served as a small site. No database, no JavaScript, nothing to log in to.

Modules: `content`, `ui`.

## Run

    ./run                     # :8080, pages from ./pages

Environment: `ADDR` (`:8080`), `SITE_NAME` (shown top left), `BASE_URL` (used in the sitemap),
`PAGES_DIR` (`pages`).

## Write a page

Add `pages/pricing.md`:

    ---
    title: Pricing
    ---
    Text in **Markdown**. Raw HTML (an image, a video iframe) passes through.

It is served at `/items/pricing` and listed on `/`. Restart to pick up new files. Do not give a page a
`price_cents`: it would show a Buy button, and this template has no checkout (the saas template does).

## Routes

    GET /                  list of pages (HTML for browsers, JSON for scripts)
    GET /items/{slug}      one page
    GET /sitemap.xml       every page, with BASE_URL
    GET /robots.txt        points at the sitemap
    GET /healthz           "ok"

## Files

    main.go       config, graceful shutdown
    app.go        newApp: content routes + sitemap/robots/health
    app_test.go   httptest: HTML and JSON views, escaping, 404, sitemap
    pages/        two sample pages

## Test

    go test ./templates/website

Not verified: how it looks on a phone; the stylesheet is the `ui` module's one small block.
