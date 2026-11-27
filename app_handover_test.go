package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/gateway"
	"github.com/oboo/moho/internal/settings"
	"github.com/oboo/moho/internal/sysproxy"
)

// fakeHandover replaces the guard's process and its question, and counts
// the guards started and the questions asked; stop is the answer.
func fakeHandover(t *testing.T, stop bool) (guards, asked *int, answer *bool) {
	t.Helper()
	saved, wait := osHandover, handoverWait
	t.Cleanup(func() { osHandover, handoverWait = saved, wait })
	guards, asked, answer = new(int), new(int), new(bool)
	*answer = stop
	osHandover.guard = func(string) error { *guards++; return nil }
	osHandover.ask = func() bool { *asked++; return *answer }
	handoverWait = 50 * time.Millisecond
	return guards, asked, answer
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
	a.restartForUpdate(context.Background())
	if a.handingOver.Load() {
		t.Error("a quit the user began hands over")
	}
	a.quitting = false
	a.restartForUpdate(context.Background())
	if !a.handingOver.Load() {
		t.Error("the restart for an update does not hand over")
	}
}

// TestNothingToHandOver: without a running core the restart is the
// ordinary quit, and leaves no marker and no guard; the daemon, which
// does not update itself, never hands over.
func TestNothingToHandOver(t *testing.T) {
	guards, _, _ := fakeHandover(t, false)
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

// TestClaimHandover: of two that claim the marker at once (the new
// version and the guard), one gets it, and it is gone for both; a marker
// that was claimed is no one's to claim again, even while it is still
// there.
func TestClaimHandover(t *testing.T) {
	dir := t.TempDir()
	if _, ok := claimHandover(dir); ok {
		t.Fatal("claimed a marker there is none of")
	}
	for range 50 {
		dropClaims(dir) // as handOver does
		if err := writeHandover(dir, handover{PID: 4242, Bin: "/x/mihomo", At: time.Now(), ID: randomID()}); err != nil {
			t.Fatal(err)
		}
		var won atomic.Int32
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if h, ok := claimHandover(dir); ok && h.PID == 4242 && h.Bin == "/x/mihomo" {
					won.Add(1)
				}
			}()
		}
		wg.Wait()
		if won.Load() != 1 {
			t.Fatalf("%d claims won", won.Load())
		}
		if _, ok := pendingHandover(dir); ok || exists(handoverPath(dir)) {
			t.Fatal("the marker is still there to claim")
		}
	}
	h := handover{PID: 4242, At: time.Now(), ID: randomID()}
	if err := writeHandover(dir, h); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claimPath(dir, h.ID), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := claimHandover(dir); ok {
		t.Error("claimed a marker someone had claimed")
	}
	dropClaims(dir)
	if left, _ := filepath.Glob(filepath.Join(dir, "handover-*")); len(left) != 0 {
		t.Errorf("claims left: %v", left)
	}
}

// TestGuardEndsWithTheMarker: with no marker the guard goes at once, and
// so it does over a marker whose core is gone, without asking.
func TestGuardEndsWithTheMarker(t *testing.T) {
	_, asked, _ := fakeHandover(t, true)
	dir := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, dir)
	defer mygo.App.SetPath(mygo.PathUserData, "")
	if err := runHandoverGuard(); err != nil {
		t.Fatal(err)
	}
	if err := writeHandover(dir, handover{PID: 1 << 30, At: time.Now(), ID: randomID()}); err != nil {
		t.Fatal(err)
	}
	if err := runHandoverGuard(); err != nil {
		t.Fatal(err)
	}
	if *asked != 0 {
		t.Errorf("asked %d times with nothing left running", *asked)
	}
}

// streamVia opens a connection through the proxy port to a server that
// sends a byte every 50 ms, and returns what reads the next bytes: an
// error once the connection is cut.
func streamVia(t *testing.T, port int) (next func() error) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for r.Context().Err() == nil {
			if _, err := w.Write([]byte("x")); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			time.Sleep(50 * time.Millisecond)
		}
	}))
	t.Cleanup(srv.Close)
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	fmt.Fprintf(conn, "GET %s/ HTTP/1.1\r\nHost: %s\r\n\r\n", srv.URL, srv.Listener.Addr())
	r := bufio.NewReader(conn)
	next = func() error {
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, err := r.Peek(1)
		if err == nil {
			_, err = r.Discard(r.Buffered())
		}
		return err
	}
	if err := next(); err != nil {
		t.Fatalf("nothing came through the proxy: %v", err)
	}
	return next
}

// TestHandOverFlows drives the whole of it with a real core:
//   - a restart for an update leaves the core running, and a connection
//     through it open;
//   - a guard that is told to keep the proxy touches nothing;
//   - the next app takes the core over and reloads it, and the connection
//     stays open;
//   - a core that is not the one the next app would run is replaced;
//   - a guard that is told to stop ends the core;
//   - and an ordinary quit ends the core and leaves nothing behind.
//
// The system proxy is out of reach throughout.
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
	osProxy.check = func(int) (sysproxy.Status, error) { return sysproxy.Status{}, nil }
	defer func() { osProxy = saved }()
	savedGw := osGateway
	osGateway = func() gateway.System { return &gwFake{t: t} }
	defer func() { osGateway = savedGw }()
	savedDNS := osDNS
	osDNS.supported = false
	defer func() { osDNS = savedDNS }()
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
	guards, asked, stop := fakeHandover(t, false)
	port, err := core.FreePort()
	if err != nil {
		t.Fatal(err)
	}
	app := func() *App {
		t.Helper()
		a, err := newApp()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.settings.Update(func(s *settings.Settings) { s.MixedPort = port }); err != nil {
			t.Fatal(err)
		}
		a.copyGeo()
		return a
	}
	// up is what start does with the core: take over the one left, if
	// any, then apply the config.
	up := func(a *App) core.Status {
		t.Helper()
		a.takeOverCore()
		if err := a.applyConfig(); err != nil {
			t.Fatalf("start: %v", err)
		}
		return a.core.Status()
	}
	restart := func(a *App) {
		t.Helper()
		a.handingOver.Store(true)
		a.shutdown()
	}

	a := app()
	pid := up(a).PID
	stream := streamVia(t, port)

	// The restart for an update.
	restart(a)
	h, ok := readHandover(dir)
	if !ok || h.PID != pid || h.Bin != a.core.BinPath() || core.Left(coreDir) != pid || *guards != 1 {
		t.Fatalf("after the restart's quit: marker %+v %v, core %d (was %d), guards %d", h, ok, core.Left(coreDir), pid, *guards)
	}
	if !sysproxy.Listening(context.Background(), "127.0.0.1", port) {
		t.Fatal("the proxy port is closed")
	}
	if err := stream(); err != nil {
		t.Fatalf("the restart's quit cut a connection: %v", err)
	}

	// The new version is slow: the guard asks, and is told to keep it.
	if err := runHandoverGuard(); err != nil {
		t.Fatal(err)
	}
	if _, ok := readHandover(dir); !ok || core.Left(coreDir) != pid || *asked != 1 {
		t.Fatalf("after keeping: marker %v, core %d, asked %d", ok, core.Left(coreDir), *asked)
	}

	// The new version comes up, and takes the core over.
	b := app()
	if st := up(b); st.State != core.Running || st.PID != pid {
		t.Fatalf("the new app's core: %+v, the one left was %d", st, pid)
	}
	if exists(handoverPath(dir)) {
		t.Error("the marker outlived the takeover")
	}
	if err := stream(); err != nil {
		t.Errorf("taking the core over and reloading it cut a connection: %v", err)
	}
	if err := runHandoverGuard(); err != nil || *asked != 1 {
		t.Errorf("the guard after the takeover: %v, asked %d", err, *asked)
	}

	// It restarts for an update in turn, into a version that runs another
	// core: that one is not taken over but replaced.
	restart(b)
	if core.Left(coreDir) != pid {
		t.Fatal("the second restart ended the core")
	}
	h, _ = readHandover(dir)
	h.Bin += ".other"
	if err := writeHandover(dir, h); err != nil {
		t.Fatal(err)
	}
	c := app()
	st := up(c)
	if st.State != core.Running || st.PID == pid || st.Error != "" {
		t.Fatalf("the core of the version that replaced it: %+v", st)
	}
	if exists(handoverPath(dir)) || core.Left(coreDir) != st.PID {
		t.Fatalf("after the replacement: marker %v, core %d", exists(handoverPath(dir)), core.Left(coreDir))
	}
	pid = st.PID

	// It restarts for an update, never comes back, and the user tells the
	// guard to stop.
	restart(c)
	if core.Left(coreDir) != pid || *guards != 3 {
		t.Fatalf("the third restart: core %d, guards %d", core.Left(coreDir), *guards)
	}
	*stop = true
	if err := runHandoverGuard(); err != nil {
		t.Fatal(err)
	}
	if core.Left(coreDir) != 0 || exists(handoverPath(dir)) || *asked != 2 {
		t.Fatalf("after stopping: core %d, marker %v, asked %d", core.Left(coreDir), exists(handoverPath(dir)), *asked)
	}

	// An ordinary quit ends the core, and leaves nothing for anyone.
	d := app()
	if st := up(d); st.State != core.Running || st.PID == pid {
		t.Fatalf("the last app's core: %+v", st)
	}
	d.shutdown()
	if core.Left(coreDir) != 0 || exists(handoverPath(dir)) || *guards != 3 {
		t.Fatalf("after an ordinary quit: core %d, marker %v, guards %d", core.Left(coreDir), exists(handoverPath(dir)), *guards)
	}
}
