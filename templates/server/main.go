// Command server is the smallest back end worth deploying: accounts (cookie session), a file store with
// .torrent/magnet links, a health check, panic recovery and access log. JSON in, JSON out; no pages of its own.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"fastwebapp/kit/database"
)

func main() {
	db, err := database.Open(envOr("DATABASE_URL", "server.db"))
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	secret := os.Getenv("SESSION_SECRET")
	if secret == "" {
		secret = randomHex(32)
		log.Println("SESSION_SECRET not set: sessions will not survive a restart")
	}
	maxMB, _ := strconv.ParseInt(envOr("MAX_UPLOAD_MB", "100"), 10, 64)
	h, err := newApp(db, []byte(secret), envOr("DOWNLOADS_DIR", "downloads"), maxMB<<20)
	if err != nil {
		log.Fatalf("app: %v", err)
	}
	serve(envOr("ADDR", ":8080"), h)
}

// serve runs h until SIGINT/SIGTERM, then lets in-flight requests finish (10 s).
func serve(addr string, h http.Handler) {
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

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("random: %v", err)
	}
	return hex.EncodeToString(b)
}
