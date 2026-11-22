package downloads

import "testing"

func TestBencodeString(t *testing.T) {
	got, err := bencode("spam")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "4:spam" {
		t.Errorf("got %q", got)
	}
}

func TestBencodeInt(t *testing.T) {
	got, err := bencode(42)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "i42e" {
		t.Errorf("got %q", got)
	}
}

func TestBencodeList(t *testing.T) {
	got, err := bencode([]any{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "l1:a1:be" {
		t.Errorf("got %q", got)
	}
}

// The canonical example from the bencode spec: dict keys come out sorted ("cow" before "spam")
// regardless of Go map iteration order.
func TestBencodeDictSortsKeys(t *testing.T) {
	got, err := bencode(map[string]any{"spam": "eggs", "cow": "moo"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "d3:cow3:moo4:spam4:eggse" {
		t.Errorf("got %q", got)
	}
}

func TestBencodeNestedStructure(t *testing.T) {
	got, err := bencode(map[string]any{
		"name":  "file.txt",
		"nodes": []any{[]any{"router.bittorrent.com", 6881}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "d4:name8:file.txt5:nodesll21:router.bittorrent.comi6881eeee"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBencodeUnsupportedType(t *testing.T) {
	_, err := bencode(3.14)
	if err == nil {
		t.Fatal("expected an error for an unsupported type")
	}
}
