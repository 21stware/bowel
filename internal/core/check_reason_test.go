//go:build !windows

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A core that refuses a config without printing anything still says why:
// the check names how it ended instead of stopping at the colon.
func TestCheckSaysWhyWhenCoreSaysNothing(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "mihomo")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 3\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	m := &Manager{Bin: bin, Dir: dir}
	err := m.Check([]byte("mixed-port: 7890\n"))
	if err == nil || !strings.Contains(err.Error(), "exit status 3") {
		t.Fatalf("err = %v, want it to say exit status 3", err)
	}

	m.Bin = filepath.Join(dir, "missing")
	err = m.Check([]byte("mixed-port: 7890\n"))
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("err = %v, want it to name the core it could not run", err)
	}
}
