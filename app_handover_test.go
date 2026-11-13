package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/gateway"
	"github.com/oboo/moho/internal/settings"
	"github.com/oboo/moho/internal/sysproxy"
)

// fakeHandover replaces the guard's process and its question, and counts
// the guards started.
func fakeHandover(t *testing.T, stop bool) (guards, asked *int) {
	t.Helper()
	saved, wait := osHandover, handoverWait
	t.Cleanup(func() { osHandover, handoverWait = saved, wait })
	guards, asked = new(int), new(int)
	osHandover.guard = func(string) error { *guards++; return nil }
	osHandover.ask = func() bool { *asked++; return stop }
	handoverWait = 50 * time.Millisecond
	return guards, asked
}

// TestOnlyUpdateRestartHandsOver: the restart for an update marks its quit,
// and nothing else does.
func TestOnlyUpdateRestartHandsOver(t *testing.T) {
	fakeUpdates(t)
	a := updateApp(t)
	if a.handingOver.Load() {
		t.Fatal("handing over before any update")
	}
	a.quitting = true // the user quit first
	a.restartForUpdate(context.Background(), "9.9.9")
	if a.handingOver.Load() {
		t.Error("a quit the user began hands over")
	}
	a.quitting = false
	a.restartForUpdate(context.Background(), "9.9.9")
	if !a.handingOver.Load() {
		t.Error("the restart for an update does not hand over")
	}
}

// TestNothingToHandOver: without a running core the restart is the
// ordinary quit, and leaves no marker and no guard.
func TestNothingToHandOver(t *testing.T) {
	guards, _ := fakeHandover(t, false)
	dir := t.TempDir()
	cm, err := core.New("/nonexistent/mihomo", filepath.Join(dir, "core"))
	if err != nil {
		t.Fatal(err)
	}
	a := &App{dataDir: dir, core: cm}
	if a.handOver() {
		t.Error("handed over a core that does not run")
	}
	if exists(handoverPath(dir)) || *guards != 0 {
		t.Errorf("marker: %v, guards: %d", exists(handoverPath(dir)), *guards)
	}
	a.headless = true
	if a.handOver() {
		t.Error("the daemon handed over")
	}
}

// TestGuardEndsWithTheMarker: once the new version has taken the marker
// the guard goes without asking, and so it does over a marker whose core
// is gone.
func TestGuardEndsWithTheMarker(t *testing.T) {
	_, asked := fakeHandover(t, true)
	dir := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, dir)
	defer mygo.App.SetPath(mygo.PathUserData, "")
	if err := runHandoverGuard(); err != nil {
		t.Fatal(err)
	}
	if err := writeHandover(dir, handover{PID: 1 << 30, At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := runHandoverGuard(); err != nil {
		t.Fatal(err)
	}
	if *asked != 0 {
		t.Errorf("asked %d times with nothing left running", *asked)
	}
}

// TestHandOverFlows drives the whole of it with a real core: a restart for
// an update leaves the core running and the next app takes it over; a
// guard that is told to keep the proxy touches nothing; one that is told
// to stop ends the core; and an ordinary quit ends a core that was taken
// over. The system proxy is out of reach throughout.
func TestHandOverFlows(t *testing.T) {
	if testing.Short() {
		t.Skip("starts the core")
	}
	saved := osProxy
	osProxy.enable = func(string, int, []string) error { t.Error("the test tried to enable the system proxy"); return nil }
	osProxy.disable = func() error { t.Error("the test tried to disable the system proxy"); return nil }
	osProxy.snapshot = func() (sysproxy.Saved, error) { return sysproxy.Saved{}, nil }
	osProxy.restore = func(context.Context, sysproxy.Saved) error {
		t.Error("the test tried to restore the system proxy")
		return nil
	}
	defer func() { osProxy = saved }()
	savedGw := osGateway
	osGateway = func() gateway.System { return &gwFake{t: t} }
	defer func() { osGateway = savedGw }()
	dir, err := os.MkdirTemp("", "mh") // short: unix socket paths are limited
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	mygo.App.SetPath(mygo.PathUserData, dir)
	defer mygo.App.SetPath(mygo.PathUserData, "")
	if !exists(bundledCore()) {
		t.Skip("no core; run scripts/fetch-core.sh")
	}
	coreDir := filepath.Join(dir, "core")
	defer func() { // whatever happens below, no core is left

		if m, err := core.New("", coreDir); err == nil {
			m.Close()
		}
	}()
	guards, asked := fakeHandover(t, false)

	a, err := newApp()
	if err != nil {
		t.Fatal(err)
	}
	port, err := core.FreePort()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.settings.Update(func(s *settings.Settings) { s.MixedPort = port }); err != nil {
		t.Fatal(err)
	}
	a.copyGeo()
	if err := a.applyConfig(); err != nil {
		t.Fatalf("start: %v", err)
	}
	pid := a.core.Status().PID

	// The restart for an update.
	a.handingOver.Store(true)
	a.shutdown()
	h, ok := readHandover(dir)
	if !ok || h.PID != pid || core.Left(coreDir) != pid || *guards != 1 {
		t.Fatalf("after the restart's quit: marker %+v %v, core %d (was %d), guards %d", h, ok, core.Left(coreDir), pid, *guards)
	}
	if !sysproxy.Listening(context.Background(), "127.0.0.1", port) {
		t.Fatal("the proxy port is closed")
	}

	// The new version is slow: the guard asks, and is told to keep it.
	if err := runHandoverGuard(); err != nil {
		t.Fatal(err)
	}
	if _, ok := readHandover(dir); !ok || core.Left(coreDir) != pid || *asked != 1 {
		t.Fatalf("after keeping: marker %v, core %d, asked %d", ok, core.Left(coreDir), *asked)
	}

	// The new version comes up.
	b, err := newApp()
	if err != nil {
		t.Fatal(err)
	}
	b.firstConfig()
	if st := b.core.Status(); st.State != core.Running || st.PID != pid {
		t.Fatalf("the new app's core: %+v, the one left was %d", st, pid)
	}
	if exists(handoverPath(dir)) {
		t.Error("the marker outlived the takeover")
	}
	if err := runHandoverGuard(); err != nil || *asked != 1 {
		t.Errorf("the guard after the takeover: %v, asked %d", err, *asked)
	}

	// It restarts for an update in turn, never comes back, and the user
	// tells the guard to stop.
	b.handingOver.Store(true)
	b.shutdown()
	if core.Left(coreDir) != pid {
		t.Fatal("the second restart ended the core")
	}
	osHandover.ask = func() bool { *asked++; return true }
	if err := runHandoverGuard(); err != nil {
		t.Fatal(err)
	}
	if core.Left(coreDir) != 0 || exists(handoverPath(dir)) || *asked != 2 {
		t.Fatalf("after stopping: core %d, marker %v, asked %d", core.Left(coreDir), exists(handoverPath(dir)), *asked)
	}

	// An ordinary quit ends the core, and leaves nothing for anyone.
	c, err := newApp()
	if err != nil {
		t.Fatal(err)
	}
	c.firstConfig()
	if st := c.core.Status(); st.State != core.Running || st.PID == pid {
		t.Fatalf("the third app's core: %+v", st)
	}
	c.shutdown()
	if core.Left(coreDir) != 0 || exists(handoverPath(dir)) || *guards != 2 {
		t.Fatalf("after an ordinary quit: core %d, marker %v, guards %d", core.Left(coreDir), exists(handoverPath(dir)), *guards)
	}
}
