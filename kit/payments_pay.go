// payments is the payment module. Convention over configuration: set STRIPE_API_KEY, PAYPAL_CLIENT_ID
// or BTCPAY_SERVER_URL (plus that provider's secrets, see FromEnv), call Mount, done - checkout route, signed
// webhook route, exactly-once ledger and refund revocation are wired. Each provider lives in its own file
// (stripe.go, paypal.go, crypto.go); delete a file and that provider leaves the build. Nothing here knows about
// shop or items: the app only says what an item costs (Item, Mount's find).

package kit

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// ---- types and provider interface ----
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

// ---- guards ----

// maxWebhookBody caps what a webhook endpoint will read: real provider events are a few KB, so
// anything near this is an attack or a bug, not a payment.
const maxWebhookBody = 1 << 20

// readBody reads the request body (bounded) and puts a fresh reader back so the caller, or a
// second verification step, can read it again.
func readBody(r *http.Request) ([]byte, error) {
	b, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, maxWebhookBody))
	if err != nil {
		return nil, err
	}
	r.Body = io.NopCloser(strings.NewReader(string(b)))
	return b, nil
}

// Validate rejects a checkout that must never reach a provider: free or negative amounts,
// malformed currency codes, missing slug. Amounts are in minor units; zero-decimal currencies
// (jpy, huf, ...) are refused for PayPal/BTCPay conversion in centsToDecimal's callers via
// zeroDecimal, since "cents" there would be misread by a factor of 100.
func (c Checkout) Validate() error {
	switch {
	case c.ItemSlug == "":
		return errors.New("payments: empty item slug")
	case c.PriceCents <= 0:
		return fmt.Errorf("payments: price must be positive, got %d", c.PriceCents)
	case len(c.Currency) != 3:
		return fmt.Errorf("payments: currency %q is not a 3-letter ISO 4217 code", c.Currency)
	case zeroDecimal[strings.ToLower(c.Currency)]:
		return fmt.Errorf("payments: zero-decimal currency %q not supported", c.Currency)
	}
	return nil
}

// zeroDecimal lists currencies with no minor unit; supporting them needs a per-currency
// exponent, which is a deliberate follow-up, not a silent 100x error.
var zeroDecimal = map[string]bool{
	"bif": true, "clp": true, "djf": true, "gnf": true, "jpy": true, "kmf": true, "krw": true,
	"mga": true, "pyg": true, "rwf": true, "ugx": true, "vnd": true, "vuv": true, "xaf": true,
	"xof": true, "xpf": true, "huf": true, "twd": true,
}

// ---- ledger ----

// Ledger records paid webhooks exactly once. Providers retry deliveries (and attackers replay
// them); the same signed body must never grant an item twice. The key is provider + sha256 of
// the raw body - a retry is byte-identical, a different payment never is.
type Ledger struct{ db *sql.DB }

func NewLedger(db *sql.DB) (*Ledger, error) {
	// VARCHAR (not TEXT) so the composite key is legal on MySQL too.
	err := Migrate(db, `CREATE TABLE IF NOT EXISTS payments (
		provider   VARCHAR(32)  NOT NULL,
		event_hash VARCHAR(64)  NOT NULL,
		item_slug  VARCHAR(191) NOT NULL,
		created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (provider, event_hash)
	)`,
		// ref links a recorded payment to the provider's payment id; revocations are keyed by the
		// same (provider, ref), so a refund is idempotent and works even if it arrives first.
		`CREATE TABLE IF NOT EXISTS payment_refs (
		provider   VARCHAR(32)  NOT NULL,
		event_hash VARCHAR(64)  NOT NULL,
		ref        VARCHAR(191) NOT NULL,
		PRIMARY KEY (provider, event_hash)
	)`, `CREATE TABLE IF NOT EXISTS payment_revocations (
		provider   VARCHAR(32)  NOT NULL,
		ref        VARCHAR(191) NOT NULL,
		created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (provider, ref)
	)`)
	if err != nil {
		return nil, fmt.Errorf("payments ledger: %w", err)
	}
	return &Ledger{db: db}, nil
}

// Record stores a verified paid event. first is false when this exact event was seen before.
func (l *Ledger) Record(provider string, body []byte, slug string) (first bool, err error) {
	sum := sha256.Sum256(body)
	res, err := l.db.Exec(Rebind(l.db, InsertIgnore(l.db, `INSERT INTO payments (provider, event_hash, item_slug) VALUES (?, ?, ?)`)), provider, hex.EncodeToString(sum[:]), slug)
	if err != nil {
		return false, fmt.Errorf("payments ledger: %w", err)
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// RecordRef is Record plus the provider's payment ref, stored atomically with the payment so a
// later Revoke can find it. An empty ref behaves exactly like Record.
func (l *Ledger) RecordRef(provider string, body []byte, slug, ref string) (first bool, err error) {
	if ref == "" {
		return l.Record(provider, body, slug)
	}
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	tx, err := l.db.Begin()
	if err != nil {
		return false, fmt.Errorf("payments ledger: %w", err)
	}
	defer tx.Rollback()
	res, err := tx.Exec(Rebind(l.db, InsertIgnore(l.db, `INSERT INTO payments (provider, event_hash, item_slug) VALUES (?, ?, ?)`)), provider, hash, slug)
	if err != nil {
		return false, fmt.Errorf("payments ledger: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil || n != 1 {
		return false, err
	}
	if _, err := tx.Exec(Rebind(l.db, InsertIgnore(l.db, `INSERT INTO payment_refs (provider, event_hash, ref) VALUES (?, ?, ?)`)), provider, hash, ref); err != nil {
		return false, fmt.Errorf("payments ledger: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("payments ledger: %w", err)
	}
	return true, nil
}

// Revoke withdraws the entitlement of the payment with this provider ref (refund, chargeback,
// invalidated invoice). first is false when the ref was already revoked. Safe to call before the
// payment is recorded: Paid still treats it as revoked.
func (l *Ledger) Revoke(provider, ref string) (first bool, err error) {
	if ref == "" {
		return false, fmt.Errorf("payments ledger: empty ref")
	}
	res, err := l.db.Exec(Rebind(l.db, InsertIgnore(l.db, `INSERT INTO payment_revocations (provider, ref) VALUES (?, ?)`)), provider, ref)
	if err != nil {
		return false, fmt.Errorf("payments ledger: %w", err)
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// Paid reports whether any recorded, not-revoked payment unlocked slug.
func (l *Ledger) Paid(slug string) (bool, error) {
	var n int
	err := l.db.QueryRow(Rebind(l.db, `SELECT COUNT(*) FROM payments p WHERE p.item_slug = ?
		AND NOT EXISTS (SELECT 1 FROM payment_refs f JOIN payment_revocations r ON r.provider = f.provider AND r.ref = f.ref
			WHERE f.provider = p.provider AND f.event_hash = p.event_hash)`), slug).Scan(&n)
	return n > 0, err
}

// ---- the default wiring: Mount, Handler, FromEnv ----

// Item is what the app tells the module about something for sale.
type PayItem struct {
	Slug       string
	Title      string
	PriceCents int64
	Currency   string // lowercase ISO 4217
}

// Mount wires payments onto mux with zero config beyond the environment:
//
//	POST /items/{slug}/checkout   -> redirect to the provider's hosted checkout
//	POST /webhooks/{provider}     -> verified, amount-checked, exactly-once grant or revoke
//
// BASE_URL (default http://localhost:8080) is where the provider sends the buyer back. The returned Ledger
// answers Paid(slug). find says whether a slug is for sale and at what price.
func Mount(mux *http.ServeMux, db *sql.DB, find func(slug string) (PayItem, bool)) (*Ledger, error) {
	ledger, err := NewLedger(db)
	if err != nil {
		return nil, err
	}
	p := FromEnv()
	base := envOr("BASE_URL", "http://localhost:8080")
	mux.HandleFunc("POST /items/{slug}/checkout", func(w http.ResponseWriter, r *http.Request) {
		item, ok := find(r.PathValue("slug"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		url, err := p.CreateCheckout(Checkout{
			ItemSlug: item.Slug, ItemTitle: item.Title, PriceCents: item.PriceCents, Currency: item.Currency,
			SuccessURL: base + "/items/" + item.Slug + "?paid=1", CancelURL: base + "/items/" + item.Slug,
		})
		if err != nil {
			log.Printf("checkout %s: %v", item.Slug, err) // provider errors can carry request detail
			http.Error(w, "payment provider unavailable", http.StatusBadGateway)
			return
		}
		http.Redirect(w, r, url, http.StatusSeeOther)
	})
	mux.HandleFunc("POST /webhooks/"+p.Name(), Handler(p, ledger, func(slug string) (int64, string, bool) {
		item, ok := find(slug)
		return item.PriceCents, item.Currency, ok
	}))
	log.Printf("payments: %s", p.Name())
	return ledger, nil
}

// FromEnv picks the first provider whose variables are set: Stripe (STRIPE_API_KEY, STRIPE_WEBHOOK_SECRET),
// PayPal (PAYPAL_CLIENT_ID, PAYPAL_SECRET, PAYPAL_WEBHOOK_ID, PAYPAL_SANDBOX=true by default), BTCPay crypto
// (BTCPAY_SERVER_URL, BTCPAY_STORE_ID, BTCPAY_API_KEY, BTCPAY_WEBHOOK_SECRET). With none set it returns a
// keyless Stripe so the app still boots and checkout fails with a clear error per request.
func FromEnv() Provider {
	switch {
	case os.Getenv("STRIPE_API_KEY") != "":
		return NewStripe(os.Getenv("STRIPE_API_KEY"), os.Getenv("STRIPE_WEBHOOK_SECRET"))
	case os.Getenv("PAYPAL_CLIENT_ID") != "":
		p, err := NewPayPal(os.Getenv("PAYPAL_CLIENT_ID"), os.Getenv("PAYPAL_SECRET"), os.Getenv("PAYPAL_WEBHOOK_ID"),
			envOr("PAYPAL_SANDBOX", "true") == "true")
		if err != nil {
			log.Fatalf("paypal: %v", err)
		}
		return p
	case os.Getenv("BTCPAY_SERVER_URL") != "":
		return NewCrypto(os.Getenv("BTCPAY_SERVER_URL"), os.Getenv("BTCPAY_STORE_ID"),
			os.Getenv("BTCPAY_API_KEY"), os.Getenv("BTCPAY_WEBHOOK_SECRET"))
	}
	log.Println("payments: no provider configured (STRIPE_API_KEY / PAYPAL_CLIENT_ID / BTCPAY_SERVER_URL) - checkout will fail until one is")
	return NewStripe("", "")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// priceLookup returns what an item costs; ok is false for a slug we do not sell.
type priceLookup func(slug string) (priceCents int64, currency string, ok bool)

// Handler is the payment webhook: verify the provider signature, then either grant (paid, right
// amount/currency, known item, exactly once) or revoke (refund/chargeback).
func Handler(provider Provider, ledger *Ledger, price priceLookup) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		pay, err := provider.VerifyPayment(r)
		if err != nil {
			log.Printf("webhook %s rejected: %v", provider.Name(), err)
			http.Error(w, "rejected", http.StatusBadRequest)
			return
		}
		if pay.Revoke {
			first, err := ledger.Revoke(provider.Name(), pay.Ref)
			if err != nil {
				log.Printf("ledger revoke: %v", err)
				http.Error(w, "try again", http.StatusInternalServerError)
				return
			}
			if first {
				log.Println("revoked:", pay.Ref)
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		if pay.Paid {
			slug := pay.ItemSlug
			priceCents, currency, ok := price(slug)
			// Signature proves the provider sent it; this proves it was paid in full, in our currency,
			// for an item we sell. 200 so the provider stops retrying - a retry cannot fix an underpayment.
			if !ok || !pay.Matches(priceCents, currency) {
				log.Printf("payment REFUSED slug=%q known=%v amount=%d %s (want %d %s)", slug, pay.Known, pay.AmountCents, pay.Currency, priceCents, currency)
				w.WriteHeader(http.StatusOK)
				return
			}
			first, err := ledger.RecordRef(provider.Name(), body, slug, pay.Ref)
			if err != nil {
				// 500 makes the provider retry; nothing is lost, the ledger is the source of truth.
				log.Printf("ledger: %v", err)
				http.Error(w, "try again", http.StatusInternalServerError)
				return
			}
			if first {
				log.Println("paid:", slug)
			}
		}
		w.WriteHeader(http.StatusOK)
	}
}
