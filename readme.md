# fastwebkit

stb-styled web framework for a solo dev: chatbots, crud, saas, dropshipping. Local, easy to migrate.

One language, no headache: **Go backend + TypeScript gems** (the frontend). A "gem" is a small standalone
package (`gems/<name>/`) that knows nothing about the others; the app (`fastwebapp/cmd`) wires them together.

## Principles

1. **Critical paths (payments, auth, money) are tested to the bone** - fake provider servers, forged/replayed
   webhooks, race detector. A path that is not tested is not "done".
2. **Total independence / migration**: SQLite by default, Postgres by changing the DSN; payments behind one
   `Provider` interface (Stripe, PayPal, BTCPay crypto) - swapping a provider touches one file.
3. **Few, stable dependencies** only where they are boring and critical (stripe-go, pgx, x/crypto).
4. **Optimisations are mathematically checked** (property tests / reference implementation), never by feel.

## Layout

    gems/        payments auth database content downloads | TS: chat charts datatable export layout ...
    fastwebapp/  cmd/fastwebapp (Go wiring), frontend/ (vite)
    go.mod       module fastwebapp (root, so gems/ and fastwebapp/ are one module)

Test: `go test -race ./gems/...`  and  `cd fastwebapp && npx vitest run`.

## Payments - what is guaranteed (all covered by tests)

- Webhook signatures verified before anything else; empty secret / webhook id = refuse (an empty HMAC key is forgeable).
- Stripe: only `payment_status=paid` unlocks (delayed methods are `unpaid` on `completed`); async success handled.
- PayPal: the order is **captured** on `CHECKOUT.ORDER.APPROVED` (Orders v2 moves no money otherwise); only a
  `COMPLETED` capture unlocks; a forged event never captures. Tested against a fake PayPal API.
- BTCPay: invoice is re-fetched and must be `Settled`.
- `Checkout.Validate`: no free/negative price, 3-letter currency, no zero-decimal currencies (JPY would be off x100).
- Body capped at 1 MiB, HTTP timeouts everywhere, provider errors are not leaked to the client.
- Amount + currency: `VerifyPayment` returns what was really charged (Stripe amount_total, PayPal capture amount, BTCPay invoice); the app unlocks only if `Payment.Matches` the catalog price (underpay/overpay/wrong currency/unknown amount refused, logged, HTTP 200 so no retry storm). Proved in `lean4/Money.lean`.
- Webhook HTTP handler (`cmd/fastwebapp/webhook.go`) tested end to end with httptest + SQLite: bad signature, replay, amount/currency mismatch, oversized body, unknown slug, unpaid, 50 concurrent duplicates, refund/dispute revoke (idempotent, order-independent).
- `Ledger`: same webhook delivered twice (retry/replay, 20 concurrent) grants exactly once.

### Not verified yet (honest list)
- Nothing ran against real Stripe/PayPal sandbox or a real BTCPay - only fakes. Run sandbox end-to-end before going live.
- Postgres path of `database` and `Ledger` (`ON CONFLICT`, `?` placeholders vs `$1`) untested - only SQLite.
- Refund/chargeback revocation (Stripe `charge.refunded` full + `charge.dispute.created`, BTCPay `InvoiceInvalid`/`InvoiceExpired`, PayPal `PAYMENT.CAPTURE.REFUNDED`/`REVERSED`) is tested against fakes / signed fixtures only. Unconfirmed vs real payloads: PayPal refund->capture mapping relies on the `up` link; Stripe `payment_intent` must be present on the charge/dispute; partial Stripe refunds deliberately keep access. Revocation is by provider ref, so payments recorded before this change (no ref) can't be revoked.
- Orders table is just the ledger; no user<->purchase link yet.

## Databases (gems/database, gems/mongo) - one file per backend

`sqlite.go`, `postgres.go`, `mysql.go` each register themselves; delete a file and that backend leaves the build.
DSN picks the backend: bare path / `sqlite:`, `postgres://`, `mysql://`. `Rebind` (`?` -> `$n`), `DDL` (BLOB/TEXT keys
per dialect) and `InsertIgnore` keep gem SQL written once. MongoDB is a document store: `gems/mongo/mongo.go`, own API.
Same contract tests ran on real SQLite, PostgreSQL 17, MariaDB 11 and MongoDB 8 (podman). Re-run:

    podman run -d --rm --name fwk-pg -e POSTGRES_PASSWORD=pw -p 127.0.0.1:55432:5432 postgres:17-alpine
    podman run -d --rm --name fwk-my -e MARIADB_ROOT_PASSWORD=pw -e MARIADB_DATABASE=t -p 127.0.0.1:53306:3306 mariadb:11
    podman run -d --rm --name fwk-mongo -p 127.0.0.1:57017:27017 mongo:8
    FWK_TEST_POSTGRES=postgres://postgres:pw@127.0.0.1:55432/postgres FWK_TEST_MYSQL=mysql://root:pw@127.0.0.1:53306/t \
    FWK_TEST_MONGO=mongodb://127.0.0.1:57017 go test -race ./gems/...

auth + downloads tests also ran green on all three SQL backends (`FWK_TEST_DB=postgres|mysql|sqlite go test ./gems/auth ./gems/downloads`, helper `database.OpenTest`); gems/content has no SQL. Not done: Redis/KV, DuckDB.

## Lean 4 (lean4/) - proofs, not tests

`cd lean4 && lake build` (needs elan; toolchain pinned in lean-toolchain). Proved: money split is lossless and
injective, fraction < 100 (`Money.lean`); a replayed webhook never grants twice for any delivery sequence
(`Ledger.lean`). These prove the *model*; the Go code is tied to it by tests, not by extraction.
