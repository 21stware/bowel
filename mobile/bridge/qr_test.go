package bridge

import (
	"encoding/json"
	"strings"
	"testing"
)

// The receive sheet's code comes from Call before anything is started, and
// reads back as what it was made of.
func TestCallQR(t *testing.T) {
	const addr = "http://192.168.1.23:41234/abcd2345"
	out, err := Call("qr", `{"input":"`+addr+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	var d shareData
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatal(err)
	}
	if d.Error != "" || d.Text != addr || len(d.Rows) < 21+8 {
		t.Fatalf("got %+v", d)
	}
	for _, r := range d.Rows {
		if len(r) != len(d.Rows) || strings.Trim(r, "01") != "" {
			t.Fatalf("row %q of %d", r, len(d.Rows))
		}
	}
	if !strings.Contains(d.Rows[4], "1111111") { // the finder pattern, inside the quiet zone
		t.Fatalf("no finder in %q", d.Rows[4])
	}

	out, err = Call("qr", `{"input":""}`)
	if err != nil {
		t.Fatal(err)
	}
	var empty shareData
	if err := json.Unmarshal([]byte(out), &empty); err != nil || empty.Error == "" || len(empty.Rows) != 0 {
		t.Fatalf("empty text made %s", out)
	}
}
