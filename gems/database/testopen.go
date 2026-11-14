package database

import (
	"database/sql"
	"os"
	"path/filepath"
)

// TB is the slice of testing.TB that OpenTest needs (avoids importing "testing" in a library).
type TB interface {
	Helper()
	Fatalf(format string, args ...any)
	Cleanup(func())
	TempDir() string
}

// OpenTest opens a fresh database for a test. Backend picked by FWK_TEST_DB:
//
//	"" or "sqlite"  a temp file
//	"postgres"      FWK_TEST_POSTGRES
//	"mysql"         FWK_TEST_MYSQL
//
// On a server backend the named tables are dropped first, so tests start clean.
func OpenTest(t TB, dropTables ...string) *sql.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db")
	switch os.Getenv("FWK_TEST_DB") {
	case "postgres":
		dsn = os.Getenv("FWK_TEST_POSTGRES")
	case "mysql":
		dsn = os.Getenv("FWK_TEST_MYSQL")
	}
	db, err := Open(dsn)
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if os.Getenv("FWK_TEST_DB") != "" && os.Getenv("FWK_TEST_DB") != "sqlite" {
		for _, tbl := range dropTables {
			if _, err := db.Exec("DROP TABLE IF EXISTS " + tbl); err != nil {
				t.Fatalf("drop %s: %v", tbl, err)
			}
		}
	}
	return db
}
