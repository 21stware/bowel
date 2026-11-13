package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oboo/moho/internal/routes"
)

// TestHandOver: a core let go of (Detach) runs on, the next manager takes
// it over by its pid and reloads it without restarting it, and it ends
// with that manager. A core of another version is not taken.
func TestHandOver(t *testing.T) {
	bin := coreBin(t)
	dir, err := os.MkdirTemp("", "moho")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	geo(t, dir)
	m, err := New(bin, dir)
	if err != nil {
		t.Fatal(err)
	}
	probe, _ := FreePort()
	in := Input{
		Routes: routes.Template(), Mode: "rule", MixedPort: mustFree(t), LogLevel: "info",
		TestURL: "https://www.gstatic.com/generate_204", ProbePort: probe, Controller: m.ControllerFields(),
	}
	cfg, _ := BuildConfig(in)
	if err := m.Apply(cfg); err != nil {
		t.Fatalf("start: %v\n%s", err, m.logs.last(10))
	}
	defer m.Stop()
	if len(m.Logs()) == 0 {
		t.Error("nothing read of the core's log file")
	}
	st := m.Status()

	if got := (&Manager{}).Detach(); got != 0 {
		t.Errorf("Detach with no core = %d", got)
	}
	pid := m.Detach()
	if pid != st.PID {
		t.Fatalf("Detach = %d, the core is %d", pid, st.PID)
	}
	defer adopted{pid}.kill()
	m.Close() // what quitting does next: it must not reach the core
	time.Sleep(300 * time.Millisecond)
	if liveCore(filepath.Join(dir, "mihomo.pid")) != pid {
		t.Fatal("the core did not outlive its manager")
	}
	if err := m.start(); !errors.Is(err, ErrClosed) {
		t.Errorf("start after Detach: %v", err)
	}

	other, _ := New(bin, dir)
	if other.TakeOver(pid, "v0.0.0") {
		t.Fatal("took over a core of another version")
	}
	if other.TakeOver(pid+1, "") {
		t.Fatal("took over a pid the pid file does not name")
	}

	m2, _ := New(bin, dir)
	var states []State
	m2.OnStatus = func(s Status) { states = append(states, s.State) }
	if !m2.TakeOver(pid, st.Version) {
		t.Fatal("did not take the core over")
	}
	if s := m2.Status(); s.State != Running || s.PID != pid || s.Version != st.Version {
		t.Fatalf("after taking over: %+v", s)
	}
	if !slices.Contains(states, Running) {
		t.Errorf("states told: %v", states)
	}
	in.Mode = "global"
	cfg, _ = BuildConfig(in)
	if err := m2.Apply(cfg); err != nil {
		t.Fatal(err)
	}
	c, _ := m2.Client()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if conf, _ := c.Config(ctx); conf.Mode != "global" || m2.Status().PID != pid {
		t.Fatalf("after reload: %+v, pid %d → %d", conf, pid, m2.Status().PID)
	}
	// Only what the core says from now on: the reload, not its start.
	deadline := time.Now().Add(3 * time.Second)
	for len(m2.Logs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if len(m2.Logs()) == 0 {
		t.Error("the taken core's log is not read")
	}

	m2.Stop()
	if s := m2.Status(); s.State != Stopped {
		t.Fatalf("after stop: %+v", s)
	}
	if alive(pid) {
		t.Error("the core outlived the manager that took it over")
	}
}

// TestLogTail: lines are taken in as they are added, a line is not split
// by a read that comes in its middle, and an emptied file starts over.
func TestLogTail(t *testing.T) {
	m, err := New("/nonexistent/mihomo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(m.logPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	fmt.Fprint(f, "old\n")
	tail := m.newTail(true)
	fmt.Fprint(f, "one\ntw")
	tail.read()
	fmt.Fprint(f, "o\r\n")
	tail.read()
	if got := m.Logs(); !slices.Equal(got, []string{"one", "two"}) {
		t.Fatalf("lines = %q", got)
	}
	if err := os.Truncate(m.logPath(), 0); err != nil {
		t.Fatal(err)
	}
	fmt.Fprint(f, "three\n")
	tail.read()
	if got := m.Logs(); !slices.Equal(got, []string{"one", "two", "three"}) {
		t.Fatalf("after emptying: %q", got)
	}
}

// TestControllerKept: where the controller is on TCP, the next manager of
// the same folder has the same address and secret, to reach a core that
// was left running; with the port taken and no core, it chooses anew.
func TestControllerKept(t *testing.T) {
	dir := filepath.Join(t.TempDir(), strings.Repeat("long", 30)) // past a unix socket's limit
	a, err := New("/nonexistent/mihomo", dir)
	if err != nil {
		t.Fatal(err)
	}
	if a.TCPAddr() == "" {
		t.Fatal("not on TCP")
	}
	b, _ := New("/nonexistent/mihomo", dir)
	if b.tcpAddr != a.tcpAddr || b.secret != a.secret || a.secret == "" {
		t.Fatalf("second manager: %s %s, first: %s %s", b.tcpAddr, b.secret, a.tcpAddr, a.secret)
	}
	l, err := net.Listen("tcp", a.tcpAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	c, _ := New("/nonexistent/mihomo", dir)
	if c.tcpAddr == a.tcpAddr || c.secret == a.secret {
		t.Fatalf("kept an address another program has: %s", c.tcpAddr)
	}
	d, _ := New("/nonexistent/mihomo", dir)
	if d.tcpAddr != c.tcpAddr {
		t.Errorf("the new address was not kept: %s, then %s", c.tcpAddr, d.tcpAddr)
	}
}
