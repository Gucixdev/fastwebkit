# fastwebkit

stb-styled web framework for a solo dev: chatbots, crud, saas, dropshipping. Local, easy to migrate.

One language, no headache: **Go backend and Go-rendered pages** (`kit/ui`: one `Page` function, embedded CSS, light/dark,
no JavaScript; `GET /` and `/items` give HTML to a browser and JSON to curl), TypeScript only where a frontend part is complex.
The backend is split into modules, one folder each under `kit/` (`auth`, `content`, `downloads`, `payments`, ...), and the
templates under `templates/` (server, website, crud, chat, saas) are thin compositions of them. Copy the folder of one module
(plus `database/` and `ui/` if it lists them) into any Go project. Optional parts are separate files
(`payments/stripe.go`, `database/postgres.go`): delete the file and that part is gone from the build.

Browsers get pages, scripts get JSON, on the same routes: `GET /login` and `GET /register` are forms, a form POST sets the
session cookie and redirects to `next` (a local path only, anything else becomes `/`), a JSON POST answers JSON as before;
`GET /downloads` is a file list with an upload form when the request asks for HTML.

## Principles

1. **Critical paths (payments, auth, money) are tested to the bone** - fake provider servers, forged/replayed
   webhooks, race detector. A path that is not tested is not "done".
2. **Total independence / migration**: SQLite by default, Postgres by changing the DSN; payments behind one
   `Provider` interface (Stripe, PayPal, BTCPay crypto) - swapping a provider touches one file.
3. **Few, stable dependencies** only where they are boring and critical (stripe-go, pgx, x/crypto).
4. **Optimisations are mathematically checked** (property tests / reference implementation), never by feel.

## Layout

    kit/<module>/      Go modules, one folder each with its own README: ui database mongo content auth downloads payments
    fastwebapp/web/    TypeScript modules, one folder each: chat charts export layout render3d table widgets
    templates/<name>/  five runnable compositions of the modules: server website crud chat saas (each has `run`)
    fastwebapp/        the full demo (cmd/fastwebapp wires auth, content, downloads, payments), vite frontend, package.json
    docs/modules.md    the module table, who may import whom, what moved where, open decisions
    lean4/             proofs
    go.mod             module fastwebapp (root, so kit/, templates/ and fastwebapp/ are one module)

Modules import each other only as `docs/modules.md` allows (`ui` and `database` are the base modules); `modules_test.go`
enforces it. Names follow the folder: `auth.New`, `downloads.New`, `mongo.Open`, `payments.Item`, `ui.Site`.

Templates: `cd templates/chat && ./run`, or `go test ./templates/...`. Each is a copy-and-own starting point.

Test: `go test -race ./...` and `cd fastwebapp && npm test` (tsc, then vitest on web/ and frontend, 154 tests).

Payments, databases and what is proved or not verified: see `kit/payments/README.md` and `kit/database/README.md`.

## Lean 4 (lean4/) - proofs, not tests

`cd lean4 && lake build` (needs elan; toolchain pinned in lean-toolchain). Proved: money split is lossless and
injective, fraction < 100 (`Money.lean`); a replayed webhook never grants twice for any delivery sequence
(`Ledger.lean`). These prove the *model*; the Go code is tied to it by tests, not by extraction.
