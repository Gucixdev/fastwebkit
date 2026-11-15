// Package database opens a *sql.DB for whichever backend the DSN names. The core here knows no
// driver: each backend is ONE file (sqlite.go, postgres.go, mysql.go) that registers itself in
// init(). Delete a file and that backend (and its dependency) is gone from the build; copy a
// file into another project and it works alone next to this one. MongoDB is not SQL, so it is
// its own module (mongo).
//
// DSN: "postgres://..." | "mysql://user:pass@host/db" | "sqlite:path" or a bare path.

package kit

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
)

// backend is what a backend file registers.
type backend struct {
	scheme string // DSN prefix it claims, e.g. "postgres://"
	open   func(dsn string) (*sql.DB, error)
	mysql  bool              // INSERT IGNORE instead of ON CONFLICT DO NOTHING
	dollar bool              // placeholders are $1,$2 (true) or ? (false)
	ddl    *strings.Replacer // portable-DDL -> this dialect (nil = as written, i.e. sqlite)
}

var (
	mu       sync.RWMutex
	backends []backend // checked in registration order; the sqlite catch-all must be last (see pick)
	dollars  sync.Map  // *sql.DB -> bool
	mysqls   sync.Map  // *sql.DB -> true for mysql
	ddls     sync.Map  // *sql.DB -> *strings.Replacer
)

func register(b backend) {
	mu.Lock()
	defer mu.Unlock()
	backends = append(backends, b)
}

// pick returns the backend for dsn: a registered scheme prefix wins, otherwise a bare path
// means the backend registered with scheme "" (sqlite).
func pick(dsn string) (backend, bool) {
	mu.RLock()
	defer mu.RUnlock()
	var fallback *backend
	for i := range backends {
		b := backends[i]
		if b.scheme == "" {
			fallback = &backends[i]
			continue
		}
		if strings.HasPrefix(dsn, b.scheme) {
			return b, true
		}
	}
	if fallback != nil {
		return *fallback, true
	}
	return backend{}, false
}

// Open returns a ready *sql.DB.
func Open(dsn string) (*sql.DB, error) {
	b, ok := pick(dsn)
	if !ok {
		return nil, fmt.Errorf("database: no backend compiled in for %q", dsn)
	}
	db, err := b.open(dsn)
	if err != nil {
		return nil, err
	}
	dollars.Store(db, b.dollar)
	if b.mysql {
		mysqls.Store(db, true)
	}
	if b.ddl != nil {
		ddls.Store(db, b.ddl)
	}
	return db, nil
}

// Rebind rewrites "?" placeholders into $1,$2,... when db is a Postgres connection, so a module
// writes its SQL once, with "?". "?" inside single-quoted string literals is left alone.
func Rebind(db *sql.DB, q string) string {
	if v, ok := dollars.Load(db); !ok || !v.(bool) {
		return q
	}
	var b strings.Builder
	n, inStr := 0, false
	for i := 0; i < len(q); i++ {
		c := q[i]
		switch {
		case c == '\'':
			inStr = !inStr
			b.WriteByte(c)
		case c == '?' && !inStr:
			n++
			fmt.Fprintf(&b, "$%d", n)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// InsertIgnore turns "INSERT INTO t (..) VALUES (..)" into the dialect's "insert unless the
// key exists" form: ON CONFLICT DO NOTHING (sqlite, postgres) or INSERT IGNORE (mysql).
func InsertIgnore(db *sql.DB, q string) string {
	if _, ok := mysqls.Load(db); ok {
		return strings.Replace(q, "INSERT INTO", "INSERT IGNORE INTO", 1)
	}
	return q + " ON CONFLICT DO NOTHING"
}

// DDL rewrites portable schema text to db's dialect. Write schemas in sqlite style with the
// portable types TEXT / BLOB; "TEXT PRIMARY KEY" and "TEXT UNIQUE" become bounded VARCHARs on
// MySQL (which cannot key an unbounded TEXT) and BLOB becomes BYTEA on Postgres.
func DDL(db *sql.DB, ddl string) string {
	if r, ok := ddls.Load(db); ok {
		return r.(*strings.Replacer).Replace(ddl)
	}
	return ddl
}

// Migrate runs each statement if it hasn't already (statements should be idempotent, e.g.
// "CREATE TABLE IF NOT EXISTS ..."). No version tracking - modules call this once at startup with
// their own schema, which is enough for a framework this size without becoming its own project.
func Migrate(db *sql.DB, statements ...string) error {
	for _, stmt := range statements {
		if _, err := db.Exec(DDL(db, stmt)); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}
