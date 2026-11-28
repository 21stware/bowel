//go:build windows

package robustio

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// busyFor is how long a refused rename or remove is tried again.
var busyFor = 2 * time.Second

func whileBusy(f func() error) error {
	end := time.Now().Add(busyFor)
	for wait := time.Millisecond; ; wait = min(2*wait, 100*time.Millisecond) {
		err := f()
		busy := errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_SHARING_VIOLATION)
		if !busy || time.Now().After(end) {
			return err
		}
		time.Sleep(wait)
	}
}

// Rename puts tmp in path's place.
func Rename(tmp, path string) error {
	return whileBusy(func() error { return os.Rename(tmp, path) })
}

// Remove removes path.
func Remove(path string) error {
	return whileBusy(func() error { return os.Remove(path) })
}
