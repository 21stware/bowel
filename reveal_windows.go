//go:build windows

package main

import (
	"os/exec"
	"syscall"

	"github.com/oboo/moho/internal/i18n"
)

// revealFile shows the file at path in Explorer, selected.
//
// This is exec.Command on purpose, not nowindow.Command: the flag nowindow
// sets to keep a console from flashing is read by Explorer as an order to
// open its window hidden, and the folder comes up as a window that is there
// but invisible. Explorer is a GUI program and opens no console to hide.
func revealFile(path string) error {
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: revealLine(path)}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Explorer answers 1 whether or not it selected anything, so nothing is
	// learned by waiting.
	return cmd.Process.Release()
}

// revealLine is the line Explorer is given: the program first, then
// /select,<path> as one argument with the path quoted. Go would quote a
// path holding a space as a whole, and Explorer would read those quotes as
// part of the path.
func revealLine(path string) string { return `explorer.exe /select,"` + path + `"` }

// revealLabel is what the button says on this system.
func revealLabel() string { return i18n.T("在资源管理器中显示", "Show in Explorer") }
