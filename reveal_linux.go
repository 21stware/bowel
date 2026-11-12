package main

import (
	"net/url"
	"path/filepath"
	"strings"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/nowindow"
)

// revealFile shows the file at path in the desktop's file manager. It
// first asks over D-Bus (org.freedesktop.FileManager1.ShowItems, which
// Nautilus, Dolphin, Nemo, Caja, Thunar and others answer), which opens
// the folder with the file selected. With no D-Bus session or no file
// manager answering it, it opens the folder with xdg-open, which shows
// the folder but selects nothing.
func revealFile(path string) error {
	if err := nowindow.Command("dbus-send", showItemsArgs(path)...).Run(); err == nil {
		return nil
	}
	cmd := nowindow.Command("xdg-open", filepath.Dir(path))
	if err := cmd.Start(); err != nil {
		return err
	}
	// xdg-open may stay until the file manager it started exits; reap it
	// whenever that is rather than holding the button.
	go cmd.Wait()
	return nil
}

// showItemsArgs are dbus-send's arguments for asking the file manager to
// show path selected. --print-reply makes dbus-send wait for the answer,
// so that no file manager on the bus is an error and not a silent
// nothing.
func showItemsArgs(path string) []string {
	// dbus-send splits an array's elements at commas, and a URI path may
	// keep them, so they are escaped too.
	uri := (&url.URL{Scheme: "file", Path: path}).String()
	uri = strings.ReplaceAll(uri, ",", "%2C")
	return []string{
		"--session", "--print-reply", "--reply-timeout=5000",
		"--dest=org.freedesktop.FileManager1",
		"/org/freedesktop/FileManager1",
		"org.freedesktop.FileManager1.ShowItems",
		"array:string:" + uri,
		"string:",
	}
}

// revealLabel is what the button says on this system.
func revealLabel() string { return i18n.T("在文件管理器中显示", "Show in file manager") }
