// Command fastwebapp is the framework's entrypoint: wires the database/content/payments/auth
// gems together on a stdlib net/http.ServeMux (Go 1.22+ method+wildcard routing, no router
// dependency). The checkout/webhook glue below is app-level - it's what an app built on
// fastwebapp writes, not something that belongs inside a gem.
package main

import (
	"crypto/rand"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"fastwebapp/gems/auth"
	"fastwebapp/gems/content"
	"fastwebapp/gems/database"
	"fastwebapp/gems/downloads"
	"fastwebapp/gems/payments"
)

func main() {
	dsn := envOr("DATABASE_URL", "fastwebapp.db")
	db, err := database.Open(dsn)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	sessionSecret := []byte(envOr("SESSION_SECRET", randomSecret()))
	auther, err := auth.New(db, sessionSecret)
	if err != nil {
		log.Fatalf("auth: %v", err)
	}

	catalog, err := content.Load("gems/content/items")
	if err != nil {
		log.Fatalf("content: %v", err)
	}

	provider := selectPaymentProvider()
	ledger, err := payments.NewLedger(db)
	if err != nil {
		log.Fatalf("payments: %v", err)
	}
	baseURL := envOr("BASE_URL", "http://localhost:8080")

	mux := http.NewServeMux()
	auther.Routes(mux)
	catalog.Routes(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	// Backs the upload gem's UploadZone (gems/upload/upload.ts), which posts multipart/form-data
	// via XHR, through the downloads gem: every upload gets a stable file URL plus a generated
	// .torrent/magnet so it can be grabbed over BitTorrent too, not just HTTP. Supersedes the
	// earlier bare-bones /uploads handler (filepath.Base + random-prefix naming) - downloads
	// never derives a filesystem path from the client's filename at all, which is strictly safer.
	var trackers []string
	if v := os.Getenv("DOWNLOAD_TRACKERS"); v != "" {
		trackers = strings.Split(v, ",")
	}
	downloader, err := downloads.New(db, envOr("DOWNLOADS_DIR", "downloads"), trackers)
	if err != nil {
		log.Fatalf("downloads: %v", err)
	}
	downloader.Routes(mux)

	// Checkout and the payment webhook are app glue: they reach into both the content catalog
	// and the chosen payments.Provider, which is exactly the kind of wiring a gem shouldn't do
	// on an app's behalf.
	mux.HandleFunc("POST /items/{slug}/checkout", func(w http.ResponseWriter, r *http.Request) {
		item, ok := catalog.BySlug[r.PathValue("slug")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		url, err := provider.CreateCheckout(payments.Checkout{
			ItemSlug:   item.Slug,
			ItemTitle:  item.Title,
			PriceCents: item.PriceCents,
			Currency:   item.Currency,
			SuccessURL: baseURL + "/items/" + item.Slug + "?paid=1",
			CancelURL:  baseURL + "/items/" + item.Slug,
		})
		if err != nil {
			log.Printf("checkout %s: %v", item.Slug, err) // provider errors can carry request detail
			http.Error(w, "payment provider unavailable", http.StatusBadGateway)
			return
		}
		http.Redirect(w, r, url, http.StatusSeeOther)
	})
	mux.HandleFunc("POST /webhooks/"+provider.Name(), webhookHandler(provider, ledger, func(slug string) (int64, string, bool) {
		item, ok := catalog.BySlug[slug]
		return item.PriceCents, item.Currency, ok
	}))

	addr := envOr("ADDR", ":8080")
	log.Printf("fastwebapp listening on %s (db=%s, payments=%s)", addr, dsn, provider.Name())
	log.Fatal(http.ListenAndServe(addr, mux))
}

// selectPaymentProvider picks Stripe, PayPal or BTCPay crypto by which env vars are set, in
// that order - the first one whose required vars are present wins. Falls back to a disabled
// Stripe provider (keys empty) so the app still boots with a clear per-request error instead
// of refusing to start.
func selectPaymentProvider() payments.Provider {
	switch {
	case os.Getenv("STRIPE_API_KEY") != "":
		return payments.NewStripe(os.Getenv("STRIPE_API_KEY"), os.Getenv("STRIPE_WEBHOOK_SECRET"))
	case os.Getenv("PAYPAL_CLIENT_ID") != "":
		p, err := payments.NewPayPal(
			os.Getenv("PAYPAL_CLIENT_ID"),
			os.Getenv("PAYPAL_SECRET"),
			os.Getenv("PAYPAL_WEBHOOK_ID"),
			envOr("PAYPAL_SANDBOX", "true") == "true",
		)
		if err != nil {
			log.Fatalf("paypal: %v", err)
		}
		return p
	case os.Getenv("BTCPAY_SERVER_URL") != "":
		return payments.NewCrypto(
			os.Getenv("BTCPAY_SERVER_URL"),
			os.Getenv("BTCPAY_STORE_ID"),
			os.Getenv("BTCPAY_API_KEY"),
			os.Getenv("BTCPAY_WEBHOOK_SECRET"),
		)
	default:
		log.Println("no payment provider configured (STRIPE_API_KEY / PAYPAL_CLIENT_ID / BTCPAY_SERVER_URL) - checkout will fail until one is")
		return payments.NewStripe("", "")
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func randomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("generate session secret: %v", err)
	}
	return fmt.Sprintf("%x", b)
}
