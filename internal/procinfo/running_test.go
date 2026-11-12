package procinfo

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// TestRunningHasItself: the list of running programs has this test's own,
// by its path, or on Windows by its file name.
func TestRunningHasItself(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("no list of processes here")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	got := Running()
	want := exe
	if runtime.GOOS == "windows" {
		want = filepath.Base(exe)
	}
	if !slices.ContainsFunc(got, func(s string) bool { return strings.EqualFold(s, want) }) {
		t.Errorf("%d programs, %q not among them: %q", len(got), want, got[:min(len(got), 20)])
	}
}
