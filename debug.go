package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

// debugDriver lets scripts drive a development build: with MOHO_DEBUG_DIR
// set under `mygo dev`, it runs the command written to <dir>/cmd and
// writes "ok" or an error to <dir>/out. Commands: "shot" (the main window
// to shot.png), "page <home|activity|providers>", "x <cell>" (expand a
// cell, "" to collapse), "sub <route>". Off otherwise.
func (a *App) debugDriver() {
	dir := os.Getenv("MOHO_DEBUG_DIR")
	if dir == "" || !mygo.IsDev() {
		return
	}
	cmdFile := filepath.Join(dir, "cmd")
	for range time.Tick(250 * time.Millisecond) {
		b, err := os.ReadFile(cmdFile)
		if err != nil {
			continue
		}
		_ = os.Remove(cmdFile)
		out := a.debugRun(dir, strings.TrimSpace(string(b)))
		_ = os.WriteFile(filepath.Join(dir, "out.tmp"), []byte(out), 0o600)
		_ = os.Rename(filepath.Join(dir, "out.tmp"), filepath.Join(dir, "out"))
	}
}

func (a *App) debugRun(dir, cmd string) string {
	u := a.uiRef()
	if u == nil || u.win == nil {
		return "error: no window"
	}
	verb, arg, _ := strings.Cut(cmd, " ")
	switch verb {
	case "shot":
		png, err := u.win.CapturePage()
		if err != nil {
			return "error: " + err.Error()
		}
		if err := os.WriteFile(filepath.Join(dir, "shot.png"), png, 0o600); err != nil {
			return "error: " + err.Error()
		}
	case "page":
		u.win.Update(func() { u.page = arg })
	case "x":
		u.win.Update(func() { u.home.x, u.home.sub = arg, "" })
	case "sub":
		u.win.Update(func() { u.home.x, u.home.sub = "routes", arg })
	case "show":
		a.showWindow()
	default:
		return "error: unknown command"
	}
	return "ok"
}
