package main

// Handing the core over to the next version. A restart for an update used
// to be an ordinary quit: the system proxy and the gateway went back and
// the core ended, so for the seconds until the new version had its own
// core up, what should have been proxied went out directly. Now that one
// restart leaves all three as they are: the core runs on by itself
// (core.Manager.Detach), a marker in the data folder names it, and the new
// version takes it over (TakeOver) and reloads its config, which closes
// nothing.
//
// On Windows the controller is on TCP, at an address kept in the core's
// folder for this (core's tcpController), and the core that TUN's task
// runs is waited for and ended through the task (tun.Attach).
//
// Only that restart does this. Every other way out is the ordinary quit,
// and a launch that finds no marker starts its own core as before.
//
// Should the new version not come up, the core would run with no one to
// ask about it. The guard, a process the old version leaves behind, waits
// for the marker to be taken and otherwise asks the user. It never ends the
// core unasked, as that would send the traffic out directly: with no
// answer the core runs on, and opening bowel again takes it over.

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/tun"
)

const (
	// handoverMarker names the core that a restart for an update left
	// running. The version that takes it over removes it.
	handoverMarker = "handover.json"
	// guardCommand is the argument that makes the executable the guard.
	guardCommand = "handover-guard"
)

// handoverWait is how long the new version has to take the core over
// before the guard asks.
var handoverWait = 30 * time.Second

// handover is the content of handoverMarker.
type handover struct {
	PID int       `json:"pid"`
	At  time.Time `json:"at"`
}

// osHandover is what handing over needs of the system. Tests replace it,
// so that they never start a process or open a dialog.
var osHandover = struct {
	// guard starts the guard for the data folder, a process of its own.
	guard func(dataDir string) error
	// ask asks the user whether to stop the proxy that was left running,
	// and is true only when they said so.
	ask func() bool
}{guard: startGuard, ask: askStopLeftCore}

func handoverPath(dataDir string) string { return filepath.Join(dataDir, handoverMarker) }

func readHandover(dataDir string) (handover, bool) {
	var h handover
	b, err := os.ReadFile(handoverPath(dataDir))
	if err != nil || json.Unmarshal(b, &h) != nil || h.PID <= 1 {
		return handover{}, false
	}
	return h, true
}

func writeHandover(dataDir string, h handover) error {
	b, err := json.Marshal(h)
	if err != nil {
		return err
	}
	path := handoverPath(dataDir)
	if err := os.WriteFile(path+".tmp", b, 0o600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

// handOver leaves the core running for the next version, in place of the
// quit's steps, and reports whether it did. When it did not, nothing has
// changed and the ordinary quit follows.
func (a *App) handOver() bool {
	if a.headless {
		return false
	}
	st := a.core.Status()
	if st.State != core.Running || st.PID <= 1 {
		return false
	}
	// The marker first: a core that runs on with nothing naming it would
	// be no one's.
	if err := writeHandover(a.dataDir, handover{PID: st.PID, At: time.Now()}); err != nil {
		log.Println("handover:", err)
		return false
	}
	if pid := a.core.Detach(); pid != st.PID {
		_ = os.Remove(handoverPath(a.dataDir))
		if pid == 0 {
			return false
		}
		// The core was replaced in between: name the one that was let go.
		if err := writeHandover(a.dataDir, handover{PID: pid, At: time.Now()}); err != nil {
			log.Println("handover:", err)
		}
	}
	a.mu.Lock()
	cancel := a.monCancel
	a.monCancel = nil
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	a.mon.Flush()
	if err := osHandover.guard(a.dataDir); err != nil {
		log.Println("handover: no guard:", err) // the core runs on all the same
	}
	log.Printf("handover: left core %d running for the next version", st.PID)
	return true
}

// takeOverCore takes the core the last version left running, if it left
// one. Before the first config is applied, which then reloads that core
// where it would have started one.
func (a *App) takeOverCore() {
	h, ok := readHandover(a.dataDir)
	if !ok {
		return
	}
	took := a.core.TakeOver(h.PID, tun.Version(a.core.Bin))
	log.Printf("handover: core %d left %s ago, taken over: %v", h.PID, time.Since(h.At).Round(time.Second), took)
}

// endHandover takes the marker away: this version is up, with the core it
// took over or one of its own, and the guard has nothing left to watch.
func (a *App) endHandover() {
	_ = os.Remove(handoverPath(a.dataDir))
}

// runHandoverGuard is the guard: it ends when the new version has taken
// the marker, or the core is gone by itself. After handoverWait with
// neither it asks the user, and only on their word ends the core and puts
// the system proxy and the gateway back, as quitting would have.
func runHandoverGuard() error {
	data, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return err
	}
	watching := func() bool {
		h, ok := readHandover(data)
		return ok && core.Left(filepath.Join(data, "core")) == h.PID
	}
	for deadline := time.Now().Add(handoverWait); time.Now().Before(deadline); time.Sleep(250 * time.Millisecond) {
		if !watching() {
			return nil
		}
	}
	if !watching() || !osHandover.ask() {
		return nil
	}
	// The question may have been open for long: bowel may be back.
	h, ok := readHandover(data)
	if !ok || running(data) {
		return nil
	}
	a, err := newApp()
	if err != nil {
		return err
	}
	a.headless = true
	a.core.TakeOver(h.PID, "") // else Close ends it by the pid file
	_ = os.Remove(handoverPath(data))
	a.shutdown()
	log.Printf("handover: core %d ended on the user's word", h.PID)
	return nil
}

// askStopLeftCore asks with what the desktop has for it. Anything but the
// answer to stop, including no way to ask, leaves the proxy running.
func askStopLeftCore() bool {
	msg := i18n.T("bowel 更新后没有重新打开，代理还在按更新前的配置运行。\n\n重新打开 bowel 就能接着用。要停止代理并还原系统的网络设置吗？停止后流量不再经过代理。",
		"bowel did not reopen after updating, and the proxy is still running with the config it had.\n\nOpening bowel again picks it up. Stop the proxy and put the system's network settings back? Traffic will then no longer go through the proxy.")
	stop, keep := i18n.T("停止代理", "Stop the Proxy"), i18n.T("继续代理", "Keep It Running")
	ctx := context.Background()
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.CommandContext(ctx, "/usr/bin/osascript",
			"-e", "on run argv",
			"-e", `display dialog (item 1 of argv) with title "bowel" buttons {item 2 of argv, item 3 of argv} default button 2 with icon caution`,
			"-e", "button returned of result",
			"-e", "end run", msg, stop, keep).Output()
		return err == nil && strings.TrimSpace(string(out)) == stop
	case "windows":
		return askYesNo(msg + "\n\n" + i18n.T("选「是」停止代理，选「否」让它继续运行。", "Choose Yes to stop the proxy, No to keep it running."))
	case "linux":
		// Stopping is the affirmative answer in both, so that Esc, closing
		// the window and a failure all keep the proxy.
		if bin, err := exec.LookPath("zenity"); err == nil {
			return exec.CommandContext(ctx, bin, "--question", "--title", "bowel", "--text", msg,
				"--ok-label", stop, "--cancel-label", keep, "--default-cancel", "--width", "440").Run() == nil
		}
		if bin, err := exec.LookPath("kdialog"); err == nil {
			return exec.CommandContext(ctx, bin, "--title", "bowel", "--warningyesno", msg,
				"--yes-label", stop, "--no-label", keep).Run() == nil
		}
	}
	return false
}
