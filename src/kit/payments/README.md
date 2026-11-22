# payments

Hosted checkout and a verified, exactly-once ledger, behind one `Provider` interface. Convention over
configuration: set `STRIPE_API_KEY` (or `PAYPAL_CLIENT_ID`, or `BTCPAY_SERVER_URL`) plus that provider's secrets
and call `payments.Mount(mux, db, find)`. Nothing here knows about shops or users: the app only says what an
item costs (`Item`).

Depends on: `database` (the ledger tables). Used by: the saas template.

## Interface

    payments.Mount(mux, db, find) (*Ledger, error)   // POST /items/{slug}/checkout, POST /webhooks/<provider>
    payments.FromEnv() Provider                      // first provider whose variables are set
    payments.Handler(provider, ledger, price)        // the webhook handler alone, for apps that wire their own routes
    payments.NewLedger(db)                           // Record / RecordRef / Revoke / Paid(slug)
    payments.Provider                                // Name, CreateCheckout, VerifyWebhook, VerifyPayment
    payments.Checkout{...}.Validate()   payments.Payment.Matches(cents, currency)

Env: Stripe `STRIPE_API_KEY` `STRIPE_WEBHOOK_SECRET`; PayPal `PAYPAL_CLIENT_ID` `PAYPAL_SECRET` `PAYPAL_WEBHOOK_ID`
`PAYPAL_SANDBOX` (default true); BTCPay `BTCPAY_SERVER_URL` `BTCPAY_STORE_ID` `BTCPAY_API_KEY` `BTCPAY_WEBHOOK_SECRET`;
`BASE_URL` (where the buyer returns, default http://localhost:8080).

## Files

    pay.go        types, guards, ledger, Mount, Handler, FromEnv
    stripe.go     one provider per file; delete a file and that provider leaves the build
    paypal.go
    crypto.go     BTCPay
    *_test.go     fake provider servers, forged and replayed webhooks, concurrency

## Guaranteed (each is covered by a test)

- Signature checked before anything else; an empty secret or webhook id refuses (an empty HMAC key is forgeable).
- Stripe: only `payment_status=paid` unlocks. PayPal: the order is captured on approval and only a `COMPLETED`
  capture unlocks. BTCPay: the invoice is re-fetched and must be `Settled`.
- The amount and currency really charged must equal the item's price, else refused and logged (HTTP 200, so no
  retry storm). Proved in `lean4/Money.lean`.
- `Checkout.Validate`: no free or negative price, 3-letter currency, no zero-decimal currencies (JPY would be off x100).
- Same webhook delivered twice, even 50 at once, grants once. Refund and chargeback revoke, idempotent and
  order-independent. Body capped at 1 MiB, timeouts everywhere, provider errors not leaked to the client.

## Not verified

- Nothing ran against real Stripe or PayPal sandboxes or a real BTCPay, only fakes. Run a sandbox end to end first.
- Postgres/MySQL ledger paths (`ON CONFLICT`, `$n`) were run before the split, not after.
- Refund and chargeback revocation (Stripe `charge.refunded` full and `charge.dispute.created`, BTCPay
  `InvoiceInvalid`/`InvoiceExpired`, PayPal `PAYMENT.CAPTURE.REFUNDED`/`REVERSED`) is tested against fakes and signed
  fixtures only. Unconfirmed against real payloads: PayPal's refund-to-capture mapping relies on the `up` link; Stripe's
  `payment_intent` must be present on the charge or dispute.
- Partial Stripe refunds keep access on purpose; payments recorded before refs existed cannot be revoked.
- Access is per item slug, not per user. To sell per user, encode the user in the slug (the saas template does).

## Test

    go test -race ./kit/payments
