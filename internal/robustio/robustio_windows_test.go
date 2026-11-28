//go:build windows

package robustio

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// hold opens path as the core would, and lets go of it after d.
func hold(t *testing.T, path string, d time.Duration) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if d > 0 {
		time.AfterFunc(d, func() { _ = f.Close() })
	}
}

// TestWhileOpen: a rename onto a file another program has open, and a
// remove of one, wait for it to be closed; a file that stays open is
// given up on, with Windows' own error.
func TestWhileOpen(t *testing.T) {
	dir := t.TempDir()
	path, tmp := filepath.Join(dir, "a.txt"), filepath.Join(dir, "a.txt.tmp")
	write := func(name, s string) {
		if err := os.WriteFile(name, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(path, "old")
	write(tmp, "new")
	hold(t, path, 200*time.Millisecond)
	if err := Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "new" {
		t.Fatalf("after the rename: %q", b)
	}

	hold(t, path, 200*time.Millisecond)
	if err := Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("after the remove: %v", err)
	}

	saved := busyFor
	busyFor = 100 * time.Millisecond
	t.Cleanup(func() { busyFor = saved })
	write(path, "old")
	write(tmp, "new")
	hold(t, path, 0)
	if err := Rename(tmp, path); !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Errorf("a rename onto a file still open: %v", err)
	}
	if err := Remove(path); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Errorf("a remove of a file still open: %v", err)
	}
}
