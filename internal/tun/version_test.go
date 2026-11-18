package tun

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// fakeCore is a program that answers -v the way mihomo does: a shell
// script, or a batch file under Windows, which runs no scripts.
func fakeCore(t *testing.T, line string) string {
	t.Helper()
	p, text := filepath.Join(t.TempDir(), "mihomo"), "#!/bin/sh\necho '"+line+"'\necho 'Use tags: x'\n"
	if runtime.GOOS == "windows" {
		p, text = p+".cmd", "@echo off\r\necho "+line+"\r\necho Use tags: x\r\n"
	}
	if err := os.WriteFile(p, []byte(text), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestStale(t *testing.T) {
	v1 := fakeCore(t, "Mihomo Meta v1.19.33 darwin arm64 with go1.26.10 Wed Oct 21 2026")
	same := fakeCore(t, "Mihomo Meta v1.19.33 darwin arm64 with go1.26.11 Thu Oct 22 2026")
	v2 := fakeCore(t, "Mihomo Meta v1.20.0 darwin arm64 with go1.26.10 Wed Oct 21 2026")
	junk := fakeCore(t, "something else")
	for _, c := range []struct {
		name, a, b string
		want       bool
	}{
		{"other version", v2, v1, true},
		{"same version, another build", same, v1, false},
		{"unreadable output", junk, v1, false},
		{"missing file", v1, filepath.Join(t.TempDir(), "none"), false},
	} {
		if got := Stale(c.a, c.b); got != c.want {
			t.Errorf("%s: %v", c.name, got)
		}
	}
}
