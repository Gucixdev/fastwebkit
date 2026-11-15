package kit

import (
	"bytes"
	"crypto/sha1"
	"os"
	"path/filepath"
	"testing"
)

func TestHashPiecesSplitsIntoFixedChunks(t *testing.T) {
	data := []byte("abcdefghij") // 10 bytes
	got, err := hashPieces(bytes.NewReader(data), 4)
	if err != nil {
		t.Fatal(err)
	}
	// 10 bytes / 4-byte pieces = 3 pieces (4, 4, 2) -> 3 * 20 raw SHA-1 bytes.
	if len(got) != 3*sha1.Size {
		t.Fatalf("got %d bytes of piece hashes, want %d", len(got), 3*sha1.Size)
	}

	want := append(append(
		sha1Sum(data[0:4]),
		sha1Sum(data[4:8])...),
		sha1Sum(data[8:10])...,
	)
	if !bytes.Equal(got, want) {
		t.Error("piece hashes don't match manually computed SHA-1 of each chunk")
	}
}

func TestHashPiecesEmptyInput(t *testing.T) {
	got, err := hashPieces(bytes.NewReader(nil), 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %d bytes, want 0 for an empty file", len(got))
	}
}

func sha1Sum(b []byte) []byte {
	sum := sha1.Sum(b)
	return sum[:]
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.bin")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBuildTorrentIsDeterministic(t *testing.T) {
	path := writeTempFile(t, "hello torrent world")
	a, err := BuildTorrent(path, "data.bin", 8, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := BuildTorrent(path, "data.bin", 8, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.InfoHash != b.InfoHash {
		t.Error("building the same file twice should produce the same info hash")
	}
	if !bytes.Equal(a.Bytes, b.Bytes) {
		t.Error("building the same file twice should produce byte-identical metainfo")
	}
}

func TestBuildTorrentHashChangesWithContent(t *testing.T) {
	pathA := writeTempFile(t, "content one")
	pathB := writeTempFile(t, "content two, different")

	a, err := BuildTorrent(pathA, "a.bin", 8, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := BuildTorrent(pathB, "b.bin", 8, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.InfoHash == b.InfoHash {
		t.Error("different file contents should produce different info hashes")
	}
}

func TestBuildTorrentIncludesTrackersAndDHTNodes(t *testing.T) {
	path := writeTempFile(t, "trackers test")
	torrent, err := BuildTorrent(path, "trackers-test.bin", 8, []string{"https://tracker.example/announce"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(torrent.Bytes)
	if !bytes.Contains(torrent.Bytes, []byte("8:announce")) {
		t.Error("expected an announce key when trackers are given")
	}
	if !bytes.Contains(torrent.Bytes, []byte("tracker.example")) {
		t.Errorf("expected the tracker URL in the metainfo, got %q", s)
	}
	if !bytes.Contains(torrent.Bytes, []byte("router.bittorrent.com")) {
		t.Error("expected public DHT bootstrap nodes to always be included")
	}
}

func TestBuildTorrentWellFormedBencodeShape(t *testing.T) {
	path := writeTempFile(t, "shape check")
	torrent, err := BuildTorrent(path, "shape.bin", 8, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(torrent.Bytes) == 0 || torrent.Bytes[0] != 'd' || torrent.Bytes[len(torrent.Bytes)-1] != 'e' {
		t.Errorf("expected a bencoded dict (starts with 'd', ends with 'e'), got %q", torrent.Bytes)
	}
}
