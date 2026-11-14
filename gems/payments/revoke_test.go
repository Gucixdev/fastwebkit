package payments

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"fastwebapp/gems/database"
)

func TestBTCPayInvalidAfterSettledRevokes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/stores/s/invoices/inv1", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"Invalid","amount":"34.99","currency":"USD","metadata":{"itemSlug":"w"}}`))
	})
	mux.HandleFunc("GET /api/v1/stores/s/invoices/inv2", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"Settled","amount":"34.99","currency":"USD"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	p := NewCrypto(srv.URL, "s", "k", "sec")
	verify := func(body string) Payment {
		r := httptest.NewRequest("POST", "/w", strings.NewReader(body))
		r.Header.Set("BTCPay-Sig", signBTCPay("sec", []byte(body)))
		pay, err := p.VerifyPayment(r)
		if err != nil {
			t.Fatal(err)
		}
		return pay
	}
	if pay := verify(`{"type":"InvoiceInvalid","invoiceId":"inv1"}`); !pay.Revoke || pay.Ref != "inv1" {
		t.Fatalf("%+v", pay)
	}
	if pay := verify(`{"type":"InvoiceExpired","invoiceId":"inv1"}`); !pay.Revoke {
		t.Fatalf("%+v", pay)
	}
	if pay := verify(`{"type":"InvoiceExpired","invoiceId":"inv2"}`); pay.Revoke {
		t.Fatal("revoked a still-settled invoice")
	}
}

func TestPayPalRefundReversedRevoke(t *testing.T) {
	var c int32
	srv := fakePayPal(t, &c, "SUCCESS")
	p, _ := newPayPalAt("c", "s", "WH", srv.URL)
	for _, typ := range []string{"PAYMENT.CAPTURE.REFUNDED", "PAYMENT.CAPTURE.REVERSED"} {
		ev := `{"event_type":"` + typ + `","resource":{"id":"R1","links":[{"rel":"self","href":"https://api/v2/payments/refunds/R1"},{"rel":"up","href":"https://api/v2/payments/captures/C1"}]}}`
		pay, err := p.VerifyPayment(ppWebhook(ev))
		if err != nil || !pay.Revoke || pay.Ref != "C1" || pay.Paid {
			t.Fatalf("%s: %+v %v", typ, pay, err)
		}
	}
	pay, _ := p.VerifyPayment(ppWebhook(`{"event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"id":"C1","status":"COMPLETED","custom_id":"w","amount":{"value":"34.99","currency_code":"USD"}}}`))
	if pay.Ref != "C1" || !pay.Paid {
		t.Fatalf("%+v", pay)
	}
}

func TestLedgerRevokeIdempotent(t *testing.T) {
	db, _ := database.Open(filepath.Join(t.TempDir(), "t.db"))
	defer db.Close()
	l, _ := NewLedger(db)
	if f, err := l.RecordRef("paypal", []byte("a"), "w", "C1"); !f || err != nil {
		t.Fatal(f, err)
	}
	if f, _ := l.RecordRef("paypal", []byte("a"), "w", "C1"); f {
		t.Fatal("dup granted")
	}
	if f, _ := l.Revoke("paypal", "C1"); !f {
		t.Fatal("first revoke")
	}
	if f, _ := l.Revoke("paypal", "C1"); f {
		t.Fatal("second revoke not idempotent")
	}
	if ok, _ := l.Paid("w"); ok {
		t.Fatal("still paid")
	}
}
