# database

Opens a `*sql.DB` for whichever backend the DSN names, and hides the differences between SQL dialects so a
module writes its SQL once. One file per backend: `sqlite.go`, `postgres.go`, `mysql.go` each register themselves in
`init()`. Delete a file and that backend (and its driver dependency) leaves the build.

Depends on: nothing in this repo. Used by: `auth`, `downloads`, `payments`, and the templates.
MongoDB is not SQL, so it is the separate `mongo` module.

## Interface

    database.Open(dsn) (*sql.DB, error)       // "postgres://..." | "mysql://user:pass@host/db" | "sqlite:path" | bare path = SQLite
    database.Rebind(db, q) string             // "?" placeholders -> $1,$2,... on Postgres (string literals left alone)
    database.InsertIgnore(db, q) string       // INSERT ... -> ON CONFLICT DO NOTHING / INSERT IGNORE
    database.DDL(db, ddl) string              // portable schema text -> dialect (TEXT keys become VARCHAR on MySQL, BLOB -> BYTEA)
    database.Migrate(db, statements...) error // runs idempotent statements ("CREATE TABLE IF NOT EXISTS ...") through DDL
    database.OpenTest(t, dropTables...)       // fresh DB for a test; backend from FWK_TEST_DB=sqlite|postgres|mysql

Write schemas in SQLite style with `TEXT` / `BLOB`, or `VARCHAR(n)` when a column is part of a key
(MySQL cannot key an unbounded TEXT). `CREATE INDEX IF NOT EXISTS` is not portable to MySQL.

## Files

    db.go        Open, Rebind, InsertIgnore, DDL, Migrate, backend registry
    sqlite.go    modernc.org/sqlite (no cgo), WAL
    postgres.go  pgx
    mysql.go     go-sql-driver/mysql (also MariaDB)
    testopen.go  OpenTest (a plain file, so other modules' tests can import it)

## Tested

The contract tests and the auth and downloads tests ran green on real SQLite, PostgreSQL 17 and MariaDB 11
(podman). Re-run:

    podman run -d --rm --name fwk-pg -e POSTGRES_PASSWORD=pw -p 127.0.0.1:55432:5432 postgres:17-alpine
    podman run -d --rm --name fwk-my -e MARIADB_ROOT_PASSWORD=pw -e MARIADB_DATABASE=t -p 127.0.0.1:53306:3306 mariadb:11
    FWK_TEST_POSTGRES=postgres://postgres:pw@127.0.0.1:55432/postgres FWK_TEST_MYSQL=mysql://root:pw@127.0.0.1:53306/t \
    FWK_TEST_DB=postgres go test ./kit/...      # then FWK_TEST_DB=mysql

## Not done

- No migration versions: `Migrate` is "run once at startup, statements idempotent".
- Postgres and MySQL were not run again after the split into folders (only SQLite). Redis/KV and DuckDB do not exist.

## Test

    go test ./kit/database
