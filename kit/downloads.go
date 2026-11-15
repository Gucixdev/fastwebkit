// downloads is the file-hosting module: upload a file, it gets a stable download URL plus
// a generated .torrent and magnet link so it can be grabbed either way. A standalone module - it
// only needs a *sql.DB (from database) and a directory to write into, nothing shop- or
// app-specific leaks in here.

package kit

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

const downloadsSchema = `CREATE TABLE IF NOT EXISTS downloads (
	id TEXT PRIMARY KEY,
	filename TEXT NOT NULL,
	size_bytes INTEGER NOT NULL,
	content_type TEXT NOT NULL,
	info_hash TEXT NOT NULL,
	torrent_data BLOB NOT NULL,
	uploaded_at TEXT NOT NULL
)`

type Entry struct {
	ID          string
	Filename    string
	SizeBytes   int64
	ContentType string
	InfoHash    string // hex-encoded SHA-1, what a magnet link's btih is
	UploadedAt  time.Time
}

type Downloads struct {
	db         *sql.DB
	storageDir string
	trackers   []string
}

// New runs the downloads table migration and returns a Downloads bound to db, storing uploaded
// files under storageDir (created if missing). trackers is optional - see torrent.go, BEP-5 DHT
// bootstrap nodes are always included regardless.
func DownloadsNew(db *sql.DB, storageDir string, trackers []string) (*Downloads, error) {
	if err := Migrate(db, downloadsSchema); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		return nil, fmt.Errorf("downloads: create storage dir: %w", err)
	}
	return &Downloads{db: db, storageDir: storageDir, trackers: trackers}, nil
}

func (d *Downloads) filePath(id string) string {
	return filepath.Join(d.storageDir, id)
}

// Add stores src on disk under a fresh id, builds its .torrent, and records both in the
// downloads table. filename is the display name only (what's shown, and what the torrent's
// info.name is) - never used to build a filesystem path, which is what keeps this safe from
// path traversal regardless of what a client sends as a filename.
func (d *Downloads) Add(filename, contentType string, src io.Reader) (Entry, error) {
	id := uuid.NewString()
	path := d.filePath(id)

	dst, err := os.Create(path)
	if err != nil {
		return Entry{}, fmt.Errorf("downloads: create file: %w", err)
	}
	size, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		os.Remove(path)
		return Entry{}, fmt.Errorf("downloads: write file: %w", copyErr)
	}
	if closeErr != nil {
		os.Remove(path)
		return Entry{}, fmt.Errorf("downloads: close file: %w", closeErr)
	}

	torrent, err := BuildTorrent(path, filename, 0, d.trackers)
	if err != nil {
		os.Remove(path)
		return Entry{}, err
	}

	entry := Entry{
		ID:          id,
		Filename:    filename,
		SizeBytes:   size,
		ContentType: contentType,
		InfoHash:    hex.EncodeToString(torrent.InfoHash[:]),
		UploadedAt:  time.Now(),
	}
	_, err = d.db.Exec(
		Rebind(d.db, `INSERT INTO downloads (id, filename, size_bytes, content_type, info_hash, torrent_data, uploaded_at) VALUES (?, ?, ?, ?, ?, ?, ?)`),
		entry.ID, entry.Filename, entry.SizeBytes, entry.ContentType, entry.InfoHash, torrent.Bytes, entry.UploadedAt.Format(time.RFC3339),
	)
	if err != nil {
		os.Remove(path)
		return Entry{}, fmt.Errorf("downloads: insert row: %w", err)
	}
	return entry, nil
}

// List returns every entry, most recently uploaded first.
func (d *Downloads) List() ([]Entry, error) {
	rows, err := d.db.Query(`SELECT id, filename, size_bytes, content_type, info_hash, uploaded_at FROM downloads ORDER BY uploaded_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("downloads: query list: %w", err)
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		var uploadedAt string
		if err := rows.Scan(&e.ID, &e.Filename, &e.SizeBytes, &e.ContentType, &e.InfoHash, &uploadedAt); err != nil {
			return nil, fmt.Errorf("downloads: scan row: %w", err)
		}
		e.UploadedAt, _ = time.Parse(time.RFC3339, uploadedAt)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// Get looks up one entry's metadata by id.
func (d *Downloads) Get(id string) (Entry, bool, error) {
	var e Entry
	var uploadedAt string
	err := d.db.QueryRow(
		Rebind(d.db, `SELECT id, filename, size_bytes, content_type, info_hash, uploaded_at FROM downloads WHERE id = ?`), id,
	).Scan(&e.ID, &e.Filename, &e.SizeBytes, &e.ContentType, &e.InfoHash, &uploadedAt)
	if err == sql.ErrNoRows {
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, fmt.Errorf("downloads: query entry: %w", err)
	}
	e.UploadedAt, _ = time.Parse(time.RFC3339, uploadedAt)
	return e, true, nil
}

// OpenFile opens the stored file's bytes for serving.
func (d *Downloads) OpenFile(id string) (*os.File, error) {
	return os.Open(d.filePath(id))
}

// TorrentBytes returns the pre-built .torrent file for an entry (built once at upload time,
// not recomputed per request).
func (d *Downloads) TorrentBytes(id string) ([]byte, bool, error) {
	var data []byte
	err := d.db.QueryRow(Rebind(d.db, `SELECT torrent_data FROM downloads WHERE id = ?`), id).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("downloads: query torrent: %w", err)
	}
	return data, true, nil
}
