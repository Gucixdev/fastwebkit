# chat

A small multi-room chat. Sign up, open `/rooms/<name>`, post, and everyone in the room sees it live. History is in
the database. The page works without JavaScript (reload to see new messages); one inline script upgrades it to live
updates with `EventSource`, so there is no websocket library.

Modules: `auth`, `database`, `ui`. The TypeScript `chat` module (message tree, end-to-end encryption) is not used here:
this template stores plain text.

## Run

    ./run                     # :8080, SQLite file chat.db, open http://localhost:8080/

Environment: `ADDR` (`:8080`), `DATABASE_URL` (`chat.db`), `SESSION_SECRET` (random per start if unset), `SITE_NAME`.

## Routes (all need a session; a browser is sent to the login form)

    GET  /                          redirect to /rooms/general
    GET  /rooms/{room}              last 50 messages: HTML, or JSON {"room","messages"}
    POST /rooms/{room}/messages     {"text"} as JSON, or the form field "text"; max 2000 bytes
    GET  /rooms/{room}/events       server-sent events; Last-Event-ID or ?after=<id> replays what was missed

Room names are `a-z0-9-`, 1 to 32 characters; a room exists once someone posts. The author shown is the part of the
e-mail before the `@`, visible to every member.

## How it behaves

- Message ids are a per-process nanosecond clock (strictly increasing, sent as strings). One process only: the
  live fan-out is in memory.
- A listener that falls 32 messages behind is dropped and reconnects by itself; the replay fills the gap (the last 500).
- Posting never waits for slow readers.

## Files

    main.go       config, graceful shutdown
    app.go        newApp: auth + rooms
    rooms.go      storage, fan-out, the three handlers, the page
    app_test.go   httptest: history, login gate, validation, escaping, live delivery, replay, slow listener

## Test

    go test -race ./templates/chat

Not verified: a real browser's EventSource, a reverse proxy that buffers (the header `X-Accel-Buffering: no` is set),
more than one process, Postgres/MySQL as the backend.
