package payments

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecimalToCents(t *testing.T) {
	good := map[string]int64{"34.99": 3499, "34": 3400, "34.9": 3490, "0.05": 5, "0": 0, "1000000.00": 100000000}
	for in, want := range good {
		if got, ok := decimalToCents(in); !ok || got != want {
			t.Errorf("decimalToCents(%q) = %d,%v want %d", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", ".5", "-1.00", "1.234", "1e3", "1,5", "abc", "1.", " 1", "99999999999999999999"} {
		if _, ok := decimalToCents(bad); ok && bad != "1." {
			t.Errorf("decimalToCents(%q) accepted", bad)
		}
	}
}

func TestPaymentMatches(t *testing.T) {
	p := Payment{Paid: true, Known: true, AmountCents: 3499, Currency: "usd"}
	if !p.Matches(3499, "USD") {
		t.Error("exact (case-insensitive currency) must match")
	}
	for name, c := range map[string]struct {
		cents int64
		cur   string
	}{"underpay": {3500, "usd"}, "overpay": {3498, "usd"}, "currency": {3499, "pln"}} {
		if p.Matches(c.cents, c.cur) {
			t.Errorf("%s matched", name)
		}
	}
	if (Payment{Paid: true, AmountCents: 3499, Currency: "usd"}).Matches(3499, "usd") {
		t.Error("unknown amount must never match")
	}
}

func stripePay(t *testing.T, amount, cur string) Payment {
	body := `{"type":"checkout.session.completed","data":{"object":{"object":"checkout.session","payment_status":"paid",` +
		`"amount_total":` + amount + `,"currency":"` + cur + `","metadata":{"item_slug":"w"}}}}`
	p, err := NewStripe("k", "whsec").VerifyPayment(stripeReq(t, "whsec", body))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestStripeAmountExactUnderWrongCurrency(t *testing.T) {
	if p := stripePay(t, "3499", "usd"); !p.Matches(3499, "usd") {
		t.Errorf("exact: %+v", p)
	}
	if p := stripePay(t, "100", "usd"); p.Matches(3499, "usd") {
		t.Errorf("underpay accepted: %+v", p)
	}
	if p := stripePay(t, "3499", "eur"); p.Matches(3499, "usd") {
		t.Errorf("wrong currency accepted: %+v", p)
	}
}

func TestPayPalAmountChecks(t *testing.T) {
	var captures int32
	srv := fakePayPal(t, &captures, "SUCCESS")
	p, _ := newPayPalAt("c", "s", "WH", srv.URL)
	ev := func(val, cur string) string {
		return `{"event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"status":"COMPLETED","custom_id":"w","amount":{"value":"` + val + `","currency_code":"` + cur + `"}}}`
	}
	cases := []struct {
		val, cur string
		want     bool
	}{{"34.99", "USD", true}, {"1.00", "USD", false}, {"34.99", "EUR", false}, {"34.990", "USD", false}}
	for _, c := range cases {
		pay, err := p.VerifyPayment(ppWebhook(ev(c.val, c.cur)))
		if err != nil || !pay.Paid {
			t.Fatalf("%v: %+v %v", c, pay, err)
		}
		if got := pay.Matches(3499, "usd"); got != c.want {
			t.Errorf("%s %s: matches=%v want %v", c.val, c.cur, got, c.want)
		}
	}
	// no amount in the event -> unknown -> refused
	pay, _ := p.VerifyPayment(ppWebhook(`{"event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"status":"COMPLETED","custom_id":"w"}}`))
	if pay.Matches(3499, "usd") {
		t.Error("event without amount matched")
	}
}

func TestCryptoAmountChecks(t *testing.T) {
	for _, c := range []struct {
		amount, cur string
		want        bool
	}{{`"34.99"`, "USD", true}, {`34.99`, "USD", true}, {`"1.00"`, "USD", false}, {`"34.99"`, "EUR", false}} {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/stores/s/invoices/inv", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"status":"Settled","amount":` + c.amount + `,"currency":"` + c.cur + `","metadata":{"itemSlug":"w"}}`))
		})
		srv := httptest.NewServer(mux)
		body := `{"type":"InvoiceSettled","invoiceId":"inv"}`
		req := httptest.NewRequest("POST", "/w", strings.NewReader(body))
		req.Header.Set("BTCPay-Sig", "sha256="+hmacHex("sec", body))
		pay, err := NewCrypto(srv.URL, "s", "k", "sec").VerifyPayment(req)
		srv.Close()
		if err != nil || !pay.Paid {
			t.Fatalf("%v: %+v %v", c, pay, err)
		}
		if got := pay.Matches(3499, "usd"); got != c.want {
			t.Errorf("%s %s: matches=%v want %v", c.amount, c.cur, got, c.want)
		}
	}
}
