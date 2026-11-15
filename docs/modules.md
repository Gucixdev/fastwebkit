# Modules and templates

fastwebkit is a set of modules and a set of templates built from them. A module is one folder with its own
`README.md` (100 lines at most). A template is a small program in `templates/<name>/` that wires a few modules
together and nothing more.

## The rule

A module imports no other module, with two exceptions: the **base** modules `ui` and `database`, which exist to be
imported. The allowed edges are exactly the table below. `modules_test.go` at the repo root reads the imports of every
folder and fails on any other edge, on a missing or over-long README, and on a TypeScript module that reaches into
another module's folder. Modules talk to each other only through interfaces their README documents: a `*sql.DB`,
`http.ServeMux` routes, `payments.Provider`, a `find` function. Templates do the composing, never the modules.

## Go modules (`kit/<module>`, import path `fastwebapp/kit/<module>`)

| module      | what it is                                         | may import         |
|-------------|----------------------------------------------------|--------------------|
| `ui`        | HTML page shell, `WantsHTML`, money format (base)  | -                  |
| `database`  | `*sql.DB` for SQLite/Postgres/MySQL, dialect helpers (base) | -         |
| `mongo`     | MongoDB document store, own five-call API          | -                  |
| `content`   | Markdown folder as a catalog, list and detail pages | `ui`              |
| `auth`      | accounts, bcrypt, signed-cookie sessions           | `database`, `ui`   |
| `downloads` | file hosting with .torrent and magnet links        | `database`, `ui`   |
| `payments`  | hosted checkout, signed webhooks, exactly-once ledger | `database`      |

Optional parts stay one file each (`payments/stripe.go`, `database/postgres.go`): delete the file and that part is
gone from the build. To use a module outside this repo, copy its folder plus the base modules it lists, and change the
import path (the module is named `fastwebapp` in `go.mod`).

## TypeScript modules (`fastwebapp/web/<module>`)

No module imports another folder. Files in a folder may import each other (`table/datatable.ts` uses
`table/pagination.ts`), and the README says so.

| module     | files                                                              |
|------------|--------------------------------------------------------------------|
| `chat`     | chat (message tree, view), chat_crypto (ECDH + AES-GCM)            |
| `charts`   | bar and line charts on canvas                                      |
| `export`   | CSV, XLSX read/write, stored ZIP                                   |
| `layout`   | classic grid layout, constraint (Cassowary) layout                 |
| `render3d` | three.js scene wrapper                                             |
| `table`    | data table, pagination                                             |
| `widgets`  | notifications, progressbar, searchbar, spring, theme, upload       |

## Templates (`templates/<name>`)

Each has `run` (no arguments, settings from the environment), `main.go` (config, graceful shutdown), `app.go`
(`newApp`: the composition, tested with `httptest`), `app_test.go`, `README.md`. A template is meant to be copied and
owned; it is the only place where modules meet.

| template  | modules                                  | what you get                                           |
|-----------|------------------------------------------|--------------------------------------------------------|
| `server`  | auth, downloads, database                | JSON API: accounts, `/me`, file store, health, panic recovery, access log |
| `website` | content, ui                              | Markdown folder as a site, sitemap, robots, no database |
| `crud`    | auth, database, ui                       | one owner-scoped resource as JSON and HTML pages       |
| `chat`    | auth, database, ui                       | rooms, history, live messages over server-sent events  |
| `saas`    | auth, content, payments, database, ui    | plans in Markdown, hosted checkout, members-only `/app`, per-user access, refunds close it |

## Where things moved

    kit/auth.go, auth_*_test.go          -> kit/auth/        (auth_ prefix dropped, AuthNew -> auth.New)
    kit/content.go, content_items/       -> kit/content/     (items -> kit/content/items)
    kit/database_*.go                    -> kit/database/    (database_ prefix dropped)
    kit/downloads*.go                    -> kit/downloads/   (DownloadsNew -> downloads.New)
    kit/mongo*.go                        -> kit/mongo/       (MongoOpen -> mongo.Open)
    kit/payments_*.go                    -> kit/payments/    (PayItem -> payments.Item)
    kit/ui.go                            -> kit/ui/          (UISite -> ui.Site, uiMoney -> ui.Money)
    fastwebapp/web/<file>.ts             -> fastwebapp/web/<module>/<file>.ts
    the catalog HTML tests in ui_test.go -> kit/content/html_test.go (they test content, not ui)

Added: `auth.UserByID` (the chat template shows who wrote a message), `templates/`, `modules_test.go`.

## Open decisions

- The Go module is still called `fastwebapp`; renaming it to `fastwebkit` touches every import line.
- `templates/chat` holds its own room logic (`rooms.go`, about 300 lines). A second consumer would be the reason to
  promote it to `kit/chat`; until then it stays in the template.
- The TypeScript modules still live under `fastwebapp/` because `package.json` and `node_modules` do. Moving them to a
  top-level `web/` is one `git mv` plus the paths in `tsconfig.json`, `vitest.config.ts`, `vite.config.ts`.
