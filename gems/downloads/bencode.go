// Bencode is BitTorrent's own tiny serialization format - simple enough (4 types, no escaping,
// no library needed) that hand-rolling an encoder here is the same call this project already
// made for the ZIP writer in gems/export: own it rather than pull in a torrent library for one
// format. Decoding isn't implemented - this gem only ever produces .torrent files, never reads
// one back.
package downloads

import (
	"bytes"
	"fmt"
	"sort"
)

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
