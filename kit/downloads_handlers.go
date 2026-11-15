package kit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
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
