// auth is the login/register module: users table, bcrypt passwords, signed-cookie
// sessions. A standalone module - it only needs a *sql.DB (from database), nothing shop- or
// app-specific leaks in here.

package kit

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailTaken   = errors.New("auth: email already registered")
	ErrInvalidLogin = errors.New("auth: invalid email or password")
)

type User struct {
	ID        string
	Email     string
	CreatedAt time.Time
}

type Auth struct {
	db     *sql.DB
	cookie signedCookie
}

const authSchema = `CREATE TABLE IF NOT EXISTS users (
	id TEXT PRIMARY KEY,
	email TEXT UNIQUE NOT NULL,
	password_hash TEXT NOT NULL,
	created_at TEXT NOT NULL
)`

// New runs the users table migration and returns an Auth bound to db. Works the same against
// SQLite or PostgreSQL - the schema avoids anything dialect-specific (no SERIAL/AUTOINCREMENT,
// no native TIMESTAMP type). secret signs session cookies - any change to it invalidates every
// existing session.
func AuthNew(db *sql.DB, secret []byte) (*Auth, error) {
	if err := Migrate(db, authSchema); err != nil {
		return nil, err
	}
	return &Auth{db: db, cookie: signedCookie{secret: secret}}, nil
}

func (a *Auth) Register(email, password string) (User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, fmt.Errorf("auth: hash password: %w", err)
	}

	u := User{ID: uuid.NewString(), Email: email, CreatedAt: time.Now()}
	_, err = a.db.Exec(
		Rebind(a.db, `INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, ?, ?)`),
		u.ID, u.Email, string(hash), u.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		// sqlite and pgx report a unique-violation with different driver error types, but both
		// surface the constraint name in the error text - good enough without parsing driver-
		// specific error codes for a single-column UNIQUE check.
		if isUniqueViolation(err) {
			return User{}, ErrEmailTaken
		}
		return User{}, fmt.Errorf("auth: insert user: %w", err)
	}
	return u, nil
}

func (a *Auth) Login(email, password string) (User, error) {
	var u User
	var hash, createdAt string
	err := a.db.QueryRow(
		Rebind(a.db, `SELECT id, email, password_hash, created_at FROM users WHERE email = ?`), email,
	).Scan(&u.ID, &u.Email, &hash, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrInvalidLogin
	}
	if err != nil {
		return User{}, fmt.Errorf("auth: query user: %w", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return User{}, ErrInvalidLogin
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return u, nil
}

func isUniqueViolation(err error) bool {
	msg := err.Error()
	return contains(msg, "UNIQUE constraint") || contains(msg, "unique constraint") || contains(msg, "duplicate key") || contains(msg, "Duplicate entry")
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
