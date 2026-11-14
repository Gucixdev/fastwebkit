// sqlite backend (pure Go, modernc.org/sqlite - no cgo): one file, zero setup, the self-hosted
// default. Single file: delete it and sqlite leaves the build.
package database

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

func init() {
	register(backend{scheme: "", open: openSQLite}) // catch-all: a bare path is a sqlite file
	register(backend{scheme: "sqlite:", open: openSQLite})
}

func openSQLite(dsn string) (*sql.DB, error) {
	path := strings.TrimPrefix(dsn, "sqlite:")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// One writer at a time; WAL lets readers continue; busy_timeout waits instead of failing.
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite pragmas: %w", err)
	}
	db.SetMaxOpenConns(1)
	return db, nil
}
