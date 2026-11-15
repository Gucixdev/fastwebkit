// postgres backend (pgx stdlib adapter). Single file: delete it and postgres leaves the build.

package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var pgDDL = strings.NewReplacer("BLOB", "BYTEA")

func init() {
	open := openPostgres
	register(backend{scheme: "postgres://", open: open, dollar: true, ddl: pgDDL})
	register(backend{scheme: "postgresql://", open: open, dollar: true, ddl: pgDDL})
}

func openPostgres(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	db.SetMaxOpenConns(20)
	db.SetConnMaxLifetime(30 * time.Minute)
	return db, nil
}
