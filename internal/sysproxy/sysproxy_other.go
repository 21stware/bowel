//go:build !darwin && !windows

package sysproxy

import (
	"context"
	"errors"

	"github.com/oboo/moho/internal/i18n"
)

// TODO: gsettings/kwriteconfig on Linux.

// DefaultBypass is the bypass list a service gets back when Forget clears
// one that was all Moho's.
var DefaultBypass []string

func Enable(host string, port int, bypass []string) error {
	return errors.New(i18n.T("原型暂只支持 macOS 的系统代理", "For now the system proxy works on macOS only"))
}
func Disable() error { return nil }

// Snapshot and Restore have nothing to keep where Enable does nothing.
func Snapshot() (Saved, error)             { return Saved{}, nil }
func Restore(context.Context, Saved) error { return nil }
