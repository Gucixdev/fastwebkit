package kit

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Every backend must pass the same contract. sqlite always runs; postgres/mysql run when
// FWK_TEST_POSTGRES / FWK_TEST_MYSQL hold a DSN (see tests/db-containers in the readme).
func dsns(t *testing.T) map[string]string {
	m := map[string]string{"sqlite": filepath.Join(t.TempDir(), "c.db")}
	if v := os.Getenv("FWK_TEST_POSTGRES"); v != "" {
		m["postgres"] = v
	}
	if v := os.Getenv("FWK_TEST_MYSQL"); v != "" {
		m["mysql"] = v
	}
	return m
}

func TestBackendContract(t *testing.T) {
	for name, dsn := range dsns(t) {
		t.Run(name, func(t *testing.T) {
			db, err := Open(dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			db.Exec(`DROP TABLE IF EXISTS fwk_contract`)
			if err := Migrate(db, `CREATE TABLE IF NOT EXISTS fwk_contract (
				id TEXT PRIMARY KEY, email TEXT UNIQUE NOT NULL, data BLOB NOT NULL, note TEXT NOT NULL)`); err != nil {
				t.Fatal(err)
			}
			if err := Migrate(db, `CREATE TABLE IF NOT EXISTS fwk_contract (id TEXT PRIMARY KEY, email TEXT UNIQUE NOT NULL, data BLOB NOT NULL, note TEXT NOT NULL)`); err != nil {
				t.Fatalf("migrate not idempotent: %v", err)
			}
			blob := []byte{0, 1, 2, 255, 0, 39, 63} // NULs, a quote and a '?' - must survive untouched
			ins := Rebind(db, `INSERT INTO fwk_contract (id, email, data, note) VALUES (?, ?, ?, 'is it ok?')`)
			if _, err := db.Exec(ins, "1", "a@x", blob); err != nil {
				t.Fatalf("insert: %v", err)
			}
			if _, err := db.Exec(ins, "2", "a@x", blob); err == nil {
				t.Fatal("duplicate email accepted: UNIQUE not enforced")
			}
			var got []byte
			var note string
			if err := db.QueryRow(Rebind(db, `SELECT data, note FROM fwk_contract WHERE id = ?`), "1").Scan(&got, &note); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, blob) || note != "is it ok?" {
				t.Fatalf("roundtrip: %v %q", got, note)
			}
			// InsertIgnore: replays are silent no-ops, concurrently too
			ig := Rebind(db, InsertIgnore(db, `INSERT INTO fwk_contract (id, email, data, note) VALUES (?, ?, ?, 'n')`))
			var wg sync.WaitGroup
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); db.Exec(ig, "9", "z@x", blob) }()
			}
			wg.Wait()
			var n int
			db.QueryRow(`SELECT COUNT(*) FROM fwk_contract WHERE id = '9'`).Scan(&n)
			if n != 1 {
				t.Fatalf("InsertIgnore rows=%d, want 1", n)
			}
			db.Exec(`DROP TABLE fwk_contract`)
		})
	}
}

func TestRebindLeavesLiteralsAlone(t *testing.T) {
	db, _ := Open(filepath.Join(t.TempDir(), "r.db"))
	defer db.Close()
	dollars.Store(db, true)
	if got, want := Rebind(db, `a=? AND b='x?y' AND c=?`), `a=$1 AND b='x?y' AND c=$2`; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
