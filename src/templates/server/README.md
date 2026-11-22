# server

The smallest back end worth deploying. JSON in, JSON out, no pages of its own.

Modules: `auth`, `downloads`, `database`.

## Run

    ./run                     # :8080, SQLite file server.db, files in ./downloads

Environment: `ADDR` (`:8080`), `DATABASE_URL` (`server.db`; `postgres://...` or `mysql://...` also work),
`SESSION_SECRET` (random per start if unset, so sessions end at restart), `DOWNLOADS_DIR`, `MAX_UPLOAD_MB` (100).

## Routes

    GET  /healthz                      "ok", or 503 when the database does not answer
    POST /register  /login  /logout    JSON {"email","password"}; sets the session cookie
    GET  /me                           {"id"} of the signed-in user, 401 without a session
    POST /downloads                    multipart field "file"; needs a session, capped at MAX_UPLOAD_MB
    GET  /downloads                    list (public)
    GET  /downloads/{id}/file          the file (public)
    GET  /downloads/{id}/torrent       .torrent (public)

Every request is logged (method, path, status, time); a panic in a handler becomes a 500.

## Try it

    curl -c jar -H 'Content-Type: application/json' -d '{"email":"a@b.c","password":"hunter22"}' localhost:8080/register
    curl -b jar -F file=@photo.jpg localhost:8080/downloads
    curl localhost:8080/downloads

## Files

    main.go       config from the environment, graceful shutdown
    app.go        newApp: the composition, plus the two middlewares
    app_test.go   httptest: health, session gate, upload, size cap, panic

## Test

    go test ./templates/server

Not verified: a real browser session, Postgres/MySQL as the backend, uploads over 32 MB (larger bodies spill to disk).
