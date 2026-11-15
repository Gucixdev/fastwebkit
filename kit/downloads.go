// downloads is the file-hosting module: upload a file, it gets a stable download URL plus
// a generated .torrent and magnet link so it can be grabbed either way. A standalone module - it
// only needs a *sql.DB (from database) and a directory to write into, nothing shop- or
// app-specific leaks in here.

package kit

import (
	"bytes"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

// Bencode is BitTorrent's own tiny serialization format - simple enough (4 types, no escaping,
// no library needed) that hand-rolling an encoder here is the same call this project already
// made for the ZIP writer in export: own it rather than pull in a torrent library for one
// format. Decoding isn't implemented - this module only ever produces .torrent files, never reads
// one back.

// bencode supports the four bencode value types: string (Go string or []byte), integer (any
// Go int kind), list ([]any) and dict (map[string]any, keys always sorted - required by the
// spec so two encoders produce byte-identical output, which is what makes the info-hash stable).
func bencode(value any) ([]byte, error) {
	var buf bytes.Buffer
	if err := encodeInto(&buf, value); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodeInto(buf *bytes.Buffer, value any) error {
	switch v := value.(type) {
	case string:
		fmt.Fprintf(buf, "%d:%s", len(v), v)
	case []byte:
		fmt.Fprintf(buf, "%d:", len(v))
		buf.Write(v)
	case int:
		fmt.Fprintf(buf, "i%de", v)
	case int64:
		fmt.Fprintf(buf, "i%de", v)
	case []any:
		buf.WriteByte('l')
		for _, item := range v {
			if err := encodeInto(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte('e')
	case map[string]any:
		buf.WriteByte('d')
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if err := encodeInto(buf, k); err != nil {
				return err
			}
			if err := encodeInto(buf, v[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('e')
	default:
		return fmt.Errorf("bencode: unsupported type %T", value)
	}
	return nil
}

// Builds a standard .torrent metainfo file for an uploaded file, so a download can also be
// grabbed by any BitTorrent client - not just a direct HTTP GET. No tracker or DHT node of our
// own runs here (that's a whole second server); instead each torrent lists the well-known public
// DHT bootstrap nodes, which is exactly how a modern "trackerless" (BEP-5) torrent finds peers
// with no tracker involved. Seeding still requires something on the swarm - a real deployment
// would need a client/daemon seeding the file - but the metainfo this produces is real and valid,
// openable in any client, not a stub.

const defaultPieceLength = 256 * 1024 // 256 KiB - a reasonable default for small-to-medium files

// publicDHTNodes seeds BEP-5 trackerless discovery - the same bootstrap nodes most desktop
// torrent clients ship with by default.
var publicDHTNodes = []any{
	[]any{"router.bittorrent.com", 6881},
	[]any{"router.utorrent.com", 6881},
	[]any{"dht.transmissionbt.com", 6881},
}

// Torrent is the result of building a .torrent for one file: the encoded metainfo bytes ready
// to serve/save, plus the BEP-9/magnet-link info hash derived from it.
type Torrent struct {
	Bytes    []byte
	InfoHash [20]byte // SHA-1 of the bencoded info dict - what a magnet link's btih is
}

// BuildTorrent hashes filePath in pieceLength-sized chunks and bencodes a single-file metainfo
// dict. pieceLength <= 0 uses defaultPieceLength. trackers is optional (BEP-5 nodes cover the
// trackerless case); any given are added as an announce-list.
func BuildTorrent(filePath, displayName string, pieceLength int64, trackers []string) (Torrent, error) {
	if pieceLength <= 0 {
		pieceLength = defaultPieceLength
	}

	f, err := os.Open(filePath)
	if err != nil {
		return Torrent{}, fmt.Errorf("downloads: open for hashing: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return Torrent{}, fmt.Errorf("downloads: stat for hashing: %w", err)
	}

	pieces, err := hashPieces(f, pieceLength)
	if err != nil {
		return Torrent{}, err
	}

	info := map[string]any{
		"name":         displayName,
		"piece length": pieceLength,
		"pieces":       pieces,
		"length":       stat.Size(),
	}
	infoBytes, err := bencode(info)
	if err != nil {
		return Torrent{}, fmt.Errorf("downloads: bencode info dict: %w", err)
	}
	infoHash := sha1.Sum(infoBytes)

	metainfo := map[string]any{
		"info":  info,
		"nodes": publicDHTNodes,
	}
	if len(trackers) > 0 {
		metainfo["announce"] = trackers[0]
		list := make([]any, 0, len(trackers))
		for _, t := range trackers {
			list = append(list, []any{t})
		}
		metainfo["announce-list"] = list
	}

	data, err := bencode(metainfo)
	if err != nil {
		return Torrent{}, fmt.Errorf("downloads: bencode metainfo: %w", err)
	}

	return Torrent{Bytes: data, InfoHash: infoHash}, nil
}

// hashPieces reads r in pieceLength-sized chunks (the final chunk may be shorter) and returns
// the concatenated raw 20-byte SHA-1 digests - the "pieces" string of a torrent's info dict.
func hashPieces(r io.Reader, pieceLength int64) ([]byte, error) {
	var pieces []byte
	buf := make([]byte, pieceLength)
	for {
		n, err := io.ReadFull(r, buf)
		if n > 0 {
			sum := sha1.Sum(buf[:n])
			pieces = append(pieces, sum[:]...)
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("downloads: hash piece: %w", err)
		}
	}
	return pieces, nil
}

// listing is what both the upload response and the list endpoint return - raw Entry plus the
// URLs/magnet link a frontend actually needs, computed once here rather than making every
// caller rebuild them.
type listing struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	SizeBytes   int64  `json:"sizeBytes"`
	ContentType string `json:"contentType"`
	InfoHash    string `json:"infoHash"`
	UploadedAt  string `json:"uploadedAt"`
	FileURL     string `json:"fileURL"`
	TorrentURL  string `json:"torrentURL"`
	Magnet      string `json:"magnet"`
}

func (d *Downloads) toListing(e Entry) listing {
	return listing{
		ID:          e.ID,
		Filename:    e.Filename,
		SizeBytes:   e.SizeBytes,
		ContentType: e.ContentType,
		InfoHash:    e.InfoHash,
		UploadedAt:  e.UploadedAt.Format("2006-01-02T15:04:05Z07:00"),
		FileURL:     "/downloads/" + e.ID + "/file",
		TorrentURL:  "/downloads/" + e.ID + "/torrent",
		Magnet:      d.magnetLink(e),
	}
}

// magnetLink builds a BEP-9 magnet URI: the info hash is the only required part, dn/tr are
// just hints a client uses to show a nicer name / try a tracker before falling back to DHT.
func (d *Downloads) magnetLink(e Entry) string {
	v := url.Values{}
	v.Set("dn", e.Filename)
	for _, t := range d.trackers {
		v.Add("tr", t)
	}
	return fmt.Sprintf("magnet:?xt=urn:btih:%s&%s", e.InfoHash, v.Encode())
}

// Routes registers the upload/list/file/torrent endpoints. Mount it under whatever prefix the
// app wants - there's no auth check here, same as the rest of this framework's modules; an app
// that needs uploads restricted to logged-in users wraps these with auth.RequireUser itself.
func (d *Downloads) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /downloads", d.handleUpload)
	mux.HandleFunc("GET /downloads", d.handleList)
	mux.HandleFunc("GET /downloads/{id}/file", d.handleFile)
	mux.HandleFunc("GET /downloads/{id}/torrent", d.handleTorrent)
}

func (d *Downloads) handleUpload(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	entry, err := d.Add(header.Filename, contentType, file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if WantsHTML(r) {
		http.Redirect(w, r, "/downloads", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d.toListing(entry))
}

func (d *Downloads) handleList(w http.ResponseWriter, r *http.Request) {
	entries, err := d.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]listing, 0, len(entries))
	for _, e := range entries {
		out = append(out, d.toListing(e))
	}
	if WantsHTML(r) {
		downloadsPage(w, out)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

var downloadsTmpl = template.Must(template.New("downloads").Parse(`<h1>Downloads</h1>
<form method="post" action="/downloads" enctype="multipart/form-data">
<p><input type="file" name="file" required> <button type="submit">Upload</button></p>
</form>
{{if .}}<ul class="cards">{{range .}}
<li><span><a href="{{.FileURL}}">{{.Filename}}</a> <span class="price">{{.Size}}</span></span>
<span><a href="{{.TorrentURL}}">.torrent</a> · <a href="{{.MagnetURL}}">magnet</a></span></li>{{end}}</ul>
{{else}}<p class="price">No files yet.</p>{{end}}`))

// downloadsPage lists the files for a browser, with an upload form that posts back to /downloads.
func downloadsPage(w http.ResponseWriter, items []listing) {
	type row struct {
		listing
		Size string
		// html/template refuses the magnet: scheme in href; the link is built here from a hex hash and
		// url.Values.Encode, so it is safe to mark as a URL (attribute escaping still applies).
		MagnetURL template.URL
	}
	rows := make([]row, len(items))
	for i, it := range items {
		rows[i] = row{it, humanSize(it.SizeBytes), template.URL(it.Magnet)}
	}
	var b strings.Builder
	_ = downloadsTmpl.Execute(&b, rows)
	Page(w, http.StatusOK, "Downloads", template.HTML(b.String()))
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

func (d *Downloads) handleFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	entry, ok, err := d.Get(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	f, err := d.OpenFile(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", entry.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, entry.Filename))
	http.ServeContent(w, r, entry.Filename, entry.UploadedAt, f)
}

func (d *Downloads) handleTorrent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	entry, ok, err := d.Get(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	data, ok, err := d.TorrentBytes(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/x-bittorrent")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, entry.Filename+".torrent"))
	w.Write(data)
}
