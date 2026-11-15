package kit

import (
	"fmt"
	"path/filepath"
	"testing"
)

// Ceiling of paid-webhook throughput on SQLite (single writer): go test -bench Ledger ./payments
func BenchmarkLedgerRecordNew(b *testing.B) {
	db, err := Open(filepath.Join(b.TempDir(), "b.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	l, _ := NewLedger(db)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := l.Record("stripe", []byte(fmt.Sprint("body-", i)), "w"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLedgerRecordReplay(b *testing.B) {
	db, _ := Open(filepath.Join(b.TempDir(), "b.db"))
	defer db.Close()
	l, _ := NewLedger(db)
	l.Record("stripe", []byte("same"), "w")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Record("stripe", []byte("same"), "w")
	}
}
