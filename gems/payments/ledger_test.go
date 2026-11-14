package payments

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"fastwebapp/gems/database"
)

func TestLedgerIdempotentAndConcurrent(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	l, err := NewLedger(db)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := l.Paid("w"); ok {
		t.Fatal("paid before any payment")
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	firsts := 0
	for i := 0; i < 20; i++ { // 20 simultaneous deliveries of one webhook: exactly one wins
		wg.Add(1)
		go func() {
			defer wg.Done()
			if f, err := l.Record("stripe", []byte("body-1"), "w"); err == nil && f {
				mu.Lock()
				firsts++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if firsts != 1 {
		t.Fatalf("firsts=%d, want exactly 1", firsts)
	}
	if f, _ := l.Record("stripe", []byte("body-2"), "w"); !f {
		t.Error("a different event was treated as a replay")
	}
	if f, _ := l.Record("paypal", []byte("body-1"), "w"); !f {
		t.Error("same body on another provider must be independent")
	}
	if ok, _ := l.Paid("w"); !ok {
		t.Error("not paid after payment")
	}
}

// Same ledger test against real servers when provided (FWK_TEST_POSTGRES / FWK_TEST_MYSQL).
func TestLedgerOnServers(t *testing.T) {
	for env, name := range map[string]string{"FWK_TEST_POSTGRES": "postgres", "FWK_TEST_MYSQL": "mysql"} {
		dsn := os.Getenv(env)
		if dsn == "" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			db, err := database.Open(dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			db.Exec(`DROP TABLE IF EXISTS payments`)
			l, err := NewLedger(db)
			if err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			var mu sync.Mutex
			firsts := 0
			for i := 0; i < 20; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if f, err := l.Record("stripe", []byte("b"), "w"); err == nil && f {
						mu.Lock()
						firsts++
						mu.Unlock()
					}
				}()
			}
			wg.Wait()
			if firsts != 1 {
				t.Fatalf("firsts=%d want 1", firsts)
			}
			if ok, _ := l.Paid("w"); !ok {
				t.Error("not paid")
			}
		})
	}
}
