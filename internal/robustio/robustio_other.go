//go:build !windows

package robustio

import "os"

// Rename puts tmp in path's place.
func Rename(tmp, path string) error { return os.Rename(tmp, path) }

// Remove removes path.
func Remove(path string) error { return os.Remove(path) }
