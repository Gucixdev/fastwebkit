# auth

Accounts: a `users` table, bcrypt passwords, signed-cookie sessions (userID + expiry, HMAC-signed, no session
table). Login and register come as forms for browsers and as JSON for scripts on the same routes.

Depends on: `database` (`Migrate`, `Rebind`), `ui` (the forms). Used by: every template that has users.

## Interface

    auth.New(db, secret) (*Auth, error)       // runs the users migration; changing secret ends every session
    a.Register(email, password) (User, error) // ErrEmailTaken
    a.Login(email, password) (User, error)    // ErrInvalidLogin (same error for unknown e-mail and bad password)
    a.UserByID(id) (User, error)              // ErrNoUser; for pages that show who did something
    a.UserIDFromRequest(r) (id, ok)
    a.RequireUser(next) http.HandlerFunc      // 401 without a session
    a.Routes(mux)                             // GET/POST /register, GET/POST /login, POST /logout

A form POST sets the cookie and redirects to `next` (a local path only; `//host` or backslash tricks become `/`).
A JSON POST answers the user as JSON. Wrong login: 401 as text for JSON clients, the form again for browsers.

## Files

    auth.go                 the whole module
    auth_test.go            register/login/UserByID
    handlers_test.go        forms, redirects, JSON
    session_test.go         cookie forgery, expiry

## Not done

- No password reset, no e-mail verification, no rate limiting on login (put one in front), no roles.
- The cookie has no `Secure` flag; set it at the reverse proxy or add it when you serve over HTTPS only.
- `users.email` is stored as typed (case-sensitive).

## Test

    go test ./kit/auth
