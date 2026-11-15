package kit

import (
	"path/filepath"
	"testing"
)

func TestOpenSQLiteAndMigrate(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := Migrate(db, `CREATE TABLE IF NOT EXISTS widgets (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO widgets (name) VALUES (?)`, "gear"); err != nil {
		t.Fatalf("insert: %v", err)
	}

	var name string
	if err := db.QueryRow(`SELECT name FROM widgets WHERE name = ?`, "gear").Scan(&name); err != nil {
		t.Fatalf("query: %v", err)
	}
	if name != "gear" {
		t.Fatalf("got %q, want %q", name, "gear")
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	stmt := `CREATE TABLE IF NOT EXISTS widgets (id INTEGER PRIMARY KEY)`
	if err := Migrate(db, stmt); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	// Running the same migration again must not error - modules call Migrate once at every
	// startup, not just the first ever run.
	if err := Migrate(db, stmt); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

func TestMigrateStopsAtFirstError(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	err = Migrate(db, `CREATE TABLE ok (id INTEGER)`, `NOT VALID SQL`, `CREATE TABLE unreached (id INTEGER)`)
	if err == nil {
		t.Fatal("expected an error from invalid SQL")
	}

	var count int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'unreached'`).Scan(&count); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if count != 0 {
		t.Fatal("statement after the failing one should not have run")
	}
}
