package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"fastwebapp/kit/database"
	"fastwebapp/kit/payments"
)

// fakeProvider stands in for Stripe/PayPal/BTCPay: no network. A webhook is "signed" by the header
// X-Fake-Sig: ok and carries {slug, amount, currency, ref, revoke} as JSON.
type fakeProvider struct{ checkouts []payments.Checkout }

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) CreateCheckout(c payments.Checkout) (string, error) {
	f.checkouts = append(f.checkouts, c)
	return "https://pay.example/session/" + url.PathEscape(c.ItemSlug), nil
}

func (f *fakeProvider) VerifyWebhook(r *http.Request) (bool, string, error) {
	p, err := f.VerifyPayment(r)
	return p.Paid, p.ItemSlug, err
}

func (f *fakeProvider) VerifyPayment(r *http.Request) (payments.Payment, error) {
	if r.Header.Get("X-Fake-Sig") != "ok" {
		return payments.Payment{}, errors.New("bad signature")
	}
	var in struct {
		Slug, Currency, Ref string
		Amount              int64
		Revoke              bool
	}
	b, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(b, &in); err != nil {
		return payments.Payment{}, err
	}
	if in.Revoke {
		return payments.Payment{Revoke: true, Ref: in.Ref}, nil
	}
	return payments.Payment{Paid: true, ItemSlug: in.Slug, Known: true, AmountCents: in.Amount, Currency: in.Currency, Ref: in.Ref}, nil
}

type rig struct {
	t *testing.T
	h http.Handler
	p *fakeProvider
}

func newRig(t *testing.T) *rig {
	t.Helper()
	db := database.OpenTest(t, "users", "payments", "payment_refs", "payment_revocations")
	p := &fakeProvider{}
	h, err := newApp(db, []byte("test-secret"), "plans", p, "https://app.example/")
	if err != nil {
		t.Fatal(err)
	}
	return &rig{t, h, p}
}

func (g *rig) do(method, path, ctype, accept, body string, hdr map[string]string, c *http.Cookie) *httptest.ResponseRecorder {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	r := httptest.NewRequest(method, path, rd)
	if ctype != "" {
		r.Header.Set("Content-Type", ctype)
	}
	if accept != "" {
		r.Header.Set("Accept", accept)
	}
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	if c != nil {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	g.h.ServeHTTP(w, r)
	return w
}

type member struct {
	id     string
	cookie *http.Cookie
}

func (g *rig) signUp(email string) member {
	g.t.Helper()
	w := g.do("POST", "/register", "application/json", "", `{"email":"`+email+`","password":"hunter22"}`, nil, nil)
	var u struct{ ID string }
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &u) != nil {
		g.t.Fatalf("register: %d %s", w.Code, w.Body)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "session" {
			return member{u.ID, c}
		}
	}
	g.t.Fatal("no session cookie")
	return member{}
}

func (g *rig) app(m member) (int, string) {
	w := g.do("GET", "/app", "", "", "", nil, m.cookie)
	var out map[string]string
	json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out["plan"]
}

// webhook delivers a signed provider event.
func (g *rig) webhook(body string, signed bool) int {
	hdr := map[string]string{}
	if signed {
		hdr["X-Fake-Sig"] = "ok"
	}
	return g.do("POST", "/webhooks/fake", "application/json", "", body, hdr, nil).Code
}

func paid(slug string, amount int64, currency, ref string) string {
	b, _ := json.Marshal(map[string]any{"slug": slug, "amount": amount, "currency": currency, "ref": ref})
	return string(b)
}

func TestBuyingAPlanOpensTheAppForThatUserOnly(t *testing.T) {
	g := newRig(t)
	alice, bob := g.signUp("alice@example.com"), g.signUp("bob@example.com")

	if code, _ := g.app(alice); code != 402 {
		t.Fatalf("before paying /app = %d, want 402", code)
	}

	w := g.do("POST", "/items/pro/checkout", "", "", "", nil, alice.cookie)
	if w.Code != 303 || !strings.HasPrefix(w.Header().Get("Location"), "https://pay.example/session/") {
		t.Fatalf("checkout = %d -> %q", w.Code, w.Header().Get("Location"))
	}
	if len(g.p.checkouts) != 1 {
		t.Fatalf("provider saw %d checkouts", len(g.p.checkouts))
	}
	c := g.p.checkouts[0]
	if c.ItemSlug != "pro."+alice.id || c.PriceCents != 1900 || c.Currency != "usd" ||
		c.SuccessURL != "https://app.example/app?paid=1" || c.CancelURL != "https://app.example/items/pro" {
		t.Fatalf("checkout sent to provider: %+v", c)
	}

	if code := g.webhook(paid(c.ItemSlug, 1900, "usd", "pay_1"), true); code != 200 {
		t.Fatalf("webhook = %d", code)
	}
	if code, plan := g.app(alice); code != 200 || plan != "pro" {
		t.Fatalf("after paying alice /app = %d %q", code, plan)
	}
	if code, _ := g.app(bob); code != 402 {
		t.Fatalf("bob never paid but /app = %d", code)
	}
}

func TestWebhookDefences(t *testing.T) {
	g := newRig(t)
	alice := g.signUp("alice@example.com")
	slug := "pro." + alice.id

	for name, tc := range map[string]struct {
		body   string
		signed bool
		want   int
	}{
		"unsigned (forged)":                    {paid(slug, 1900, "usd", "p1"), false, 400},
		"underpaid":                            {paid(slug, 100, "usd", "p2"), true, 200},
		"wrong currency":                       {paid(slug, 1900, "eur", "p3"), true, 200},
		"plan we do not sell":                  {paid("gold."+alice.id, 1900, "usd", "p4"), true, 200},
		"no user part in the slug":             {paid("pro", 1900, "usd", "p5"), true, 200},
		"cheaper plan paid for the dearer one": {paid("team."+alice.id, 1900, "usd", "p6"), true, 200},
	} {
		if got := g.webhook(tc.body, tc.signed); got != tc.want {
			t.Errorf("%s: webhook = %d, want %d", name, got, tc.want)
		}
		if code, plan := g.app(alice); code != 402 {
			t.Fatalf("%s unlocked the app: /app = %d %q", name, code, plan)
		}
	}
}

func TestReplayDoesNotStackAndRefundCloses(t *testing.T) {
	g := newRig(t)
	alice := g.signUp("alice@example.com")
	body := paid("pro."+alice.id, 1900, "usd", "pay_9")
	for i := 0; i < 3; i++ { // provider retries
		if code := g.webhook(body, true); code != 200 {
			t.Fatalf("delivery %d = %d", i, code)
		}
	}
	if code, _ := g.app(alice); code != 200 {
		t.Fatalf("/app = %d after payment", code)
	}
	refund := `{"revoke":true,"ref":"pay_9"}`
	for i := 0; i < 2; i++ { // refund twice: idempotent
		if code := g.webhook(refund, true); code != 200 {
			t.Fatalf("refund %d = %d", i, code)
		}
	}
	if code, _ := g.app(alice); code != 402 {
		t.Fatalf("/app = %d after refund, want 402", code)
	}
}

func TestDearestPlanWins(t *testing.T) {
	g := newRig(t)
	m := g.signUp("alice@example.com")
	g.webhook(paid("team."+m.id, 4900, "usd", "t1"), true)
	g.webhook(paid("pro."+m.id, 1900, "usd", "p1"), true)
	if code, plan := g.app(m); code != 200 || plan != "team" {
		t.Fatalf("/app = %d %q, want team", code, plan)
	}
}

func TestCheckoutNeedsLoginAndAKnownPlan(t *testing.T) {
	g := newRig(t)
	if w := g.do("POST", "/items/pro/checkout", "", "", "", nil, nil); w.Code != 401 {
		t.Errorf("script without session = %d, want 401", w.Code)
	}
	w := g.do("POST", "/items/pro/checkout", "", "text/html", "", nil, nil)
	if w.Code != 303 || w.Header().Get("Location") != "/login?next="+url.QueryEscape("/items/pro") {
		t.Errorf("browser without session = %d -> %q", w.Code, w.Header().Get("Location"))
	}
	m := g.signUp("alice@example.com")
	if w := g.do("POST", "/items/nope/checkout", "", "", "", nil, m.cookie); w.Code != 404 {
		t.Errorf("unknown plan = %d, want 404", w.Code)
	}
	if len(g.p.checkouts) != 0 {
		t.Errorf("provider was called without a valid purchase: %+v", g.p.checkouts)
	}
}

func TestCatalogAndAppPagesForBrowsers(t *testing.T) {
	g := newRig(t)
	list := g.do("GET", "/items", "", "text/html", "", nil, nil).Body.String()
	if !strings.Contains(list, `href="/items/pro"`) || !strings.Contains(list, "19.00 USD") || !strings.Contains(list, "49.00 USD") {
		t.Fatalf("catalog: %s", list)
	}
	if d := g.do("GET", "/items/pro", "", "text/html", "", nil, nil).Body.String(); !strings.Contains(d, `action="/items/pro/checkout"`) {
		t.Fatalf("plan page has no Buy form: %s", d)
	}
	w := g.do("GET", "/app", "", "text/html", "", nil, nil)
	if w.Code != 303 || !strings.HasPrefix(w.Header().Get("Location"), "/login") {
		t.Fatalf("/app anonymous in a browser = %d -> %q", w.Code, w.Header().Get("Location"))
	}
	m := g.signUp("alice@example.com")
	w = g.do("GET", "/app", "", "text/html", "", nil, m.cookie)
	if w.Code != 402 || !strings.Contains(w.Body.String(), "Pick a plan") {
		t.Fatalf("/app without plan = %d %s", w.Code, w.Body)
	}
}
