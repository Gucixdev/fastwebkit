// Command chat is a small multi-room chat: sign up, open /rooms/<name>, post, and everyone in the room sees it
// live (server-sent events, no websocket library). History is kept in the database. Pages work without
// JavaScript (reload to see new messages); one inline script upgrades the room page to live updates.
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
	"syscall"
	"time"

	"fastwebapp/kit/database"
	"fastwebapp/kit/ui"
)

func main() {
	ui.Site = envOr("SITE_NAME", "chat")
	db, err := database.Open(envOr("DATABASE_URL", "chat.db"))
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	secret := os.Getenv("SESSION_SECRET")
	if secret == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			log.Fatalf("random: %v", err)
		}
		secret = hex.EncodeToString(b)
		log.Println("SESSION_SECRET not set: sessions will not survive a restart")
	}
	h, err := newApp(db, []byte(secret))
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
