package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"fastwebapp/kit/database"
)

func testApp(t *testing.T) http.Handler {
	t.Helper()
	db := database.OpenTest(t, "users", "chat_messages")
	h, err := newApp(db, []byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func do(h http.Handler, method, path, ctype, accept, body string, c *http.Cookie) *httptest.ResponseRecorder {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	r := httptest.NewRequest(method, path, rd)
	if ctype != "" {
		r.Header.Set("Content-Type", ctype)
	}
	if accept != "" {
		r.Header.Set("Accept", accept)
	}
	if c != nil {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func signUp(t *testing.T, h http.Handler, email string) *http.Cookie {
	t.Helper()
	w := do(h, "POST", "/register", "application/json", "", `{"email":"`+email+`","password":"hunter22"}`, nil)
	for _, c := range w.Result().Cookies() {
		if c.Name == "session" && w.Code == 200 {
			return c
		}
	}
	t.Fatalf("register %s: %d %s", email, w.Code, w.Body)
	return nil
}

func say(t *testing.T, h http.Handler, c *http.Cookie, room, text string) message {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"text": text})
	w := do(h, "POST", "/rooms/"+room+"/messages", "application/json", "", string(b), c)
	var m message
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &m) != nil || m.ID == "" {
		t.Fatalf("post: %d %s", w.Code, w.Body)
	}
	return m
}

func TestPostThenHistory(t *testing.T) {
	h := testApp(t)
	c := signUp(t, h, "alice@example.com")
	m := say(t, h, c, "general", "hello room")
	if m.Author != "alice" {
		t.Errorf("author = %q, want the part before the @ only", m.Author)
	}
	say(t, h, c, "general", "second")
	say(t, h, c, "other", "elsewhere")

	w := do(h, "GET", "/rooms/general", "", "", "", c)
	var out struct {
		Room     string
		Messages []message
	}
	if json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Room != "general" || len(out.Messages) != 2 ||
		out.Messages[0].Text != "hello room" || out.Messages[1].Text != "second" {
		t.Fatalf("history: %d %s", w.Code, w.Body)
	}
}

func TestLoginRequiredAndRoomNames(t *testing.T) {
	h := testApp(t)
	if w := do(h, "GET", "/rooms/general", "", "", "", nil); w.Code != 401 {
		t.Errorf("script without session = %d, want 401", w.Code)
	}
	w := do(h, "GET", "/rooms/general", "", "text/html", "", nil)
	if w.Code != 303 || w.Header().Get("Location") != "/login?next="+url.QueryEscape("/rooms/general") {
		t.Errorf("browser without session = %d -> %q", w.Code, w.Header().Get("Location"))
	}
	if w := do(h, "POST", "/rooms/general/messages", "application/json", "", `{"text":"x"}`, nil); w.Code != 401 {
		t.Errorf("anonymous post = %d, want 401", w.Code)
	}
	c := signUp(t, h, "bob@example.com")
	for _, bad := range []string{"UPPER", "has space", "a.b", strings.Repeat("a", 33)} {
		if w := do(h, "GET", "/rooms/"+url.PathEscape(bad), "", "", "", c); w.Code != 404 {
			t.Errorf("room %q = %d, want 404", bad, w.Code)
		}
	}
}

func TestPostValidation(t *testing.T) {
	h := testApp(t)
	c := signUp(t, h, "v@example.com")
	for name, body := range map[string]string{
		"empty":    `{"text":"   "}`,
		"too long": `{"text":"` + strings.Repeat("x", maxText+1) + `"}`,
		"not json": `nope`,
	} {
		if w := do(h, "POST", "/rooms/general/messages", "application/json", "", body, c); w.Code != 400 {
			t.Errorf("%s: %d, want 400", name, w.Code)
		}
	}
}

func TestHTMLFormFlowEscapes(t *testing.T) {
	h := testApp(t)
	c := signUp(t, h, "h@example.com")
	form := url.Values{"text": {`<script>alert(1)</script>`}}.Encode()
	w := do(h, "POST", "/rooms/general/messages", "application/x-www-form-urlencoded", "text/html", form, c)
	if w.Code != 303 || w.Header().Get("Location") != "/rooms/general" {
		t.Fatalf("form post: %d %q", w.Code, w.Header().Get("Location"))
	}
	page := do(h, "GET", "/rooms/general", "", "text/html", "", c).Body.String()
	if strings.Contains(page, "<script>alert(1)") || !strings.Contains(page, "&lt;script&gt;alert(1)") {
		t.Fatalf("message not escaped: %s", page)
	}
	if !strings.Contains(page, `new EventSource("/rooms/general/events?after=`) {
		t.Errorf("page lacks the live script")
	}
}

// sseLines reads an event stream line by line on a goroutine so a test can wait with a timeout.
func sseLines(resp *http.Response) <-chan string {
	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			ch <- sc.Text()
		}
	}()
	return ch
}

func nextData(t *testing.T, lines <-chan string) message {
	t.Helper()
	timeout := time.After(3 * time.Second)
	for {
		select {
		case l, ok := <-lines:
			if !ok {
				t.Fatal("stream closed")
			}
			if d, found := strings.CutPrefix(l, "data: "); found {
				var m message
				if err := json.Unmarshal([]byte(d), &m); err != nil {
					t.Fatal(err)
				}
				return m
			}
		case <-timeout:
			t.Fatal("no event within 3s")
		}
	}
}

func openEvents(t *testing.T, srv *httptest.Server, c *http.Cookie, path string, lastID string) <-chan string {
	t.Helper()
	req, _ := http.NewRequest("GET", srv.URL+path, nil)
	req.AddCookie(c)
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("events: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	return sseLines(resp)
}

func TestLiveDeliveryToOtherMember(t *testing.T) {
	h := testApp(t)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close) // runs after the stream bodies are closed (cleanups are LIFO); a deferred Close would wait for them forever
	alice, bob := signUp(t, h, "alice@example.com"), signUp(t, h, "bob@example.com")

	// returns once the stream is subscribed, so the post below cannot be missed
	lines := openEvents(t, srv, alice, "/rooms/general/events", "")
	say(t, h, bob, "other", "wrong room")
	sent := say(t, h, bob, "general", "hi alice")

	got := nextData(t, lines)
	if got.Text != "hi alice" || got.Author != "bob" || got.ID != sent.ID {
		t.Fatalf("got %+v, want bob's message %+v (the other room's message must not arrive)", got, sent)
	}
}

func TestReplayAfterLastEventID(t *testing.T) {
	h := testApp(t)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close) // runs after the stream bodies are closed (cleanups are LIFO); a deferred Close would wait for them forever
	c := signUp(t, h, "r@example.com")
	first := say(t, h, c, "general", "one")
	say(t, h, c, "general", "two")
	say(t, h, c, "general", "three")

	lines := openEvents(t, srv, c, "/rooms/general/events", first.ID)
	if m := nextData(t, lines); m.Text != "two" {
		t.Fatalf("first replayed = %q, want two", m.Text)
	}
	if m := nextData(t, lines); m.Text != "three" {
		t.Fatalf("second replayed = %q, want three", m.Text)
	}
	// ?after= does the same for the page's first connect
	lines = openEvents(t, srv, c, "/rooms/general/events?after="+first.ID, "")
	if m := nextData(t, lines); m.Text != "two" {
		t.Fatalf("?after replay = %q, want two", m.Text)
	}
}

func TestSlowListenerIsDroppedNotBlocking(t *testing.T) {
	r := &rooms{subs: map[string]map[chan message]bool{}}
	ch, cancel := r.subscribe("general")
	defer cancel()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ { // nobody reads ch
			r.publish("general", message{seq: int64(i)})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("publish blocked on a listener that does not read")
	}
	n := 0
	for range ch { // closed by publish, drains what was buffered
		n++
	}
	if n != 32 {
		t.Errorf("buffered %d messages before the drop, want 32", n)
	}
}

func TestMessageIDsAreStrictlyIncreasing(t *testing.T) {
	r := &rooms{}
	prev := int64(0)
	for i := 0; i < 10000; i++ {
		n := r.nextSeq()
		if n <= prev {
			t.Fatalf("seq %d after %d", n, prev)
		}
		prev = n
	}
}
