package payments

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

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
