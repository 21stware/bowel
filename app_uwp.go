package main

import (
	"os"
	"path/filepath"

	"github.com/oboo/moho/internal/uwploop"
)

// The loopback exemption of Windows' packaged apps (internal/uwploop):
// what lets the Store, Mail and the rest reach the system proxy.

// uwpRecord is where what bowel added to the exemptions is kept.
func (a *App) uwpRecord() string { return filepath.Join(a.dataDir, "uwp-loopback.json") }

// uwpStatus is how many packaged apps may reach the proxy now.
func (a *App) uwpStatus() (uwploop.Status, error) { return uwploop.Read(a.uwpRecord()) }

// uwpAllow exempts every packaged app not yet exempted, after a UAC
// prompt; uwploop.ErrCanceled when the user dismissed it.
func (a *App) uwpAllow() error { return uwploop.AllowAll(a.uwpRecord(), os.TempDir()) }

// uwpUndo takes away what uwpAllow added.
func (a *App) uwpUndo() error { return uwploop.Undo(a.uwpRecord(), os.TempDir()) }
