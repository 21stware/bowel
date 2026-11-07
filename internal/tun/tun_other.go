//go:build !darwin

// Package tun installs the core so that it can open a TUN device.
package tun

import (
	"errors"

	"github.com/oboo/moho/internal/i18n"
)

// ErrCanceled is returned by Install and Uninstall when the user dismissed
// the administrator prompt.
var ErrCanceled error = canceled{}

// canceled says it in the interface's language when it is said.
type canceled struct{}

func (canceled) Error() string {
	return i18n.T("用户取消了授权", "The authorization was canceled")
}

var errUnsupported = errors.New("not supported")

// Installed is always false outside macOS.
func Installed(path string) bool { return false }

// Install is not supported outside macOS.
func Install(src, dst string) error { return errUnsupported }

// Uninstall is not supported outside macOS.
func Uninstall(dst string) error { return errUnsupported }
