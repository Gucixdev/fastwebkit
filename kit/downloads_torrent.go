// Builds a standard .torrent metainfo file for an uploaded file, so a download can also be
// grabbed by any BitTorrent client - not just a direct HTTP GET. No tracker or DHT node of our
// own runs here (that's a whole second server); instead each torrent lists the well-known public
// DHT bootstrap nodes, which is exactly how a modern "trackerless" (BEP-5) torrent finds peers
// with no tracker involved. Seeding still requires something on the swarm - a real deployment
// would need a client/daemon seeding the file - but the metainfo this produces is real and valid,
// openable in any client, not a stub.

package kit

import (
	"crypto/sha1"
	"fmt"
	"io"
	"os"
)

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
