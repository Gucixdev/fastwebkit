package mongo

import (
	"errors"
	"os"
	"testing"
)

// Needs a server: FWK_TEST_MONGO=mongodb://127.0.0.1:27017 (skipped otherwise - never faked).
func TestStoreCRUD(t *testing.T) {
	uri := os.Getenv("FWK_TEST_MONGO")
	if uri == "" {
		t.Skip("FWK_TEST_MONGO not set")
	}
	s, err := Open(uri, "fwk_test")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.db.Collection("u").Drop(t.Context())

	if err := s.Unique("u", "email"); err != nil {
		t.Fatal(err)
	}
	id, err := s.Insert("u", Doc{"email": "a@x", "n": 1})
	if err != nil || id == "" {
		t.Fatalf("insert: %v %q", id, err)
	}
	if _, err := s.Insert("u", Doc{"email": "a@x"}); err == nil {
		t.Fatal("duplicate unique field accepted")
	}
	if _, err := s.Insert("u", Doc{"_id": id, "email": "b@x"}); err == nil {
		t.Fatal("duplicate _id overwrote instead of failing")
	}
	d, err := s.Get("u", id)
	if err != nil || d["email"] != "a@x" {
		t.Fatalf("get: %v %v", d, err)
	}
	if err := s.Update("u", id, Doc{"n": 2}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Find("u", Doc{"email": "a@x"}, 10)
	if len(got) != 1 || got[0]["email"] != "a@x" {
		t.Fatalf("find: %v", got)
	}
	if err := s.Delete("u", id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("u", id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
	if err := s.Update("u", "nope", Doc{"n": 1}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}
	if _, err := Open("mongodb://127.0.0.1:1", "x"); err == nil {
		t.Fatal("unreachable server did not error")
	}
}
