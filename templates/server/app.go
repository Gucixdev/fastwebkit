package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"fastwebapp/kit/auth"
	"fastwebapp/kit/downloads"
)

// newApp composes the modules: auth (accounts) + downloads (files). Reading files is public, uploading
// needs a session and is capped at maxUpload bytes.
func newApp(db *sql.DB, secret []byte, dir string, maxUpload int64) (http.Handler, error) {
	a, err := auth.New(db, secret)
	if err != nil {
		return nil, err
	}
	d, err := downloads.New(db, dir, nil)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.PingContext(r.Context()); err != nil {
			http.Error(w, "database down", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	})
	a.Routes(mux)
	mux.HandleFunc("GET /me", a.RequireUser(func(w http.ResponseWriter, r *http.Request) {
		id, _ := a.UserIDFromRequest(r)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": id})
	}))

	// downloads has no auth of its own (see its README): gate the write side here.
	files := http.NewServeMux()
	d.Routes(files)
	mux.HandleFunc("POST /downloads", a.RequireUser(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
		files.ServeHTTP(w, r)
	}))
	for _, p := range []string{"GET /downloads", "GET /downloads/{id}/file", "GET /downloads/{id}/torrent"} {
		mux.Handle(p, files)
	}
	return recoverer(accessLog(mux)), nil
}

// recoverer turns a panic in a handler into a 500 instead of a dropped connection.
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Printf("panic %s %s: %v", r.Method, r.URL.Path, v)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (s *statusWriter) WriteHeader(c int) { s.code = c; s.ResponseWriter.WriteHeader(c) }

// Unwrap lets http.ResponseController reach the real writer (Flush, deadlines).
func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.code, time.Since(start).Round(time.Millisecond))
	})
}
