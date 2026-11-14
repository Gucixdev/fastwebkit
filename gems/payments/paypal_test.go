package payments

import "testing"

// PayPal's CreateCheckout/VerifyWebhook both call PayPal's real API (GetAccessToken,
// VerifyWebhookSignature) - there's no local-only path to exercise, unlike Stripe's webhook
// verification (pure HMAC) or BTCPay's (HMAC + a self-hosted server this test can fake). No
// PayPal sandbox credentials are available in this environment, so only client construction
// and the provider name - the parts that don't require a network call - are covered here.
func TestNewPayPalConstructsForSandboxAndLive(t *testing.T) {
	sandbox, err := NewPayPal("client-id", "secret", "webhook-id", true)
	if err != nil {
		t.Fatalf("NewPayPal(sandbox=true): %v", err)
	}
	if sandbox.Name() != "paypal" {
		t.Errorf("Name() = %q, want %q", sandbox.Name(), "paypal")
	}

	live, err := NewPayPal("client-id", "secret", "webhook-id", false)
	if err != nil {
		t.Fatalf("NewPayPal(sandbox=false): %v", err)
	}
	if live.Name() != "paypal" {
		t.Errorf("Name() = %q, want %q", live.Name(), "paypal")
	}
}
