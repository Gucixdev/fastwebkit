package downloads

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"fastwebapp/gems/database"
)

func newTestServer(t *testing.T) (*Downloads, *http.ServeMux) {
	t.Helper()
	db := database.OpenTest(t, "downloads")

	d, err := New(db, filepath.Join(t.TempDir(), "storage"), []string{"https://tracker.example/announce"})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	d.Routes(mux)
	return d, mux
}

func multipartUpload(t *testing.T, filename, content string) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, writer.FormDataContentType()
}

func TestUploadThenListThenDownload(t *testing.T) {
	_, mux := newTestServer(t)

	body, contentType := multipartUpload(t, "notes.txt", "hello downloads")
	req := httptest.NewRequest(http.MethodPost, "/downloads", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"filename":"notes.txt"`) {
		t.Errorf("upload response missing filename: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "magnet:?xt=urn:btih:") {
		t.Errorf("upload response missing magnet link: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "tracker.example") {
		t.Errorf("upload response missing tracker in magnet link: %s", w.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/downloads", nil)
	listW := httptest.NewRecorder()
	mux.ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("list status = %d", listW.Code)
	}
	if !strings.Contains(listW.Body.String(), "notes.txt") {
		t.Errorf("list response missing uploaded file: %s", listW.Body.String())
	}

	// Pull the id out of the upload response well enough to hit /file and /torrent without a
	// full JSON decode: it's the fixed-width hex id right after "id":".
	idStart := strings.Index(w.Body.String(), `"id":"`) + len(`"id":"`)
	idEnd := strings.Index(w.Body.String()[idStart:], `"`)
	id := w.Body.String()[idStart : idStart+idEnd]

	fileReq := httptest.NewRequest(http.MethodGet, "/downloads/"+id+"/file", nil)
	fileW := httptest.NewRecorder()
	mux.ServeHTTP(fileW, fileReq)
	if fileW.Code != http.StatusOK {
		t.Fatalf("file status = %d", fileW.Code)
	}
	if fileW.Body.String() != "hello downloads" {
		t.Errorf("file content = %q", fileW.Body.String())
	}
	if disp := fileW.Header().Get("Content-Disposition"); !strings.Contains(disp, "notes.txt") {
		t.Errorf("Content-Disposition = %q", disp)
	}

	torrentReq := httptest.NewRequest(http.MethodGet, "/downloads/"+id+"/torrent", nil)
	torrentW := httptest.NewRecorder()
	mux.ServeHTTP(torrentW, torrentReq)
	if torrentW.Code != http.StatusOK {
		t.Fatalf("torrent status = %d", torrentW.Code)
	}
	if torrentW.Header().Get("Content-Type") != "application/x-bittorrent" {
		t.Errorf("torrent Content-Type = %q", torrentW.Header().Get("Content-Type"))
	}
	if torrentW.Body.Len() == 0 || torrentW.Body.Bytes()[0] != 'd' {
		t.Error("expected a non-empty bencoded torrent body")
	}
}

func TestUploadMissingFileFieldIs400(t *testing.T) {
	_, mux := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/downloads", strings.NewReader(""))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestFileAndTorrentUnknownIDAre404(t *testing.T) {
	_, mux := newTestServer(t)
	for _, path := range []string{"/downloads/nope/file", "/downloads/nope/torrent"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404", path, w.Code)
		}
	}
}
