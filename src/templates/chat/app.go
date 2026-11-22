package main

import (
	"database/sql"
	"net/http"

	"fastwebapp/kit/auth"
)

// newApp composes: auth (accounts) + database (history) + ui (pages), with the rooms on top.
func newApp(db *sql.DB, secret []byte) (http.Handler, error) {
	a, err := auth.New(db, secret)
	if err != nil {
		return nil, err
	}
	r, err := newRooms(db, a)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/rooms/general", http.StatusSeeOther)
	})
	a.Routes(mux)
	r.routes(mux)
	return mux, nil
}
