# saas

A paid web app in four modules: accounts, a plan catalog written in Markdown, hosted checkout, and a members-only
page that opens when the signed-in user has paid. A refund closes it again.

Modules: `auth`, `content`, `payments`, `database`, `ui`.

## Run

    ./run                     # :8080, SQLite file saas.db, plans from ./plans

Environment: `ADDR`, `DATABASE_URL`, `SESSION_SECRET`, `PLANS_DIR` (`plans`), `BASE_URL` (where the buyer comes back,
`http://localhost:8080`) and one provider: `STRIPE_API_KEY` + `STRIPE_WEBHOOK_SECRET`, or `PAYPAL_CLIENT_ID` +
`PAYPAL_SECRET` + `PAYPAL_WEBHOOK_ID`, or `BTCPAY_SERVER_URL` + `BTCPAY_STORE_ID` + `BTCPAY_API_KEY` +
`BTCPAY_WEBHOOK_SECRET`. With none set the app boots; the first checkout calls Stripe with an empty key, is rejected and answers 502.

## Plans

`plans/pro.md` and `plans/team.md`: file name = plan id, front matter has `title`, `price_cents`, `currency`.
Add a file, restart, the plan is on sale. Free (price 0) and zero-decimal currencies (JPY) cannot be sold.

## Routes

    /register /login /logout        auth
    GET /, /items, /items/{plan}    the catalog, each plan with a Buy button
    POST /items/{plan}/checkout     signed-in only: redirect to the provider's checkout
    POST /webhooks/<provider>       signed provider event: grants or revokes
    GET /app                        200 {"plan"} for a member (HTML page for browsers), 402 without a plan
    GET /healthz

## How access is per user

`payments` records "item X was paid", not who paid. This template sells the item `"<plan>.<user id>"`, so the ledger
answers "did this user pay for this plan". The webhook looks the price up by the plan part, so an underpayment, a
wrong currency, a plan we do not sell or a missing user part is refused (HTTP 200, logged, nothing unlocked).
A member with several plans gets the dearest. Replayed deliveries grant once; a refund (revoke) is idempotent.

## Files

    main.go       config (provider from the environment), graceful shutdown
    app.go        newApp: the composition and the /app gate
    plans/        the catalog
    app_test.go   httptest with a fake provider: purchase, per-user isolation, forged/underpaid/replayed/refunded webhooks

## Test

    go test ./templates/saas

Not verified: any real provider (only the fake in the test; see `kit/payments/README.md`), a user deleting their
account, subscriptions or renewals (this is one-time payment per plan, not a recurring billing cycle).
