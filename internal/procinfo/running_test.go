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
// by its path.
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
	if !slices.ContainsFunc(got, func(s string) bool { return strings.EqualFold(s, exe) }) {
		t.Errorf("%d programs, %q not among them: %q", len(got), exe, got[:min(len(got), 20)])
	}
}
