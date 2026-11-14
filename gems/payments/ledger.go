package payments

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"

	"fastwebapp/gems/database"
)

// Ledger records paid webhooks exactly once. Providers retry deliveries (and attackers replay
// them); the same signed body must never grant an item twice. The key is provider + sha256 of
// the raw body - a retry is byte-identical, a different payment never is.
type Ledger struct{ db *sql.DB }

func NewLedger(db *sql.DB) (*Ledger, error) {
	// VARCHAR (not TEXT) so the composite key is legal on MySQL too.
	err := database.Migrate(db, `CREATE TABLE IF NOT EXISTS payments (
		provider   VARCHAR(32)  NOT NULL,
		event_hash VARCHAR(64)  NOT NULL,
		item_slug  VARCHAR(191) NOT NULL,
		created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (provider, event_hash)
	)`,
		// ref links a recorded payment to the provider's payment id; revocations are keyed by the
		// same (provider, ref), so a refund is idempotent and works even if it arrives first.
		`CREATE TABLE IF NOT EXISTS payment_refs (
		provider   VARCHAR(32)  NOT NULL,
		event_hash VARCHAR(64)  NOT NULL,
		ref        VARCHAR(191) NOT NULL,
		PRIMARY KEY (provider, event_hash)
	)`, `CREATE TABLE IF NOT EXISTS payment_revocations (
		provider   VARCHAR(32)  NOT NULL,
		ref        VARCHAR(191) NOT NULL,
		created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (provider, ref)
	)`)
	if err != nil {
		return nil, fmt.Errorf("payments ledger: %w", err)
	}
	return &Ledger{db: db}, nil
}

// Record stores a verified paid event. first is false when this exact event was seen before.
func (l *Ledger) Record(provider string, body []byte, slug string) (first bool, err error) {
	sum := sha256.Sum256(body)
	res, err := l.db.Exec(database.Rebind(l.db, database.InsertIgnore(l.db, `INSERT INTO payments (provider, event_hash, item_slug) VALUES (?, ?, ?)`)), provider, hex.EncodeToString(sum[:]), slug)
	if err != nil {
		return false, fmt.Errorf("payments ledger: %w", err)
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// RecordRef is Record plus the provider's payment ref, stored atomically with the payment so a
// later Revoke can find it. An empty ref behaves exactly like Record.
func (l *Ledger) RecordRef(provider string, body []byte, slug, ref string) (first bool, err error) {
	if ref == "" {
		return l.Record(provider, body, slug)
	}
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	tx, err := l.db.Begin()
	if err != nil {
		return false, fmt.Errorf("payments ledger: %w", err)
	}
	defer tx.Rollback()
	res, err := tx.Exec(database.Rebind(l.db, database.InsertIgnore(l.db, `INSERT INTO payments (provider, event_hash, item_slug) VALUES (?, ?, ?)`)), provider, hash, slug)
	if err != nil {
		return false, fmt.Errorf("payments ledger: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil || n != 1 {
		return false, err
	}
	if _, err := tx.Exec(database.Rebind(l.db, database.InsertIgnore(l.db, `INSERT INTO payment_refs (provider, event_hash, ref) VALUES (?, ?, ?)`)), provider, hash, ref); err != nil {
		return false, fmt.Errorf("payments ledger: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("payments ledger: %w", err)
	}
	return true, nil
}

// Revoke withdraws the entitlement of the payment with this provider ref (refund, chargeback,
// invalidated invoice). first is false when the ref was already revoked. Safe to call before the
// payment is recorded: Paid still treats it as revoked.
func (l *Ledger) Revoke(provider, ref string) (first bool, err error) {
	if ref == "" {
		return false, fmt.Errorf("payments ledger: empty ref")
	}
	res, err := l.db.Exec(database.Rebind(l.db, database.InsertIgnore(l.db, `INSERT INTO payment_revocations (provider, ref) VALUES (?, ?)`)), provider, ref)
	if err != nil {
		return false, fmt.Errorf("payments ledger: %w", err)
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// Paid reports whether any recorded, not-revoked payment unlocked slug.
func (l *Ledger) Paid(slug string) (bool, error) {
	var n int
	err := l.db.QueryRow(database.Rebind(l.db, `SELECT COUNT(*) FROM payments p WHERE p.item_slug = ?
		AND NOT EXISTS (SELECT 1 FROM payment_refs f JOIN payment_revocations r ON r.provider = f.provider AND r.ref = f.ref
			WHERE f.provider = p.provider AND f.event_hash = p.event_hash)`), slug).Scan(&n)
	return n > 0, err
}
