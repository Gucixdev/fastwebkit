package auth

import (
	"errors"
	"testing"

	"fastwebapp/kit/database"
)

func newTestAuth(t *testing.T) *Auth {
	t.Helper()
	db := database.OpenTest(t, "users")

	a, err := New(db, []byte("test-secret"))
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	return a
}

func TestRegisterAndLogin(t *testing.T) {
	a := newTestAuth(t)

	user, err := a.Register("alice@example.com", "hunter22")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if user.Email != "alice@example.com" {
		t.Errorf("Email = %q", user.Email)
	}
	if user.ID == "" {
		t.Error("ID should not be empty")
	}

	loggedIn, err := a.Login("alice@example.com", "hunter22")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if loggedIn.ID != user.ID {
		t.Errorf("Login returned a different user: %s != %s", loggedIn.ID, user.ID)
	}
}

func TestRegisterDuplicateEmailFails(t *testing.T) {
	a := newTestAuth(t)
	if _, err := a.Register("bob@example.com", "pw1"); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	_, err := a.Register("bob@example.com", "pw2")
	if !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("got error %v, want ErrEmailTaken", err)
	}
}

func TestLoginWrongPasswordFails(t *testing.T) {
	a := newTestAuth(t)
	if _, err := a.Register("carol@example.com", "correct"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	_, err := a.Login("carol@example.com", "wrong")
	if !errors.Is(err, ErrInvalidLogin) {
		t.Fatalf("got error %v, want ErrInvalidLogin", err)
	}
}

func TestLoginUnknownEmailFails(t *testing.T) {
	a := newTestAuth(t)
	_, err := a.Login("nobody@example.com", "whatever")
	if !errors.Is(err, ErrInvalidLogin) {
		t.Fatalf("got error %v, want ErrInvalidLogin", err)
	}
}

func TestPasswordIsHashedNotStoredPlain(t *testing.T) {
	a := newTestAuth(t)
	if _, err := a.Register("dave@example.com", "supersecret"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	var hash string
	if err := a.db.QueryRow(database.Rebind(a.db, `SELECT password_hash FROM users WHERE email = ?`), "dave@example.com").Scan(&hash); err != nil {
		t.Fatalf("query: %v", err)
	}
	if hash == "supersecret" {
		t.Fatal("password was stored in plaintext")
	}
}

func TestUserByID(t *testing.T) {
	a := newTestAuth(t)
	u, err := a.Register("zed@example.com", "hunter22")
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.UserByID(u.ID)
	if err != nil || got.Email != "zed@example.com" || got.ID != u.ID {
		t.Fatalf("UserByID = %+v, %v", got, err)
	}
	if _, err := a.UserByID("no-such-id"); !errors.Is(err, ErrNoUser) {
		t.Fatalf("unknown id: err = %v, want ErrNoUser", err)
	}
}
