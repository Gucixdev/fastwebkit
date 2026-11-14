package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stripe/stripe-go/v79/webhook"

	"fastwebapp/gems/database"
	"fastwebapp/gems/payments"
)

const whSecret = "whsec_test"

type env struct {
	h  http.HandlerFunc
	l  *payments.Ledger
	db *sql.DB
}

func newEnv(t *testing.T) env {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	l, err := payments.NewLedger(db)
	if err != nil {
		t.Fatal(err)
	}
	price := func(slug string) (int64, string, bool) {
		if slug == "widget" {
			return 3499, "usd", true
		}
		return 0, "", false
	}
	return env{webhookHandler(payments.NewStripe("sk_fake", whSecret), l, price), l, db}
}

func (e env) post(body string, sig string) int {
	r := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(body))
	if sig == "" {
		sig = webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: []byte(body), Secret: whSecret}).Header
	}
	r.Header.Set("Stripe-Signature", sig)
	w := httptest.NewRecorder()
	e.h(w, r)
	return w.Code
}

func (e env) grants(t *testing.T) int {
	var n int
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM payments`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func session(status, slug string, amount int, cur string) string {
	return fmt.Sprintf(`{"id":"evt_1","type":"checkout.session.completed","data":{"object":{"object":"checkout.session","payment_status":%q,"payment_intent":"pi_1","amount_total":%d,"currency":%q,"metadata":{"item_slug":%q}}}}`, status, amount, cur, slug)
}

func TestBadSignatureRejected(t *testing.T) {
	e := newEnv(t)
	if c := e.post(session("paid", "widget", 3499, "usd"), "t=1,v1=deadbeef"); c != http.StatusBadRequest {
		t.Fatalf("code %d", c)
	}
	if e.grants(t) != 0 {
		t.Fatal("granted on bad signature")
	}
}

func TestReplayIsIdempotent(t *testing.T) {
	e := newEnv(t)
	b := session("paid", "widget", 3499, "usd")
	for i := 0; i < 3; i++ {
		if c := e.post(b, ""); c != 200 {
			t.Fatalf("code %d", c)
		}
	}
	if n := e.grants(t); n != 1 {
		t.Fatalf("grants=%d", n)
	}
	if ok, _ := e.l.Paid("widget"); !ok {
		t.Fatal("not paid")
	}
}

func TestMismatchRefused(t *testing.T) {
	e := newEnv(t)
	for _, b := range []string{session("paid", "widget", 100, "usd"), session("paid", "widget", 3499, "eur"), session("paid", "widget", 9999, "usd")} {
		if c := e.post(b, ""); c != 200 {
			t.Fatalf("code %d", c)
		}
	}
	if e.grants(t) != 0 {
		t.Fatal("granted on mismatch")
	}
}

func TestOversizedBodyRejected(t *testing.T) {
	e := newEnv(t)
	if c := e.post(strings.Repeat("a", 1<<20+10), "t=1,v1=x"); c != http.StatusBadRequest {
		t.Fatalf("code %d", c)
	}
}

func TestUnknownSlugAndUnpaidIgnored(t *testing.T) {
	e := newEnv(t)
	e.post(session("paid", "nope", 3499, "usd"), "")
	e.post(session("unpaid", "widget", 3499, "usd"), "")
	if e.grants(t) != 0 {
		t.Fatal("granted")
	}
}

func TestConcurrentDuplicatesGrantOnce(t *testing.T) {
	e := newEnv(t)
	b := session("paid", "widget", 3499, "usd")
	var wg sync.WaitGroup
	var bad int32
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c := e.post(b, ""); c != 200 && c != 500 {
				atomic.AddInt32(&bad, 1)
			}
		}()
	}
	wg.Wait()
	// a 500 (sqlite busy) only makes the provider retry; redeliver once serially to settle
	e.post(b, "")
	if bad != 0 || e.grants(t) != 1 {
		t.Fatalf("bad=%d grants=%d", bad, e.grants(t))
	}
}

func refundBody(typ, obj string) string {
	return fmt.Sprintf(`{"id":"evt_r","type":%q,"data":{"object":%s}}`, typ, obj)
}

func TestStripeRefundAndDisputeRevokeIdempotently(t *testing.T) {
	for _, rb := range []string{
		refundBody("charge.refunded", `{"object":"charge","refunded":true,"payment_intent":"pi_1"}`),
		refundBody("charge.dispute.created", `{"object":"dispute","payment_intent":"pi_1"}`),
	} {
		e := newEnv(t)
		e.post(session("paid", "widget", 3499, "usd"), "")
		if ok, _ := e.l.Paid("widget"); !ok {
			t.Fatal("not paid")
		}
		for i := 0; i < 3; i++ {
			if c := e.post(rb, ""); c != 200 {
				t.Fatalf("code %d", c)
			}
		}
		if ok, _ := e.l.Paid("widget"); ok {
			t.Fatalf("still paid after %.30s", rb)
		}
	}
}

func TestPartialRefundKeepsEntitlementAndRefundBeforePayRevokes(t *testing.T) {
	e := newEnv(t)
	e.post(session("paid", "widget", 3499, "usd"), "")
	e.post(refundBody("charge.refunded", `{"object":"charge","refunded":false,"payment_intent":"pi_1"}`), "")
	if ok, _ := e.l.Paid("widget"); !ok {
		t.Fatal("partial refund revoked")
	}
	e2 := newEnv(t) // out-of-order delivery: refund first
	e2.post(refundBody("charge.refunded", `{"object":"charge","refunded":true,"payment_intent":"pi_1"}`), "")
	e2.post(session("paid", "widget", 3499, "usd"), "")
	if ok, _ := e2.l.Paid("widget"); ok {
		t.Fatal("refund-before-payment not honoured")
	}
}
