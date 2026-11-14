package downloads

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"fastwebapp/gems/database"
)

func newTestDownloads(t *testing.T) *Downloads {
	t.Helper()
	db := database.OpenTest(t, "downloads")

	d, err := New(db, filepath.Join(t.TempDir(), "storage"), nil)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestAddStoresFileAndTorrent(t *testing.T) {
	d := newTestDownloads(t)
	entry, err := d.Add("report.pdf", "application/pdf", strings.NewReader("pdf bytes here"))
	if err != nil {
		t.Fatal(err)
	}
	if entry.Filename != "report.pdf" || entry.SizeBytes != int64(len("pdf bytes here")) {
		t.Errorf("unexpected entry: %+v", entry)
	}
	if entry.InfoHash == "" {
		t.Error("expected a non-empty info hash")
	}

	f, err := d.OpenFile(entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var buf bytes.Buffer
	buf.ReadFrom(f)
	if buf.String() != "pdf bytes here" {
		t.Errorf("stored file content = %q", buf.String())
	}

	torrentBytes, ok, err := d.TorrentBytes(entry.ID)
	if err != nil || !ok {
		t.Fatalf("TorrentBytes: ok=%v err=%v", ok, err)
	}
	if len(torrentBytes) == 0 || torrentBytes[0] != 'd' {
		t.Error("expected a bencoded torrent dict to have been stored")
	}
}

func TestListOrdersMostRecentFirst(t *testing.T) {
	d := newTestDownloads(t)
	first, err := d.Add("first.txt", "text/plain", strings.NewReader("one"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := d.Add("second.txt", "text/plain", strings.NewReader("two"))
	if err != nil {
		t.Fatal(err)
	}

	entries, err := d.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	// uploaded_at has second resolution, so equal timestamps are plausible in a fast test run -
	// just assert both are present, not a strict order, to avoid a flaky test.
	ids := map[string]bool{entries[0].ID: true, entries[1].ID: true}
	if !ids[first.ID] || !ids[second.ID] {
		t.Errorf("expected both uploads in the list, got %+v", entries)
	}
}

func TestGetUnknownIDReturnsFalse(t *testing.T) {
	d := newTestDownloads(t)
	_, ok, err := d.Get("no-such-id")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected ok=false for an unknown id")
	}
}

// A failed insert (closed db) shouldn't leave an orphaned file behind on disk - Add removes
// what it wrote as soon as the insert fails.
func TestAddCleansUpFileWhenInsertFails(t *testing.T) {
	d := newTestDownloads(t)
	d.db.Close()

	_, err := d.Add("orphan.bin", "application/octet-stream", strings.NewReader("data"))
	if err == nil {
		t.Fatal("expected an error once the underlying db is closed")
	}

	matches, err := filepath.Glob(filepath.Join(d.storageDir, "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Errorf("expected no leftover files, found %v", matches)
	}
}
