//go:build windows

package provider

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// On Windows a file can't be replaced or removed while another program has
// it open, and the core opens a provider's file each time it loads it: a
// rename onto the file fails with ERROR_ACCESS_DENIED, a remove with
// ERROR_SHARING_VIOLATION, until the core lets go, which is soon. So both
// are tried again for a while, as cmd/go's robustio does.
const busyFor = 2 * time.Second

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

// replaceFile puts tmp in path's place.
func replaceFile(tmp, path string) error {
	return whileBusy(func() error { return os.Rename(tmp, path) })
}

func removeFile(path string) error {
	return whileBusy(func() error { return os.Remove(path) })
}
