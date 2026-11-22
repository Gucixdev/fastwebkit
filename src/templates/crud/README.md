# crud

The pattern for "a table, a form, a list": one resource (notes) that belongs to its owner, as JSON for scripts and as
pages for browsers on the same routes. Copy `resource.go`, rename the table and the fields, and you have the next resource.

Modules: `auth`, `database`, `ui`.

## Run

    ./run                     # :8080, SQLite file crud.db

Environment: `ADDR` (`:8080`), `DATABASE_URL` (`crud.db`), `SESSION_SECRET` (random per start if unset), `SITE_NAME`.

## Routes

    /register /login /logout        auth (forms and JSON)
    GET    /notes                   your notes: HTML with an add form, or JSON
    POST   /notes                   create: JSON {"title","body"} or the form
    GET    /notes/{id}              one note (JSON)
    PUT    /notes/{id}              change it (JSON)
    DELETE /notes/{id}              delete it (JSON, 204)
    POST   /notes/{id}/delete       delete from the HTML page
    GET    /healthz

Somebody else's note id is a 404, never a 403, so ids cannot be probed. Without a session a script gets 401 and a
browser is sent to `/login?next=...`. Title is required (200 characters), body is capped at 20000 bytes.

## Files

    main.go       config, graceful shutdown
    app.go        newApp: auth + notes
    resource.go   the resource: table, handlers, page
    app_test.go   httptest: lifecycle, isolation between two users, validation, HTML flow and escaping

## Test

    go test ./templates/crud

Not verified: Postgres/MySQL as the backend (the SQL goes through `database.Rebind`, but only SQLite ran).
No paging: a list returns every note of the user.
