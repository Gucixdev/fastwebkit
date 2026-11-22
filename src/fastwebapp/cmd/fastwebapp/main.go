// Command fastwebapp is the full demo: wires the database/content/payments/auth/downloads
// modules (kit/<module>) together on a stdlib net/http.ServeMux (Go 1.22+ method+wildcard routing, no router
// dependency). The checkout/webhook glue below is app-level - it's what an app built on
// fastwebapp writes, not something that belongs inside a gem.
package main

import (
	"crypto/rand"
	"fastwebapp/kit/auth"
	"fastwebapp/kit/content"
	"fastwebapp/kit/database"
	"fastwebapp/kit/downloads"
	"fastwebapp/kit/payments"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

func main() {
	dsn := envOr("DATABASE_URL", "fastwebapp.db")
	db, err := database.Open(dsn)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	sessionSecret := []byte(envOr("SESSION_SECRET", randomSecret()))
	authn, err := auth.New(db, sessionSecret)
	if err != nil {
		log.Fatalf("auth: %v", err)
	}

	catalog, err := content.Load(envOr("CONTENT_DIR", "../kit/content/items"))
	if err != nil {
		log.Fatalf("content: %v", err)
	}

	mux := http.NewServeMux()
	authn.Routes(mux)
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

	// Payments by convention: provider from env, POST /items/{slug}/checkout and POST /webhooks/<provider>.
	if _, err := payments.Mount(mux, db, func(slug string) (payments.Item, bool) {
		it, ok := catalog.BySlug[slug]
		return payments.Item{Slug: it.Slug, Title: it.Title, PriceCents: it.PriceCents, Currency: it.Currency}, ok
	}); err != nil {
		log.Fatalf("payments: %v", err)
	}

	addr := envOr("ADDR", ":8080")
	log.Printf("fastwebapp listening on %s (db=%s)", addr, dsn)
	log.Fatal(http.ListenAndServe(addr, mux))
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
