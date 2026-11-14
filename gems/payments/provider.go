// Package payments is the payment gem: one interface every method (Stripe, PayPal, crypto)
// plugs into. A standalone gem - shop uses it, but nothing here knows about shop or items.
package payments

import (
	"net/http"
	"strconv"
	"strings"
)

type Checkout struct {
	ItemSlug   string
	ItemTitle  string
	PriceCents int64
	Currency   string // ISO 4217, lowercase ("usd", "pln")
	SuccessURL string
	CancelURL  string
}

// Provider creates a hosted checkout and verifies the provider's webhook. VerifyWebhook takes
// the whole request (not just a body+signature pair) because providers disagree on what they
// need to check a signature - Stripe wants one header, PayPal wants several plus a stored
// webhook ID - and reads the body itself.
type Provider interface {
	Name() string
	CreateCheckout(c Checkout) (redirectURL string, err error)
	VerifyWebhook(r *http.Request) (paid bool, itemSlug string, err error)
	// VerifyPayment is VerifyWebhook plus what was actually charged, so the app can refuse an
	// underpaid or wrong-currency payment (see Payment.Matches).
	VerifyPayment(r *http.Request) (Payment, error)
}

// Payment is a verified webhook outcome. Amount and Currency are what the provider says was
// really paid; Known is false when the event did not carry them.
type Payment struct {
	Paid        bool
	ItemSlug    string
	Known       bool
	AmountCents int64
	Currency    string // lowercase ISO 4217
	// Ref is the provider's id for the underlying payment (Stripe payment_intent, PayPal capture
	// id, BTCPay invoice id). A later refund/chargeback names the same Ref, which is how the
	// ledger finds what to revoke.
	Ref string
	// Revoke marks a verified refund/chargeback/invalidation event for Ref: the entitlement must
	// be withdrawn. Paid is false on such an event.
	Revoke bool
}

// Matches reports whether the payment is for exactly priceCents in currency. An unknown amount
// never matches: refusing to unlock is the safe failure.
func (p Payment) Matches(priceCents int64, currency string) bool {
	return p.Known && p.AmountCents == priceCents && strings.EqualFold(p.Currency, currency)
}

// decimalToCents parses a provider decimal string ("34.99", "34", "34.9") into minor units.
// Strict: digits only, at most two decimals, no sign, no exponent.
func decimalToCents(s string) (int64, bool) {
	whole, frac, _ := strings.Cut(s, ".")
	if whole == "" || len(frac) > 2 {
		return 0, false
	}
	for _, part := range []string{whole, frac} {
		for _, c := range part {
			if c < '0' || c > '9' {
				return 0, false
			}
		}
	}
	for len(frac) < 2 {
		frac += "0"
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || w > (1<<62)/100 {
		return 0, false
	}
	f, _ := strconv.ParseInt(frac, 10, 64)
	return w*100 + f, true
}
