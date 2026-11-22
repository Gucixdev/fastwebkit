package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"fastwebapp/kit/auth"
	"fastwebapp/kit/database"
	"fastwebapp/kit/ui"
)

// Rooms, all behind a login:
//
//	GET  /rooms/{room}            last 50 messages (HTML for a browser, JSON {"room","messages"} for a script)
//	POST /rooms/{room}/messages   post {"text"} as JSON or a form field "text"
//	GET  /rooms/{room}/events     server-sent events; "Last-Event-ID" or ?after=<id> replays what was missed
//
// A room exists as soon as someone posts to it. Names are a-z, 0-9 and "-", 1 to 32 characters.
// One process only: the live fan-out is in memory and message ids are a per-process clock.
type message struct {
	ID     string `json:"id"` // a string: the id is a 64-bit clock, too wide for a JSON number in JavaScript
	Author string `json:"author"`
	Text   string `json:"text"`
	At     string `json:"at"`
	seq    int64
}

const maxText = 2000

var roomName = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

type rooms struct {
	db   *sql.DB
	auth *auth.Auth

	mu   sync.Mutex
	last int64                            // last issued id
	subs map[string]map[chan message]bool // room -> live listeners
}

func newRooms(db *sql.DB, a *auth.Auth) (*rooms, error) {
	err := database.Migrate(db, `CREATE TABLE IF NOT EXISTS chat_messages (
		seq     BIGINT       PRIMARY KEY,
		room    VARCHAR(32)  NOT NULL,
		author  VARCHAR(255) NOT NULL,
		body    TEXT         NOT NULL
	)`)
	if err != nil {
		return nil, err
	}
	return &rooms{db: db, auth: a, subs: map[string]map[chan message]bool{}}, nil
}

func (c *rooms) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /rooms/{room}", c.signedIn(c.page))
	mux.HandleFunc("POST /rooms/{room}/messages", c.signedIn(c.post))
	mux.HandleFunc("GET /rooms/{room}/events", c.signedIn(c.events))
}

func (c *rooms) signedIn(h func(w http.ResponseWriter, r *http.Request, userID string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !roomName.MatchString(r.PathValue("room")) {
			http.NotFound(w, r)
			return
		}
		id, ok := c.auth.UserIDFromRequest(r)
		if !ok {
			if ui.WantsHTML(r) && r.Method == http.MethodGet {
				http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.Path), http.StatusSeeOther)
				return
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h(w, r, id)
	}
}

// load returns up to limit messages of room with id > after, oldest first.
func (c *rooms) load(room string, after int64, limit int) ([]message, error) {
	rows, err := c.db.Query(database.Rebind(c.db, `SELECT seq, author, body FROM (
		SELECT seq, author, body FROM chat_messages WHERE room = ? AND seq > ? ORDER BY seq DESC LIMIT ?) t ORDER BY seq`), room, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []message{}
	for rows.Next() {
		var m message
		if err := rows.Scan(&m.seq, &m.Author, &m.Text); err != nil {
			return nil, err
		}
		m.fill()
		out = append(out, m)
	}
	return out, rows.Err()
}

func (m *message) fill() {
	m.ID = strconv.FormatInt(m.seq, 10)
	m.At = time.Unix(0, m.seq).UTC().Format(time.RFC3339)
}

func (c *rooms) page(w http.ResponseWriter, r *http.Request, _ string) {
	room := r.PathValue("room")
	msgs, err := c.load(room, 0, 50)
	if err != nil {
		log.Printf("chat: load: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ui.WantsHTML(r) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"room": room, "messages": msgs})
		return
	}
	last := "0"
	if len(msgs) > 0 {
		last = msgs[len(msgs)-1].ID
	}
	var b strings.Builder
	_ = pageTmpl.Execute(&b, struct {
		Room, Last string
		Messages   []message
	}{room, last, msgs})
	ui.Page(w, http.StatusOK, "#"+room, template.HTML(b.String()))
}

// The script is optional: without it the form still posts and a reload shows the new message.
var pageTmpl = template.Must(template.New("room").Parse(`<h1>#{{.Room}}</h1>
<ul class="cards" id="log" data-last="{{.Last}}">{{range .Messages}}<li><span><strong>{{.Author}}</strong> {{.Text}}</span></li>{{end}}</ul>
<form method="post" action="/rooms/{{.Room}}/messages">
<p><input name="text" required maxlength="2000" autofocus autocomplete="off"> <button type="submit">Send</button></p>
</form>
<script>
(function(){var log=document.getElementById("log");
var es=new EventSource("/rooms/{{.Room}}/events?after="+log.dataset.last);
es.onmessage=function(e){var m=JSON.parse(e.data),li=document.createElement("li"),s=document.createElement("span"),b=document.createElement("strong");
b.textContent=m.author;s.appendChild(b);s.appendChild(document.createTextNode(" "+m.text));li.appendChild(s);log.appendChild(li);window.scrollTo(0,document.body.scrollHeight);};})();
</script>`))

func (c *rooms) post(w http.ResponseWriter, r *http.Request, userID string) {
	isForm := strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded")
	var text string
	if isForm {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		text = r.PostFormValue("text")
	} else {
		var in struct{ Text string }
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		text = in.Text
	}
	text = strings.TrimSpace(text)
	if text == "" || len(text) > maxText {
		http.Error(w, "text is required, max 2000 bytes", http.StatusBadRequest)
		return
	}
	u, err := c.auth.UserByID(userID)
	if err != nil { // a valid cookie for a deleted account
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	room := r.PathValue("room")
	m := message{Author: displayName(u.Email), Text: text, seq: c.nextSeq()}
	m.fill()
	if _, err := c.db.Exec(database.Rebind(c.db, `INSERT INTO chat_messages (seq, room, author, body) VALUES (?, ?, ?, ?)`),
		m.seq, room, m.Author, m.Text); err != nil {
		log.Printf("chat: insert: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	c.publish(room, m)
	if isForm {
		http.Redirect(w, r, "/rooms/"+room, http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(m)
}

// displayName is the part of the e-mail before the "@": other members see it, never the domain.
func displayName(email string) string {
	name, _, _ := strings.Cut(email, "@")
	if name == "" {
		return "anon"
	}
	return name
}

// nextSeq is the message id: wall clock in nanoseconds, bumped by one when two messages land in the same tick.
func (c *rooms) nextSeq() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := time.Now().UnixNano()
	if n <= c.last {
		n = c.last + 1
	}
	c.last = n
	return n
}

func (c *rooms) subscribe(room string) (chan message, func()) {
	ch := make(chan message, 32)
	c.mu.Lock()
	if c.subs[room] == nil {
		c.subs[room] = map[chan message]bool{}
	}
	c.subs[room][ch] = true
	c.mu.Unlock()
	return ch, func() {
		c.mu.Lock()
		if c.subs[room][ch] {
			delete(c.subs[room], ch)
			close(ch)
		}
		c.mu.Unlock()
	}
}

// publish never blocks the poster: a listener that is 32 messages behind is dropped and reconnects (EventSource
// does that by itself and replays from Last-Event-ID).
func (c *rooms) publish(room string, m message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for ch := range c.subs[room] {
		select {
		case ch <- m:
		default:
			delete(c.subs[room], ch)
			close(ch)
		}
	}
}

func (c *rooms) events(w http.ResponseWriter, r *http.Request, _ string) {
	room := r.PathValue("room")
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	if h := r.Header.Get("Last-Event-ID"); h != "" {
		after, _ = strconv.ParseInt(h, 10, 64)
	}
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	// subscribe before reading history, so nothing posted in between is lost; the seq check drops duplicates
	ch, cancel := c.subscribe(room)
	defer cancel()
	backlog, err := c.load(room, after, 500)
	if err != nil {
		log.Printf("chat: replay: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	last := after
	send := func(m message) error {
		b, _ := json.Marshal(m)
		if _, err := fmt.Fprintf(w, "id: %s\ndata: %s\n\n", m.ID, b); err != nil {
			return err
		}
		last = m.seq
		return rc.Flush()
	}
	w.WriteHeader(http.StatusOK)
	for _, m := range backlog {
		if send(m) != nil {
			return
		}
	}
	if rc.Flush() != nil {
		return
	}
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case m, ok := <-ch:
			if !ok {
				return
			}
			if m.seq > last && send(m) != nil {
				return
			}
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		}
	}
}
