package asn

import (
	"bytes"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// mmdb writes a one-node IPv4 database of type dbType where every address
// is AS13335, "Cloudflare, Inc.".
func mmdb(dbType string) []byte {
	var b bytes.Buffer
	str := func(s string) {
		if len(s) < 29 {
			b.WriteByte(2<<5 | byte(len(s)))
		} else { // a longer size goes in the next byte
			b.WriteByte(2<<5 | 29)
			b.WriteByte(byte(len(s) - 29))
		}
		b.WriteString(s)
	}
	uint := func(typ byte, v uint64, n int) {
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, v)
		if typ < 8 {
			b.WriteByte(typ<<5 | byte(n))
		} else {
			b.WriteByte(byte(n))
			b.WriteByte(typ - 7)
		}
		b.Write(buf[8-n:])
	}
	// Search tree: one node, both records pointing at data offset 0
	// (node_count + 16 + 0).
	b.Write([]byte{0, 0, 17, 0, 0, 17})
	b.Write(make([]byte, 16))
	b.WriteByte(7<<5 | 2)
	str("autonomous_system_number")
	uint(6, 13335, 2)
	str("autonomous_system_organization")
	str("Cloudflare, Inc.")
	b.WriteString("\xAB\xCD\xEFMaxMind.com")
	b.WriteByte(7<<5 | 7)
	str("node_count")
	uint(6, 1, 1)
	str("record_size")
	uint(5, 24, 1)
	str("ip_version")
	uint(5, 4, 1)
	str("database_type")
	str(dbType)
	str("binary_format_major_version")
	uint(5, 2, 1)
	str("binary_format_minor_version")
	uint(5, 0, 0)
	str("build_epoch")
	uint(9, 1700000000, 4)
	return b.Bytes()
}

func serve(t *testing.T, body []byte) (*httptest.Server, *atomic.Int32) {
	var hits atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write(body)
	}))
	t.Cleanup(s.Close)
	return s, &hits
}

func TestLookupDownloadsThenAnswers(t *testing.T) {
	s, hits := serve(t, mmdb("GeoLite2-ASN"))
	path := filepath.Join(t.TempDir(), "ASN.mmdb")
	d := New(path, s.Client())
	d.url = s.URL
	loaded := make(chan struct{}, 1)
	d.OnLoad = func() { loaded <- struct{}{} }

	if _, ok := d.Lookup("8.8.8.8"); ok {
		t.Fatal("answered before the database was there")
	}
	select {
	case <-loaded:
	case <-time.After(5 * time.Second):
		t.Fatal("database never loaded")
	}
	got, ok := d.Lookup("8.8.8.8")
	if !ok || got != (Info{13335, "Cloudflare, Inc."}) {
		t.Fatalf("Lookup = %+v, %v", got, ok)
	}
	if _, ok := d.Lookup("192.168.1.1"); ok {
		t.Fatal("a private address has no network")
	}

	// The file is kept: a new DB answers from it without downloading.
	d2 := New(path, s.Client())
	d2.url = s.URL
	if got, ok := d2.Lookup("1.1.1.1"); !ok || got.Number != 13335 {
		t.Fatalf("from the file: %+v, %v", got, ok)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("%d downloads, want 1", n)
	}
}

func TestDownloadRefusesOtherDatabases(t *testing.T) {
	s, _ := serve(t, mmdb("GeoLite2-Country"))
	path := filepath.Join(t.TempDir(), "ASN.mmdb")
	d := New(path, s.Client())
	d.url = s.URL
	if _, err := d.download(t.Context()); err == nil {
		t.Fatal("a country database was taken for an ASN one")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("something was written: %v", err)
	}
}

func TestFailedDownloadWaitsBeforeTryingAgain(t *testing.T) {
	var hits atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "no", http.StatusBadGateway)
	}))
	defer s.Close()
	now := time.Unix(1_800_000_000, 0)
	d := New(filepath.Join(t.TempDir(), "ASN.mmdb"), s.Client())
	d.url, d.now = s.URL, func() time.Time { return now }
	wait := func() {
		for i := 0; i < 500; i++ {
			d.mu.Lock()
			f := d.fetching
			d.mu.Unlock()
			if !f {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("download never ended")
	}
	d.Lookup("8.8.8.8")
	wait()
	d.Lookup("8.8.8.8")
	wait()
	if n := hits.Load(); n != 1 {
		t.Fatalf("%d tries within the hour, want 1", n)
	}
	now = now.Add(retry)
	d.Lookup("8.8.8.8")
	wait()
	if n := hits.Load(); n != 2 {
		t.Fatalf("%d tries after the hour, want 2", n)
	}
}
