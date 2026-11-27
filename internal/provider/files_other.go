//go:build !windows

package provider

import "os"

// replaceFile puts tmp in path's place. A file the core has open is
// replaced all the same, and the core goes on reading what it opened.
func replaceFile(tmp, path string) error { return os.Rename(tmp, path) }

func removeFile(path string) error { return os.Remove(path) }
