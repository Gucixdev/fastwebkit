package kit

import (
	"encoding/json"
	"errors"
	"net/http"
)

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Routes registers /register, /login, /logout. Mount it under whatever prefix the app wants.
func (a *Auth) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /register", a.handleRegister)
	mux.HandleFunc("POST /login", a.handleLogin)
	mux.HandleFunc("POST /logout", a.handleLogout)
}

func (a *Auth) handleRegister(w http.ResponseWriter, r *http.Request) {
	var creds credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	user, err := a.Register(creds.Email, creds.Password)
	if errors.Is(err, ErrEmailTaken) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	a.setSessionCookie(w, user.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

func (a *Auth) handleLogin(w http.ResponseWriter, r *http.Request) {
	var creds credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	user, err := a.Login(creds.Email, creds.Password)
	if errors.Is(err, ErrInvalidLogin) {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	a.setSessionCookie(w, user.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

func (a *Auth) handleLogout(w http.ResponseWriter, r *http.Request) {
	a.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}
