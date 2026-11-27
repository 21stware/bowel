package main

// Handing the core over to the next version. A restart for an update used
// to be an ordinary quit: the system proxy and the gateway went back and
// the core ended, so for the seconds until the new version had its own
// core up, what should have been proxied went out directly, a locked
// route group's traffic included. Now that one restart leaves all of it
// as it is: the core runs on by itself (core.Manager.Detach), a marker in
// the data folder names it, and the new version takes it over (TakeOver)
// and reloads its config, which closes nothing.
//
// The new version takes over only the core it would run itself: the same
// executable (the bundled core, or the copy TUN runs on) and the version
// that file now states. Any other is replaced, as a core left by a crash
// is: the system proxy keeps pointing at the port, so for that second
// connections fail and do not leak, though what TUN carries goes direct.
//
// Under TUN the core runs with rights the app does not have: on macOS and
// Linux a copy that is setuid root, which the app may still signal. Its
// TUN device, its routes and the system's DNS servers, which point into
// it, stay up across the restart with it. On Windows the core is started
// elevated through a UAC prompt by a launcher that ends it with the app
// that started it (tun.Launch), so it does not outlive the old version:
// the new one finds it gone, or going (tun.Attach waits for it), and
// starts its own, which asks again. The system proxy keeps pointing at the
// port meanwhile, so connections fail rather than leak.
//
// Only that restart does this. Every other way out is the ordinary quit,
// and a launch that finds no marker starts its own core as before.
//
// Should the new version not come up, the core would run with no one to
// ask about it. The guard, a process the old version leaves behind, waits
// for the marker to be taken and otherwise asks the user. It never ends
// the core unasked, as that would send the traffic out directly: with no
// answer the core runs on, and opening bowel again takes it over. Whoever
// acts on the marker claims it first (claimHandover), so that the guard
// cannot end a core the new version is taking over.

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
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
	PID int `json:"pid"`
	// Bin is the core's executable, as the manager started it: the new
	// version takes over only the core it would start itself.
	Bin string    `json:"bin"`
	At  time.Time `json:"at"`
	// ID names this hand-over's claim (claimHandover).
	ID string `json:"id"`
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

func readHandover(dataDir string) (handover, bool) { return readHandoverFile(handoverPath(dataDir)) }

func readHandoverFile(path string) (handover, bool) {
	var h handover
	b, err := os.ReadFile(path)
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

// claimPath is the file that says the hand-over id was claimed.
func claimPath(dataDir, id string) string {
	return filepath.Join(dataDir, "handover-"+id+".claimed")
}

// pendingHandover is the marker while no one has claimed it.
func pendingHandover(dataDir string) (handover, bool) {
	h, ok := readHandover(dataDir)
	if !ok || h.ID == "" || exists(claimPath(dataDir, h.ID)) {
		return handover{}, false
	}
	return h, true
}

// claimHandover takes the marker for whoever is to act on it, the new
// version or the guard, and removes it. Of two at once only one gets it:
// each creates the marker's claim, which only one can, and the claim stays
// until the next hand-over, so that one who read the marker late cannot
// make it anew. (A rename is no such test: on Windows two at once can both
// succeed, the second moving the file the first had just moved.)
func claimHandover(dataDir string) (handover, bool) {
	h, ok := pendingHandover(dataDir)
	if !ok {
		return handover{}, false
	}
	f, err := os.OpenFile(claimPath(dataDir, h.ID), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return handover{}, false
	}
	f.Close()
	// Windows will not delete a file while another reads it, as the other
	// claimant or the guard's watch may for a moment. The claim holds
	// whatever comes of this.
	for try := 0; try < 20; try++ {
		if err := os.Remove(handoverPath(dataDir)); err == nil || errors.Is(err, os.ErrNotExist) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return h, true
}

// randomID names one hand-over, for its claim's file name.
func randomID() string { return strings.ToLower(rand.Text()) }

// dropClaims removes the claims of earlier hand-overs, which no one reads
// once a new one is written.
func dropClaims(dataDir string) {
	old, _ := filepath.Glob(filepath.Join(dataDir, "handover-*.claimed"))
	for _, p := range old {
		_ = os.Remove(p)
	}
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
	dropClaims(a.dataDir)
	h := handover{PID: st.PID, Bin: a.core.BinPath(), At: time.Now(), ID: randomID()}
	if err := writeHandover(a.dataDir, h); err != nil {
		log.Println("handover:", err)
		return false
	}
	if pid := a.core.Detach(); pid != h.PID {
		_ = os.Remove(handoverPath(a.dataDir))
		if pid == 0 {
			return false
		}
		// The core was replaced in between: name the one that was let go.
		h.PID, h.Bin = pid, a.core.BinPath()
		if err := writeHandover(a.dataDir, h); err != nil {
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
	log.Printf("handover: left core %d running for the next version", h.PID)
	return true
}

// takeOverCore takes the core the last version left running, if it left
// one and it is the core this version would run. Before the first config
// is applied, which then reloads that core where it would have started
// one; a core it does not take is replaced by that start.
func (a *App) takeOverCore() {
	h, ok := claimHandover(a.dataDir)
	if !ok {
		return
	}
	bin := a.core.BinPath()
	took := false
	if h.Bin == bin {
		// The version the file states now: on Linux and Windows an update
		// replaced the bundled core in place, and a core of another version
		// than this app ships is not taken over. Not knowing it, neither.
		if want := tun.Version(bin); want != "" {
			took = a.core.TakeOver(h.PID, want)
		}
	}
	log.Printf("handover: core %d (%s) left %s ago, taken over: %v", h.PID, h.Bin, time.Since(h.At).Round(time.Second), took)
}

// runHandoverGuard is the guard: it ends once the new version has taken
// the marker, or the core is gone by itself. After handoverWait with
// neither it asks the user, and only on their word ends the core and puts
// the system proxy, the system's DNS servers and the gateway back, as
// quitting would have.
func runHandoverGuard() error {
	data, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return err
	}
	coreDir := filepath.Join(data, "core")
	watching := func() bool {
		h, ok := pendingHandover(data)
		return ok && core.Left(coreDir) == h.PID
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
	if running(data) {
		return nil
	}
	h, ok := claimHandover(data)
	if !ok {
		return nil // the new version took it in the meantime
	}
	a, err := newApp()
	if err != nil {
		return err
	}
	a.headless = true
	// Only the steps of quitting: nothing reacts to the core it takes.
	a.core.OnStatus, a.core.OnListenError = nil, nil
	if !a.core.TakeOver(h.PID, "") {
		log.Printf("handover: core %d did not answer; ending it by its pid", h.PID) // Close does
	}
	a.shutdown()
	log.Printf("handover: core %d ended on the user's word", h.PID)
	return nil
}

// stopLeftCoreText is what the guard asks.
func stopLeftCoreText() (msg, stop, keep string) {
	return i18n.T("bowel 更新后没有重新打开，代理还在按更新前的配置运行。\n\n重新打开 bowel 就能接着用。要停止代理并还原系统的网络设置吗？停止后流量不再经过代理。",
			"bowel did not reopen after updating, and the proxy is still running with the config it had.\n\nOpening bowel again picks it up. Stop the proxy and put the system's network settings back? Traffic will then no longer go through the proxy."),
		i18n.T("停止代理", "Stop the Proxy"), i18n.T("继续代理", "Keep It Running")
}
