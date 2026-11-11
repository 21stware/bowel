//go:build !windows

package core

import "testing"

// TestIsCore: a core left by a crash is known by its name, the old one and
// the copy TUN runs on included, and nothing else is.
func TestIsCore(t *testing.T) {
	for comm, want := range map[string]bool{
		"/Applications/bowel.app/Contents/Resources/bin/" + BinName + "\n": true,
		"/Applications/bowel.app/Contents/Resources/bin/mihomo\n":          true,
		"/Users/a/Library/Application Support/bowel/bin/mihomo\n":          true,
		"/Applications/Safari.app/Contents/MacOS/Safari\n":                 false,
		"": false,
	} {
		if isCore(comm) != want {
			t.Errorf("isCore(%q) = %v", comm, !want)
		}
	}
}
