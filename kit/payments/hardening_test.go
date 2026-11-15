package payments

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func stripeReq(t *testing.T, secret, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/w", strings.NewReader(body))
	req.Header.Set("Stripe-Signature", signStripe(t, secret, []byte(body)))
	return req
}

func TestStripeUnpaidSessionDoesNotUnlock(t *testing.T) {
	body := `{"type":"checkout.session.completed","data":{"object":{"object":"checkout.session","payment_status":"unpaid","metadata":{"item_slug":"w"}}}}`
	paid, _, err := NewStripe("k", "whsec").VerifyWebhook(stripeReq(t, "whsec", body))
	if err != nil || paid {
		t.Fatalf("unpaid delayed-method session must not unlock: paid=%v err=%v", paid, err)
	}
}

func TestStripeAsyncPaymentSucceededUnlocks(t *testing.T) {
	body := `{"type":"checkout.session.async_payment_succeeded","data":{"object":{"object":"checkout.session","payment_status":"paid","metadata":{"item_slug":"w"}}}}`
	paid, slug, err := NewStripe("k", "whsec").VerifyWebhook(stripeReq(t, "whsec", body))
	if err != nil || !paid || slug != "w" {
		t.Fatalf("paid=%v slug=%q err=%v", paid, slug, err)
	}
}

func TestEmptyWebhookSecretIsRefusedEverywhere(t *testing.T) {
	// With an empty key anyone can compute a valid HMAC - must be an error, not "verified".
	body := `{"type":"InvoiceSettled","invoiceId":"x"}`
	if _, _, err := NewStripe("k", "").VerifyWebhook(httptest.NewRequest("POST", "/w", strings.NewReader(body))); err == nil {
		t.Error("stripe accepted an empty secret")
	}
	req := httptest.NewRequest("POST", "/w", strings.NewReader(body))
	req.Header.Set("BTCPay-Sig", "sha256="+hmacHex("", body))
	if _, _, err := NewCrypto("http://x", "s", "k", "").VerifyWebhook(req); err == nil {
		t.Error("btcpay accepted a forged signature made with an empty key")
	}
	pp, _ := NewPayPal("c", "s", "", true)
	if _, _, err := pp.VerifyWebhook(httptest.NewRequest("POST", "/w", strings.NewReader(body))); err == nil {
		t.Error("paypal accepted an empty webhook id")
	}
}

func TestOversizedWebhookBodyRejected(t *testing.T) {
	big := strings.Repeat("a", maxWebhookBody+10)
	req := httptest.NewRequest("POST", "/w", strings.NewReader(big))
	if _, _, err := NewStripe("k", "whsec").VerifyWebhook(req); err == nil {
		t.Error("oversized body was accepted")
	}
}

func TestCheckoutValidate(t *testing.T) {
	ok := Checkout{ItemSlug: "a", PriceCents: 100, Currency: "usd"}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]Checkout{
		"zero":     {ItemSlug: "a", PriceCents: 0, Currency: "usd"},
		"negative": {ItemSlug: "a", PriceCents: -1, Currency: "usd"},
		"no slug":  {PriceCents: 1, Currency: "usd"},
		"bad cur":  {ItemSlug: "a", PriceCents: 1, Currency: "dollars"},
		"jpy":      {ItemSlug: "a", PriceCents: 1, Currency: "jpy"},
	} {
		if c.Validate() == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
	// invalid checkouts never reach the network
	if _, err := NewStripe("k", "w").CreateCheckout(Checkout{}); err == nil {
		t.Error("stripe CreateCheckout accepted an empty checkout")
	}
}

func TestCryptoUnsettledInvoiceDoesNotUnlock(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/stores/s/invoices/inv", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"Processing","metadata":{"itemSlug":"w"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	body := `{"type":"InvoiceSettled","invoiceId":"inv"}`
	req := httptest.NewRequest("POST", "/w", strings.NewReader(body))
	req.Header.Set("BTCPay-Sig", "sha256="+hmacHex("sec", body))
	paid, _, err := NewCrypto(srv.URL, "s", "k", "sec").VerifyWebhook(req)
	if err != nil || paid {
		t.Fatalf("paid=%v err=%v", paid, err)
	}
}

// fakePayPal serves just enough of PayPal's REST API to run the real plutov client end to end.
func fakePayPal(t *testing.T, captures *int32, verdict string) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"access_token":"tok","token_type":"Bearer","expires_in":3600}`))
	})
	mux.HandleFunc("POST /v2/checkout/orders", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"ORD1","links":[{"rel":"approve","href":"https://pp.example/approve/ORD1"}]}`))
	})
	mux.HandleFunc("POST /v2/checkout/orders/ORD1/capture", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(captures, 1)
		w.Write([]byte(`{"id":"ORD1","status":"COMPLETED"}`))
	})
	mux.HandleFunc("POST /v1/notifications/verify-webhook-signature", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"verification_status":"` + verdict + `"}`))
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func ppWebhook(body string) *http.Request {
	r := httptest.NewRequest("POST", "/w", strings.NewReader(body))
	for k, v := range map[string]string{"PAYPAL-AUTH-ALGO": "SHA256withRSA", "PAYPAL-CERT-URL": "https://x", "PAYPAL-TRANSMISSION-ID": "1", "PAYPAL-TRANSMISSION-SIG": "s", "PAYPAL-TRANSMISSION-TIME": "2026-01-01T00:00:00Z"} {
		r.Header.Set(k, v)
	}
	return r
}

func TestPayPalFullFlowAgainstFake(t *testing.T) {
	var captures int32
	srv := fakePayPal(t, &captures, "SUCCESS")
	p, err := newPayPalAt("c", "s", "WH", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	url, err := p.CreateCheckout(Checkout{ItemSlug: "widget", ItemTitle: "W", PriceCents: 3499, Currency: "usd", SuccessURL: "https://s", CancelURL: "https://c"})
	if err != nil || url != "https://pp.example/approve/ORD1" {
		t.Fatalf("url=%q err=%v", url, err)
	}
	// buyer approved -> we must capture, and it is not yet "paid"
	paid, _, err := p.VerifyWebhook(ppWebhook(`{"event_type":"CHECKOUT.ORDER.APPROVED","resource":{"id":"ORD1"}}`))
	if err != nil || paid || atomic.LoadInt32(&captures) != 1 {
		t.Fatalf("approved: paid=%v err=%v captures=%d", paid, err, captures)
	}
	// capture completed -> unlock
	paid, slug, err := p.VerifyWebhook(ppWebhook(`{"event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"id":"C1","status":"COMPLETED","custom_id":"widget"}}`))
	if err != nil || !paid || slug != "widget" {
		t.Fatalf("completed: paid=%v slug=%q err=%v", paid, slug, err)
	}
	// pending capture must not unlock
	paid, _, _ = p.VerifyWebhook(ppWebhook(`{"event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"status":"PENDING","custom_id":"widget"}}`))
	if paid {
		t.Error("pending capture unlocked")
	}
}

func TestPayPalFailedSignatureNeverCapturesOrUnlocks(t *testing.T) {
	var captures int32
	srv := fakePayPal(t, &captures, "FAILURE")
	p, _ := newPayPalAt("c", "s", "WH", srv.URL)
	for _, ev := range []string{
		`{"event_type":"CHECKOUT.ORDER.APPROVED","resource":{"id":"ORD1"}}`,
		`{"event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"status":"COMPLETED","custom_id":"widget"}}`,
	} {
		paid, _, _ := p.VerifyWebhook(ppWebhook(ev))
		if paid {
			t.Errorf("forged event unlocked: %s", ev)
		}
	}
	if captures != 0 {
		t.Errorf("forged event triggered %d captures", captures)
	}
}

func hmacHex(key, msg string) string {
	m := hmac.New(sha256.New, []byte(key))
	m.Write([]byte(msg))
	return hex.EncodeToString(m.Sum(nil))
}
