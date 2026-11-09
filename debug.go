package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/settings"
)

// A development build runs beside the released app on the same Mac. Its
// data is apart already ("bowel Dev"), so what's left is the ports and the
// system's one proxy setting.
const (
	devMixedPort = 17899
	devDNSPort   = 11053
)

// separateFromRelease moves a development build off the released app's
// ports and starts it without taking over: it is turned on by hand in its
// window, for as long as that run. A port the user chose stays; the default
// one is never kept.
func (a *App) separateFromRelease() {
	core.LocalDNSPort = devDNSPort
	if _, err := a.settings.Update(func(s *settings.Settings) {
		s.Paused = true
		if s.MixedPort == settings.Defaults.MixedPort {
			s.MixedPort = devMixedPort
		}
	}); err != nil {
		log.Println("dev separation:", err)
	}
}

// debugDriver lets scripts drive a development build: with MOHO_DEBUG_DIR
// set under `mygo dev`, it runs the command written to <dir>/cmd and
// writes "ok" or an error to <dir>/out. Commands: "shot" (the main window
// to shot.png), "page <home|activity|providers>", "x <cell>" (expand a
// cell, "" to collapse), "sub <route>", "prof <seconds>" (a CPU profile
// of the whole app to cpu.pb, 10 s if no count is given), "heap"
// (heap.pb) and "goroutine" (goroutine.pb). Off otherwise.
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
	case "setup":
		u.win.Update(func() { u.setupLeave() })
	case "gateway":
		// Asks the administrator when it is wanted and not ready yet;
		// the answer goes to <dir>/gw.out, out only says it started.
		go func() {
			err := a.setGateway(arg != "off")
			msg := "ok"
			if err != nil {
				msg = "error: " + err.Error()
			}
			_ = os.WriteFile(filepath.Join(dir, "gw.out"), []byte(msg), 0o600)
		}()
		return "started"
	case "x":
		u.win.Update(func() { u.home.x, u.home.sub = arg, "" })
	case "sub":
		u.win.Update(func() { u.home.x, u.home.sub = "routes", arg })
	case "show":
		a.showWindow()
	case "prof":
		secs := 10
		if n, err := strconv.Atoi(arg); err == nil && n > 0 && n <= 120 {
			secs = n
		}
		f, err := os.Create(filepath.Join(dir, "cpu.pb"))
		if err != nil {
			return "error: " + err.Error()
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			_ = f.Close()
			return "error: " + err.Error()
		}
		time.Sleep(time.Duration(secs) * time.Second)
		pprof.StopCPUProfile()
		_ = f.Close()
		return "ok: cpu.pb, " + strconv.Itoa(secs) + "s"
	case "heap":
		f, err := os.Create(filepath.Join(dir, "heap.pb"))
		if err != nil {
			return "error: " + err.Error()
		}
		defer f.Close()
		if err := pprof.WriteHeapProfile(f); err != nil {
			return "error: " + err.Error()
		}
		return "ok: heap.pb"
	case "goroutine":
		f, err := os.Create(filepath.Join(dir, "goroutine.pb"))
		if err != nil {
			return "error: " + err.Error()
		}
		defer f.Close()
		if err := pprof.Lookup("goroutine").WriteTo(f, 1); err != nil {
			return "error: " + err.Error()
		}
		return "ok: goroutine.pb"
	default:
		return "error: unknown command"
	}
	return "ok"
}
