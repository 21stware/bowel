//go:build !windows

package main

import (
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/nowindow"
)

// revealFile shows the file at path in Finder, selected.
func revealFile(path string) error {
	return nowindow.Command("open", "-R", path).Run()
}

// revealLabel is what the button says on this system.
func revealLabel() string { return i18n.T("在 Finder 中显示", "Show in Finder") }
