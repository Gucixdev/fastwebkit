package kit

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stripe/stripe-go/v79/webhook"
)

// signStripe uses stripe-go's own test-signing helper (GenerateTestSignedPayload) rather than
// reimplementing Stripe's signature scheme - it's the sanctioned way the library's own test
// suite mocks a webhook, so using it here is testing against the real verification path.
func signStripe(t *testing.T, secret string, payload []byte) string {
	t.Helper()
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload: payload,
		Secret:  secret,
	})
	return signed.Header
}

func TestStripeVerifyWebhookValidSignatureCompletedCheckout(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{"type":"checkout.session.completed","data":{"object":{"id":"cs_test_1","object":"checkout.session","payment_status":"paid","metadata":{"item_slug":"widget"}}}}`)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(string(payload)))
	req.Header.Set("Stripe-Signature", signStripe(t, secret, payload))

	provider := NewStripe("sk_test_fake", secret)
	paid, slug, err := provider.VerifyWebhook(req)
	if err != nil {
		t.Fatalf("VerifyWebhook: %v", err)
	}
	if !paid || slug != "widget" {
		t.Errorf("got paid=%v slug=%q, want paid=true slug=widget", paid, slug)
	}
}

func TestStripeVerifyWebhookRejectsBadSignature(t *testing.T) {
	payload := []byte(`{"type":"checkout.session.completed","data":{"object":{}}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(string(payload)))
	req.Header.Set("Stripe-Signature", signStripe(t, "wrong-secret", payload))

	provider := NewStripe("sk_test_fake", "whsec_test")
	_, _, err := provider.VerifyWebhook(req)
	if err == nil {
		t.Fatal("expected an error for a bad signature")
	}
}

func TestStripeVerifyWebhookIgnoresOtherEventTypes(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{"type":"payment_intent.created","data":{"object":{}}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(string(payload)))
	req.Header.Set("Stripe-Signature", signStripe(t, secret, payload))

	provider := NewStripe("sk_test_fake", secret)
	paid, _, err := provider.VerifyWebhook(req)
	if err != nil {
		t.Fatalf("VerifyWebhook: %v", err)
	}
	if paid {
		t.Error("payment_intent.created should not be treated as a completed checkout")
	}
}

func TestStripeName(t *testing.T) {
	if NewStripe("", "").Name() != "stripe" {
		t.Error("Name() should be \"stripe\"")
	}
}
