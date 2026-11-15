package payments

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/checkout/session"
	"github.com/stripe/stripe-go/v79/webhook"
)

// NewStripe wires a real Stripe Checkout provider. apiKey and webhookSecret come from env vars
// in cmd/fastwebapp - never hardcode a key.
func NewStripe(apiKey, webhookSecret string) Provider {
	return stripeProvider{apiKey: apiKey, webhookSecret: webhookSecret}
}

type stripeProvider struct {
	apiKey        string
	webhookSecret string
}

func (s stripeProvider) Name() string { return "stripe" }

func (s stripeProvider) CreateCheckout(c Checkout) (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	stripe.Key = s.apiKey

	params := &stripe.CheckoutSessionParams{
		Mode:       stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL: stripe.String(c.SuccessURL),
		CancelURL:  stripe.String(c.CancelURL),
		LineItems: []*stripe.CheckoutSessionLineItemParams{{
			Quantity: stripe.Int64(1),
			PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
				Currency:   stripe.String(c.Currency),
				UnitAmount: stripe.Int64(c.PriceCents),
				ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
					Name: stripe.String(c.ItemTitle),
				},
			},
		}},
		Metadata: map[string]string{"item_slug": c.ItemSlug},
	}

	sess, err := session.New(params)
	if err != nil {
		return "", fmt.Errorf("stripe: create checkout session: %w", err)
	}
	return sess.URL, nil
}

func (s stripeProvider) VerifyWebhook(r *http.Request) (bool, string, error) {
	p, err := s.VerifyPayment(r)
	return p.Paid, p.ItemSlug, err
}

func (s stripeProvider) VerifyPayment(r *http.Request) (Payment, error) {
	if s.webhookSecret == "" {
		return Payment{}, fmt.Errorf("stripe: webhook secret not configured")
	}
	payload, err := readBody(r)
	if err != nil {
		return Payment{}, fmt.Errorf("stripe: read body: %w", err)
	}
	// IgnoreAPIVersionMismatch: stripe-go's default ConstructEvent rejects a webhook outright if
	// the account's pinned Stripe API version doesn't exactly match this library build's expected
	// version - but we only read event.Type and re-unmarshal event.Data.Raw ourselves below rather
	// than relying on stripe-go's version-specific struct decoding, so that mismatch doesn't affect
	// us and shouldn't cause every webhook from a differently-pinned account to fail.
	event, err := webhook.ConstructEventWithOptions(payload, r.Header.Get("Stripe-Signature"), s.webhookSecret,
		webhook.ConstructEventOptions{IgnoreAPIVersionMismatch: true})
	if err != nil {
		return Payment{}, fmt.Errorf("stripe: verify webhook: %w", err)
	}
	switch event.Type {
	case "charge.refunded":
		var ch struct {
			Refunded      bool            `json:"refunded"`
			PaymentIntent json.RawMessage `json:"payment_intent"`
		}
		if err := json.Unmarshal(event.Data.Raw, &ch); err != nil {
			return Payment{}, fmt.Errorf("stripe: decode charge: %w", err)
		}
		// Partial refunds keep the entitlement; only a fully refunded charge revokes.
		if ref := stripeRef(ch.PaymentIntent); ch.Refunded && ref != "" {
			return Payment{Revoke: true, Ref: ref}, nil
		}
		return Payment{}, nil
	case "charge.dispute.created":
		var d struct {
			PaymentIntent json.RawMessage `json:"payment_intent"`
		}
		if err := json.Unmarshal(event.Data.Raw, &d); err != nil {
			return Payment{}, fmt.Errorf("stripe: decode dispute: %w", err)
		}
		if ref := stripeRef(d.PaymentIntent); ref != "" {
			return Payment{Revoke: true, Ref: ref}, nil
		}
		return Payment{}, nil
	case "checkout.session.completed", "checkout.session.async_payment_succeeded":
	default:
		return Payment{}, nil
	}

	var sess stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
		return Payment{}, fmt.Errorf("stripe: decode session: %w", err)
	}
	// checkout.session.completed also fires for delayed methods (SEPA, bank transfer) while the
	// money is still "unpaid" - only a paid session may unlock anything.
	if sess.PaymentStatus != stripe.CheckoutSessionPaymentStatusPaid {
		return Payment{}, nil
	}
	ref := ""
	if sess.PaymentIntent != nil {
		ref = sess.PaymentIntent.ID
	}
	return Payment{Paid: true, ItemSlug: sess.Metadata["item_slug"], Known: true, Ref: ref,
		AmountCents: sess.AmountTotal, Currency: strings.ToLower(string(sess.Currency))}, nil
}

// stripeRef reads a payment_intent that is either an id string or an expanded object.
func stripeRef(raw json.RawMessage) string {
	var id string
	if json.Unmarshal(raw, &id) == nil {
		return id
	}
	var obj struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &obj)
	return obj.ID
}
