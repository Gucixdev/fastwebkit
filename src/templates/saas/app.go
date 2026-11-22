package main

import (
	"database/sql"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"

	"fastwebapp/kit/auth"
	"fastwebapp/kit/content"
	"fastwebapp/kit/payments"
	"fastwebapp/kit/ui"
)

// newApp composes: auth (accounts) + content (the plans, one Markdown file each) + payments (checkout,
// signed webhook, exactly-once ledger) + ui (pages). provider is injected so tests use a fake.
//
// Routes:
//
//	/register /login /logout        auth
//	GET /, /items, /items/{plan}    the plan catalog (content), each with a Buy button
//	POST /items/{plan}/checkout     signed-in users only: redirect to the provider's hosted checkout
//	POST /webhooks/<provider>       the provider's signed event: grants or revokes the plan
//	GET /app                        members only: 200 with the plan, 402 without one
//
// A payment is for one user: the item the provider sees is "<plan>.<user id>", so the ledger answers
// "did THIS user pay for THIS plan" and a refund closes only that user's access.
func newApp(db *sql.DB, secret []byte, plansDir string, provider payments.Provider, baseURL string) (http.Handler, error) {
	a, err := auth.New(db, secret)
	if err != nil {
		return nil, err
	}
	plans, err := content.Load(plansDir)
	if err != nil {
		return nil, err
	}
	ledger, err := payments.NewLedger(db)
	if err != nil {
		return nil, err
	}
	base := strings.TrimRight(baseURL, "/")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	a.Routes(mux)
	plans.Routes(mux)

	mux.HandleFunc("POST /items/{slug}/checkout", func(w http.ResponseWriter, r *http.Request) {
		plan, ok := plans.BySlug[r.PathValue("slug")]
		if !ok || plan.PriceCents <= 0 {
			http.NotFound(w, r)
			return
		}
		uid, ok := a.UserIDFromRequest(r)
		if !ok {
			deny(w, r, "/items/"+plan.Slug)
			return
		}
		c := payments.Checkout{
			ItemSlug: plan.Slug + "." + uid, ItemTitle: plan.Title, PriceCents: plan.PriceCents, Currency: strings.ToLower(plan.Currency),
			SuccessURL: base + "/app?paid=1", CancelURL: base + "/items/" + plan.Slug,
		}
		if err := c.Validate(); err != nil {
			log.Printf("saas: plan %s: %v", plan.Slug, err)
			http.Error(w, "this plan cannot be bought", http.StatusUnprocessableEntity)
			return
		}
		redirect, err := provider.CreateCheckout(c)
		if err != nil {
			log.Printf("saas: checkout %s: %v", c.ItemSlug, err) // provider errors can carry request detail
			http.Error(w, "payment provider unavailable", http.StatusBadGateway)
			return
		}
		http.Redirect(w, r, redirect, http.StatusSeeOther)
	})

	// The webhook asks for the price of "<plan>.<user id>": the plan part decides it.
	mux.HandleFunc("POST /webhooks/"+provider.Name(), payments.Handler(provider, ledger, func(slug string) (int64, string, bool) {
		i := strings.LastIndex(slug, ".")
		if i < 1 {
			return 0, "", false
		}
		plan, ok := plans.BySlug[slug[:i]]
		return plan.PriceCents, plan.Currency, ok
	}))

	mux.HandleFunc("GET /app", func(w http.ResponseWriter, r *http.Request) {
		uid, ok := a.UserIDFromRequest(r)
		if !ok {
			deny(w, r, "/app")
			return
		}
		var best *content.Item // the dearest plan this user has paid for
		for i := range plans.Items {
			it := &plans.Items[i]
			paid, err := ledger.Paid(it.Slug + "." + uid)
			if err != nil {
				log.Printf("saas: ledger: %v", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if paid && (best == nil || it.PriceCents > best.PriceCents) {
				best = it
			}
		}
		status := http.StatusOK
		if best == nil {
			status = http.StatusPaymentRequired
		}
		if !ui.WantsHTML(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			if best == nil {
				json.NewEncoder(w).Encode(map[string]string{"error": "payment required"})
			} else {
				json.NewEncoder(w).Encode(map[string]string{"plan": best.Slug})
			}
			return
		}
		var b strings.Builder
		_ = appTmpl.Execute(&b, best)
		ui.Page(w, status, "App", template.HTML(b.String()))
	})
	return mux, nil
}

var appTmpl = template.Must(template.New("app").Parse(`{{if .}}<h1>Welcome</h1><p>Your plan: <strong>{{.Title}}</strong>. This is where the product goes.</p>` +
	`{{else}}<h1>No plan yet</h1><p>This page is for members. <a href="/items">Pick a plan</a>.</p>{{end}}`))

// deny answers a request that needs a session: a login form for a browser, 401 for a script.
func deny(w http.ResponseWriter, r *http.Request, next string) {
	if ui.WantsHTML(r) {
		http.Redirect(w, r, "/login?next="+url.QueryEscape(next), http.StatusSeeOther)
		return
	}
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}
