package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"fastwebapp/kit/auth"
	"fastwebapp/kit/database"
	"fastwebapp/kit/ui"
)

// The resource: a note belongs to its owner and nobody else can see or touch it (other people's ids are 404).
//
//	GET    /notes              list (HTML for a browser, JSON for a script)
//	POST   /notes              create (JSON or form)
//	GET    /notes/{id}         one note (JSON)
//	PUT    /notes/{id}         change title/body (JSON)
//	DELETE /notes/{id}         delete (JSON)
//	POST   /notes/{id}/delete  delete from the HTML page
type note struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
}

type notes struct {
	db   *sql.DB
	auth *auth.Auth
}

const (
	maxTitle = 200
	maxBody  = 20000
)

func newNotes(db *sql.DB, a *auth.Auth) (*notes, error) {
	err := database.Migrate(db, `CREATE TABLE IF NOT EXISTS notes (
		id         VARCHAR(36)  PRIMARY KEY,
		owner_id   VARCHAR(191) NOT NULL,
		title      VARCHAR(255) NOT NULL,
		body       TEXT         NOT NULL,
		created_at VARCHAR(40)  NOT NULL
	)`)
	if err != nil {
		return nil, err
	}
	return &notes{db: db, auth: a}, nil
}

func (n *notes) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /notes", n.signedIn(n.list))
	mux.HandleFunc("POST /notes", n.signedIn(n.create))
	mux.HandleFunc("GET /notes/{id}", n.signedIn(n.get))
	mux.HandleFunc("PUT /notes/{id}", n.signedIn(n.update))
	mux.HandleFunc("DELETE /notes/{id}", n.signedIn(n.remove))
	mux.HandleFunc("POST /notes/{id}/delete", n.signedIn(n.remove))
}

// signedIn runs h with the owner's id, or answers 401 to a script and sends a browser to the login form.
func (n *notes) signedIn(h func(w http.ResponseWriter, r *http.Request, owner string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := n.auth.UserIDFromRequest(r)
		if !ok {
			if ui.WantsHTML(r) && r.Method == http.MethodGet {
				http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.Path), http.StatusSeeOther)
				return
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h(w, r, owner)
	}
}

func (n *notes) list(w http.ResponseWriter, r *http.Request, owner string) {
	rows, err := n.db.Query(database.Rebind(n.db, `SELECT id, title, body, created_at FROM notes WHERE owner_id = ? ORDER BY created_at DESC, id`), owner)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()
	out := []note{}
	for rows.Next() {
		var x note
		if err := rows.Scan(&x.ID, &x.Title, &x.Body, &x.CreatedAt); err != nil {
			serverError(w, err)
			return
		}
		out = append(out, x)
	}
	if rows.Err() != nil {
		serverError(w, rows.Err())
		return
	}
	if ui.WantsHTML(r) {
		var b strings.Builder
		_ = listTmpl.Execute(&b, out)
		ui.Page(w, http.StatusOK, "Notes", template.HTML(b.String()))
		return
	}
	writeJSON(w, http.StatusOK, out)
}

var listTmpl = template.Must(template.New("notes").Parse(`<h1>Notes</h1>
<form method="post" action="/notes">
<p><input name="title" placeholder="Title" required maxlength="200"></p>
<p><input name="body" placeholder="Body"></p>
<p><button type="submit">Add</button></p>
</form>
<ul class="cards">{{range .}}<li><span><strong>{{.Title}}</strong> {{.Body}}</span>
<form method="post" action="/notes/{{.ID}}/delete"><button type="submit">Delete</button></form></li>{{else}}<li>No notes yet.</li>{{end}}</ul>`))

// input reads title and body from a form or from JSON.
func input(r *http.Request) (title, body string, err error) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		if err = r.ParseForm(); err != nil {
			return
		}
		title, body = r.PostFormValue("title"), r.PostFormValue("body")
	} else {
		var in struct{ Title, Body string }
		if err = json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(&in); err != nil {
			return
		}
		title, body = in.Title, in.Body
	}
	title = strings.TrimSpace(title)
	if title == "" || len(title) > maxTitle || len(body) > maxBody {
		err = errors.New("title is required (max 200 chars), body max 20000")
	}
	return
}

func (n *notes) create(w http.ResponseWriter, r *http.Request, owner string) {
	title, body, err := input(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	x := note{ID: uuid.NewString(), Title: title, Body: body, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if _, err := n.db.Exec(database.Rebind(n.db, `INSERT INTO notes (id, owner_id, title, body, created_at) VALUES (?, ?, ?, ?, ?)`),
		x.ID, owner, x.Title, x.Body, x.CreatedAt); err != nil {
		serverError(w, err)
		return
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		http.Redirect(w, r, "/notes", http.StatusSeeOther)
		return
	}
	writeJSON(w, http.StatusCreated, x)
}

func (n *notes) get(w http.ResponseWriter, r *http.Request, owner string) {
	var x note
	err := n.db.QueryRow(database.Rebind(n.db, `SELECT id, title, body, created_at FROM notes WHERE id = ? AND owner_id = ?`),
		r.PathValue("id"), owner).Scan(&x.ID, &x.Title, &x.Body, &x.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, x)
}

func (n *notes) update(w http.ResponseWriter, r *http.Request, owner string) {
	title, body, err := input(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := n.db.Exec(database.Rebind(n.db, `UPDATE notes SET title = ?, body = ? WHERE id = ? AND owner_id = ?`),
		title, body, r.PathValue("id"), owner)
	if err != nil {
		serverError(w, err)
		return
	}
	if c, _ := res.RowsAffected(); c == 0 {
		http.NotFound(w, r)
		return
	}
	n.get(w, r, owner)
}

func (n *notes) remove(w http.ResponseWriter, r *http.Request, owner string) {
	res, err := n.db.Exec(database.Rebind(n.db, `DELETE FROM notes WHERE id = ? AND owner_id = ?`), r.PathValue("id"), owner)
	if err != nil {
		serverError(w, err)
		return
	}
	if c, _ := res.RowsAffected(); c == 0 {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodPost { // the HTML page's delete button
		http.Redirect(w, r, "/notes", http.StatusSeeOther)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func serverError(w http.ResponseWriter, err error) {
	// the cause goes to the log, not to the client
	log.Printf("crud: %v", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
