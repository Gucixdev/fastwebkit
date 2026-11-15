package kit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeBTCPayServer stands in for a real BTCPay Server instance: CreateCheckout/VerifyWebhook
// only need its Greenfield REST shape (invoice create + invoice get), not a live deployment -
// no real BTCPay credentials exist in this environment, so this is the honest way to exercise
// the HTTP + HMAC logic without pretending to hit a real server.
func fakeBTCPayServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/stores/store1/invoices", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["amount"] != "34.99" || body["currency"] != "USD" {
			t.Errorf("unexpected invoice request body: %v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"checkoutLink":"https://pay.example/invoice/inv1"}`))
	})
	mux.HandleFunc("GET /api/v1/stores/store1/invoices/inv1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"metadata":{"itemSlug":"widget"}}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestCryptoCreateCheckout(t *testing.T) {
	server := fakeBTCPayServer(t)
	provider := NewCrypto(server.URL, "store1", "key1", "whsecret")

	url, err := provider.CreateCheckout(Checkout{
		ItemSlug:   "widget",
		PriceCents: 3499,
		Currency:   "usd",
		SuccessURL: "https://example.com/success",
	})
	if err != nil {
		t.Fatalf("CreateCheckout: %v", err)
	}
	if url != "https://pay.example/invoice/inv1" {
		t.Errorf("got %q", url)
	}
}

func signBTCPay(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestCryptoVerifyWebhookValidSignatureSettled(t *testing.T) {
	server := fakeBTCPayServer(t)
	provider := NewCrypto(server.URL, "store1", "key1", "whsecret")

	payload := []byte(`{"type":"InvoiceSettled","invoiceId":"inv1"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/crypto", strings.NewReader(string(payload)))
	req.Header.Set("BTCPay-Sig", signBTCPay("whsecret", payload))

	paid, slug, err := provider.VerifyWebhook(req)
	if err != nil {
		t.Fatalf("VerifyWebhook: %v", err)
	}
	if !paid || slug != "widget" {
		t.Errorf("got paid=%v slug=%q, want paid=true slug=widget", paid, slug)
	}
}

func TestCryptoVerifyWebhookRejectsBadSignature(t *testing.T) {
	server := fakeBTCPayServer(t)
	provider := NewCrypto(server.URL, "store1", "key1", "whsecret")

	payload := []byte(`{"type":"InvoiceSettled","invoiceId":"inv1"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/crypto", strings.NewReader(string(payload)))
	req.Header.Set("BTCPay-Sig", signBTCPay("wrong-secret", payload))

	_, _, err := provider.VerifyWebhook(req)
	if err == nil {
		t.Fatal("expected an error for a bad signature")
	}
}

func TestCryptoVerifyWebhookIgnoresOtherEventTypes(t *testing.T) {
	server := fakeBTCPayServer(t)
	provider := NewCrypto(server.URL, "store1", "key1", "whsecret")

	payload := []byte(`{"type":"InvoiceCreated","invoiceId":"inv1"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/crypto", strings.NewReader(string(payload)))
	req.Header.Set("BTCPay-Sig", signBTCPay("whsecret", payload))

	paid, _, err := provider.VerifyWebhook(req)
	if err != nil {
		t.Fatalf("VerifyWebhook: %v", err)
	}
	if paid {
		t.Error("InvoiceCreated should not be treated as paid")
	}
}

func TestCryptoName(t *testing.T) {
	if NewCrypto("http://x", "s", "k", "w").Name() != "crypto" {
		t.Error("Name() should be \"crypto\"")
	}
}
