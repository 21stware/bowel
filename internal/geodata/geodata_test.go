package geodata

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/oboo/moho/internal/routes"
)

// field encodes a protocol buffer field: bytes for wire type 2, a varint
// for wire type 0.
func field(n int, v any) []byte {
	out := binary.AppendUvarint(nil, uint64(n)<<3|map[bool]uint64{true: 2, false: 0}[isBytes(v)])
	switch v := v.(type) {
	case string:
		return append(binary.AppendUvarint(out, uint64(len(v))), v...)
	case []byte:
		return append(binary.AppendUvarint(out, uint64(len(v))), v...)
	case int:
		return binary.AppendUvarint(out, uint64(v))
	}
	panic(v)
}

func isBytes(v any) bool { _, i := v.(int); return !i }

func cat(parts ...[]byte) []byte { return slices.Concat(parts...) }

// site is a GeoSiteList as v2ray's geosite.dat holds it: each entry its
// code, then its domains (type, value, attributes).
func site(codes ...string) []byte {
	var out []byte
	for _, c := range codes {
		domain := cat(field(1, 2), field(2, c+".example"), field(3, cat(field(1, "cn"), field(2, 1))))
		out = append(out, field(1, cat(field(1, c), field(2, domain), field(2, domain)))...)
	}
	return out
}

// TestLists: the names of a GeoSite or GeoIP file, in lower case, sorted,
// and read again once the file changes.
func TestLists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, SiteFile)
	if err := os.WriteFile(path, site("YOUTUBE", "cn", "geolocation-!cn", "cn"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Lists(path)
	if err != nil || !slices.Equal(got, []string{"cn", "geolocation-!cn", "youtube"}) {
		t.Fatalf("Lists = %q, %v", got, err)
	}

	// A GeoIPList: code, then ranges (ip, prefix) and reverse_match.
	cidr := cat(field(1, []byte{1, 0, 1, 0}), field(2, 24))
	ip := cat(field(1, cat(field(1, "CN"), field(2, cidr), field(3, 1))), field(1, cat(field(1, "PRIVATE"), field(2, cidr))))
	later := time.Now().Add(time.Minute)
	if err := os.WriteFile(path, ip, 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(path, later, later)
	if got, err = Lists(path); err != nil || !slices.Equal(got, []string{"cn", "private"}) {
		t.Errorf("after a change, Lists = %q, %v", got, err)
	}

	for _, bad := range [][]byte{{0x0a, 0x50, 1, 2}, {0xff}} {
		if err := os.WriteFile(path, bad, 0o644); err != nil {
			t.Fatal(err)
		}
		later = later.Add(time.Minute)
		_ = os.Chtimes(path, later, later)
		if got, err := Lists(path); err == nil {
			t.Errorf("Lists(% x) = %q, want an error", bad, got)
		}
	}
	if _, err := Lists(filepath.Join(dir, "none.dat")); err == nil {
		t.Error("a file that isn't there has lists")
	}
}

// TestUse: rules check lists against the data in the core's directory, and
// take any while it has none.
func TestUse(t *testing.T) {
	old := routes.GeoLists
	t.Cleanup(func() { routes.GeoLists = old })
	dir := t.TempDir()
	Use(dir)
	if r, err := routes.ParseRule(routes.KindGeo, "youtube", ""); err != nil || r.Value != "youtube" {
		t.Errorf("no data yet: %+v %v", r, err)
	}
	if err := os.WriteFile(filepath.Join(dir, SiteFile), site("netflix"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r, err := routes.ParseRule(routes.KindGeo, "youtube", ""); err == nil {
		t.Errorf("youtube with only netflix in the data: %+v", r)
	}
	if _, err := routes.ParseRule("", "geosite:netflix", ""); err != nil {
		t.Error(err)
	}
}
