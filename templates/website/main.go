// Command website serves a folder of Markdown files as a small site: pages in HTML for browsers, JSON for
// scripts, a sitemap and a health check. No database, no JavaScript, nothing to log in to.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fastwebapp/kit/ui"
)

func main() {
	ui.Site = envOr("SITE_NAME", "website")
	h, err := newApp(envOr("PAGES_DIR", "pages"), envOr("BASE_URL", "http://localhost:8080"))
	if err != nil {
		log.Fatalf("app: %v", err)
	}
	addr := envOr("ADDR", ":8080")
	srv := &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()
	log.Printf("listening on %s", addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
