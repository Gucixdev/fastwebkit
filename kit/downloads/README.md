# downloads

File hosting: upload a file and it gets a stable download URL plus a generated `.torrent` and a magnet link, so
it can be fetched over HTTP or BitTorrent (DHT nodes built in, trackers optional; no web seed, so the torrent needs a seeding peer). The client's file name is never used as a path on disk.

Depends on: `database` (`Migrate`, `Rebind`), `ui` (the list page). Used by: the server template.

## Interface

    downloads.New(db, dir, trackers) (*Downloads, error)
    d.Add(filename, contentType, reader) (Entry, error)
    d.List() / d.Get(id) / d.OpenFile(id) / d.TorrentBytes(id)
    d.Routes(mux)       // POST /downloads, GET /downloads, GET /downloads/{id}/file, GET /downloads/{id}/torrent
    downloads.BuildTorrent(path, name, pieceLength, trackers)

`GET /downloads` is a list with an upload form for a browser and JSON for a script.

There is no auth check in this module, on purpose. An app that wants uploads for signed-in users only registers
`Routes` on an inner mux and puts `auth.RequireUser` in front of the POST (the server template shows how, and
adds an upload size cap).

## Files

    downloads.go            storage, bencode, torrent builder, handlers
    *_test.go               torrent/bencode vectors, handlers, storage

## Not done

- No size limit and no virus scan inside the module (the template caps the body); no deletion route.

## Test

    go test ./kit/downloads
