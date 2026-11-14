package payments

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/plutov/paypal/v4"
)

// NewPayPal wires a real PayPal Orders v2 provider. clientID/secret/webhookID come from env vars
// in cmd/fastwebapp - never hardcode them. sandbox selects PayPal's sandbox vs live API base.
func NewPayPal(clientID, secret, webhookID string, sandbox bool) (Provider, error) {
	base := paypal.APIBaseLive
	if sandbox {
		base = paypal.APIBaseSandBox
	}
	return newPayPalAt(clientID, secret, webhookID, base)
}

// newPayPalAt is NewPayPal against an arbitrary API base, so tests can point it at a fake.
func newPayPalAt(clientID, secret, webhookID, base string) (Provider, error) {
	client, err := paypal.NewClient(clientID, secret, base)
	if err != nil {
		return nil, fmt.Errorf("paypal: new client: %w", err)
	}
	client.SetHTTPClient(&http.Client{Timeout: 15 * time.Second})
	return paypalProvider{client: client, webhookID: webhookID}, nil
}

type paypalProvider struct {
	client    *paypal.Client
	webhookID string
}

func (p paypalProvider) Name() string { return "paypal" }

func (p paypalProvider) CreateCheckout(c Checkout) (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := p.client.GetAccessToken(ctx); err != nil {
		return "", fmt.Errorf("paypal: get access token: %w", err)
	}

	order, err := p.client.CreateOrder(ctx, "CAPTURE", []paypal.PurchaseUnitRequest{{
		CustomID:    c.ItemSlug,
		Description: c.ItemTitle,
		Amount: &paypal.PurchaseUnitAmount{
			Currency: upper(c.Currency),
			Value:    centsToDecimal(c.PriceCents),
		},
	}}, nil, &paypal.ApplicationContext{
		ReturnURL: c.SuccessURL,
		CancelURL: c.CancelURL,
	})
	if err != nil {
		return "", fmt.Errorf("paypal: create order: %w", err)
	}

	for _, link := range order.Links {
		if link.Rel == "approve" {
			return link.Href, nil
		}
	}
	return "", fmt.Errorf("paypal: order %s has no approve link", order.ID)
}

func (p paypalProvider) VerifyWebhook(r *http.Request) (bool, string, error) {
	pay, err := p.VerifyPayment(r)
	return pay.Paid, pay.ItemSlug, err
}

func (p paypalProvider) VerifyPayment(r *http.Request) (Payment, error) {
	if p.webhookID == "" {
		return Payment{}, fmt.Errorf("paypal: webhook id not configured")
	}
	// readBody leaves a fresh reader on r.Body: VerifyWebhookSignature reads it again itself.
	body, err := readBody(r)
	if err != nil {
		return Payment{}, fmt.Errorf("paypal: read body: %w", err)
	}

	result, err := p.client.VerifyWebhookSignature(r.Context(), r, p.webhookID)
	if err != nil {
		return Payment{}, fmt.Errorf("paypal: verify webhook: %w", err)
	}
	if result.VerificationStatus != "SUCCESS" {
		return Payment{}, nil
	}

	var event struct {
		EventType string `json:"event_type"`
		Resource  struct {
			ID       string `json:"id"`
			Status   string `json:"status"`
			CustomID string `json:"custom_id"`
			Amount   struct {
				Value    string `json:"value"`
				Currency string `json:"currency_code"`
			} `json:"amount"`
			Links []struct {
				Rel  string `json:"rel"`
				Href string `json:"href"`
			} `json:"links"`
		} `json:"resource"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		return Payment{}, fmt.Errorf("paypal: decode event: %w", err)
	}
	switch event.EventType {
	case "CHECKOUT.ORDER.APPROVED":
		// Orders v2 does not move money when the buyer approves: the merchant must capture.
		// Capture here; PAYMENT.CAPTURE.COMPLETED arrives next and is the one that unlocks.
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if _, err := p.client.CaptureOrder(ctx, event.Resource.ID, paypal.CaptureOrderRequest{}); err != nil {
			return Payment{}, fmt.Errorf("paypal: capture order %s: %w", event.Resource.ID, err)
		}
		return Payment{}, nil
	case "PAYMENT.CAPTURE.COMPLETED":
		if event.Resource.Status != "" && event.Resource.Status != "COMPLETED" {
			return Payment{}, nil
		}
		cents, ok := decimalToCents(event.Resource.Amount.Value)
		return Payment{Paid: true, ItemSlug: event.Resource.CustomID, Known: ok, Ref: event.Resource.ID,
			AmountCents: cents, Currency: strings.ToLower(event.Resource.Amount.Currency)}, nil
	case "PAYMENT.CAPTURE.REFUNDED", "PAYMENT.CAPTURE.REVERSED":
		// The resource is the refund/reversal; the original capture is its "up" link.
		for _, l := range event.Resource.Links {
			if l.Rel == "up" {
				if i := strings.LastIndex(l.Href, "/"); i >= 0 && i < len(l.Href)-1 {
					return Payment{Revoke: true, Ref: l.Href[i+1:]}, nil
				}
			}
		}
	}
	return Payment{}, nil
}

func upper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 'a' + 'A'
		}
	}
	return string(b)
}

// centsToDecimal turns 3499 into "34.99" - PayPal's Amount.Value is a decimal string, unlike
// Stripe's integer minor-unit cents.
func centsToDecimal(cents int64) string {
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents // "-5" cents is "-0.05", not "0.05"
	}
	return sign + strconv.FormatInt(cents/100, 10) + "." + fmt.Sprintf("%02d", cents%100)
}
