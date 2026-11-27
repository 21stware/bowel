package core

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/oboo/moho/internal/routes"
)

// handoverManager is a manager of a core in a folder of its own, running
// on cfg. The folder is short: unix socket paths are limited.
func handoverManager(t *testing.T) (m *Manager, in Input) {
	t.Helper()
	bin := coreBin(t)
	dir, err := os.MkdirTemp("", "moho")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	geo(t, dir)
	m, err = New(bin, dir)
	if err != nil {
		t.Fatal(err)
	}
	probe, _ := FreePort()
	in = Input{
		Routes: routes.Template(), Mode: "rule", MixedPort: mustFree(t), LogLevel: "info",
		TestURL: "https://www.gstatic.com/generate_204", ProbePort: probe, Controller: m.ControllerFields(),
	}
	cfg, _ := BuildConfig(in)
	if err := m.Apply(cfg); err != nil {
		t.Fatalf("start: %v\n%s", err, m.logs.last(10))
	}
	return m, in
}

// leaveRunning lets go of m's core and makes sure that whatever the test
// does next, it does not outlive the test.
func leaveRunning(t *testing.T, m *Manager) int {
	t.Helper()
	st := m.Status()
	pid := m.Detach()
	if pid != st.PID || pid == 0 {
		t.Fatalf("Detach = %d, the core is %d", pid, st.PID)
	}
	t.Cleanup(func() {
		adopted{pid}.kill()
		gone(pid, killGrace)
	})
	return pid
}

// streamThrough opens a connection through the core's proxy port to a
// server that sends a byte every 50 ms for as long as it is read, and
// returns what reads the next bytes: an error once the connection is cut.
func streamThrough(t *testing.T, port int) (next func() error) {
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

// TestHandOver: a core let go of (Detach) runs on, the next manager takes
// it over by its pid and reloads it without restarting it, and it ends
// with that manager; a connection through it is never cut. A core of
// another version is not taken.
func TestHandOver(t *testing.T) {
	m, in := handoverManager(t)
	defer m.Stop()
	if len(m.Logs()) == 0 {
		t.Error("nothing read of the core's log file")
	}
	st := m.Status()
	stream := streamThrough(t, in.MixedPort)

	if got := (&Manager{}).Detach(); got != 0 {
		t.Errorf("Detach with no core = %d", got)
	}
	pid := leaveRunning(t, m)
	m.Close() // what quitting does next: it must not reach the core
	time.Sleep(300 * time.Millisecond)
	if Left(m.Dir) != pid {
		t.Fatal("the core did not outlive its manager")
	}
	if err := m.start(); !errors.Is(err, ErrClosed) {
		t.Errorf("start after Detach: %v", err)
	}

	other, _ := New(m.Bin, m.Dir)
	if other.TakeOver(pid, "v0.0.0") {
		t.Fatal("took over a core of another version")
	}
	if other.TakeOver(pid+1, "") {
		t.Fatal("took over a pid the pid file does not name")
	}

	m2, _ := New(m.Bin, m.Dir)
	var states []State
	m2.OnStatus = func(s Status) { states = append(states, s.State) }
	if !m2.TakeOver(pid, st.Version) {
		t.Fatal("did not take the core over")
	}
	if m2.TakeOver(pid, st.Version) {
		t.Error("took over a core it has already")
	}
	if s := m2.Status(); s.State != Running || s.PID != pid || s.Version != st.Version {
		t.Fatalf("after taking over: %+v", s)
	}
	if !slices.Contains(states, Running) {
		t.Errorf("states told: %v", states)
	}
	in.Mode = "global"
	cfg, _ := BuildConfig(in)
	if m2.Loaded(cfg) {
		t.Error("a core taken over says it has a config it was never given")
	}
	if err := m2.Apply(cfg); err != nil {
		t.Fatal(err)
	}
	c, _ := m2.Client()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if conf, _ := c.Config(ctx); conf.Mode != "global" || m2.Status().PID != pid {
		t.Fatalf("after reload: %+v, pid %d → %d", conf, pid, m2.Status().PID)
	}
	if err := stream(); err != nil {
		t.Errorf("the connection made before the handover was cut: %v", err)
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

// letGoEnv names, for TestLetGoAndExit, the folder of the core to start
// and let go of.
const letGoEnv = "BOWEL_TEST_LET_GO"

// TestCoreOutlivesTheApp: the core that a process lets go of runs on once
// that process has exited, as the app exits for an update's restart, and
// another process takes it over. After that it prints on, to its file: on
// a pipe to the process that is gone its next line would end it.
func TestCoreOutlivesTheApp(t *testing.T) {
	bin := coreBin(t)
	dir, err := os.MkdirTemp("", "moho")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	geo(t, dir)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestLetGoAndExit$", "-test.count=1", "-test.v")
	cmd.Env = append(os.Environ(), letGoEnv+"="+dir)
	out, err := cmd.Output()
	var pid, port int
	if m := regexp.MustCompile(`let go: pid=(\d+) port=(\d+)`).FindStringSubmatch(string(out)); m != nil {
		pid, _ = strconv.Atoi(m[1])
		port, _ = strconv.Atoi(m[2])
	}
	if err != nil || pid == 0 {
		t.Fatalf("the process that starts the core: %v\n%s", err, out)
	}
	defer func() {
		adopted{pid}.kill()
		gone(pid, killGrace)
	}()
	time.Sleep(300 * time.Millisecond)
	if Left(dir) != pid {
		t.Fatal("the core went with the process that started it")
	}

	m, err := New(bin, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	c := m.newClient()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	v, err := c.Version(ctx)
	if err != nil {
		t.Fatalf("the core left running does not answer at the kept controller: %v", err)
	}
	if !m.TakeOver(pid, v.Version) {
		t.Fatal("did not take the core over")
	}
	stream := streamThrough(t, port)
	probe, _ := FreePort()
	cfg, _ := BuildConfig(Input{
		Routes: routes.Template(), Mode: "global", MixedPort: port, LogLevel: "debug",
		TestURL: "https://www.gstatic.com/generate_204", ProbePort: probe, Controller: m.ControllerFields(),
	})
	if err := m.Apply(cfg); err != nil {
		t.Fatal(err)
	}
	if err := stream(); err != nil {
		t.Errorf("the reload cut a connection: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for len(m.Logs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if len(m.Logs()) == 0 {
		t.Error("nothing the core said after the takeover was read")
	}
	if !alive(pid) || m.Status().PID != pid {
		t.Fatalf("the core after printing on: alive %v, status %+v", alive(pid), m.Status())
	}
}

// TestLetGoAndExit is the other process of TestCoreOutlivesTheApp: it
// starts a core, lets go of it, says which and exits.
func TestLetGoAndExit(t *testing.T) {
	dir := os.Getenv(letGoEnv)
	if dir == "" {
		t.Skip("run by TestCoreOutlivesTheApp")
	}
	m, err := New(coreBin(t), dir)
	if err != nil {
		t.Fatal(err)
	}
	probe, _ := FreePort()
	port := mustFree(t)
	cfg, _ := BuildConfig(Input{
		Routes: routes.Template(), Mode: "rule", MixedPort: port, LogLevel: "info",
		TestURL: "https://www.gstatic.com/generate_204", ProbePort: probe, Controller: m.ControllerFields(),
	})
	if err := m.Apply(cfg); err != nil {
		t.Fatalf("start: %v\n%s", err, m.logs.last(10))
	}
	pid := m.Detach()
	if pid == 0 {
		t.Fatal("nothing to let go of")
	}
	fmt.Printf("let go: pid=%d port=%d\n", pid, port)
}

// TestLeftCoreReplaced: a core left running that the next manager does
// not take over is ended, and waited for, before its own starts on the
// same ports; one that a manager never came to start or take over is
// ended by its Close, so that quitting leaves no core behind.
func TestLeftCoreReplaced(t *testing.T) {
	m, in := handoverManager(t)
	pid := leaveRunning(t, m)

	m2, _ := New(m.Bin, m.Dir)
	defer m2.Stop()
	cfg, _ := BuildConfig(in) // the same ports as the core left running
	if err := m2.Apply(cfg); err != nil {
		t.Fatalf("start beside the core left running: %v\n%s", err, m2.logs.last(10))
	}
	if alive(pid) {
		t.Fatal("the core left running is still there")
	}
	if s := m2.Status(); s.State != Running || s.PID == pid || s.Error != "" {
		t.Fatalf("the new core: %+v", s)
	}

	pid2 := leaveRunning(t, m2)
	m3, _ := New(m.Bin, m.Dir)
	m3.Close()
	if alive(pid2) || Left(m.Dir) != 0 {
		t.Error("Close left the core of an earlier run behind")
	}
}

// TestLogTail: lines are taken in as they are added, a line is not split
// by a read that comes in its middle, and an emptied file starts over.
func TestLogTail(t *testing.T) {
	m, err := New("/nonexistent/mihomo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f, err := openLog(m.logPath())
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
	fmt.Fprint(f, "three\n") // at the start of the emptied file, as the core appends
	tail.read()
	if got := m.Logs(); !slices.Equal(got, []string{"one", "two", "three"}) {
		t.Fatalf("after emptying: %q", got)
	}
	if b, _ := os.ReadFile(m.logPath()); string(b) != "three\n" {
		t.Errorf("the file after emptying: %q", b)
	}
}

// TestLogFileEmptiedWhenLarge: the file is only how the lines get here, so
// past maxLogFile it is emptied, and what is written next is still read.
func TestLogFileEmptiedWhenLarge(t *testing.T) {
	m, err := New("/nonexistent/mihomo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f, err := openLog(m.logPath())
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	tail := m.newTail(false)
	line := strings.Repeat("x", 1023) + "\n"
	for written := 0; written <= maxLogFile; written += len(line) {
		fmt.Fprint(f, line)
	}
	for range 8 { // a read takes at most 1 MiB
		tail.read()
	}
	if st, _ := os.Stat(m.logPath()); st.Size() != 0 {
		t.Fatalf("the file is %d bytes, past %d", st.Size(), maxLogFile)
	}
	fmt.Fprint(f, "after\n")
	tail.read()
	if got := m.Logs(); got[len(got)-1] != "after" {
		t.Errorf("the last line read is %q", got[len(got)-1])
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

// TestTakeOverNothing: with no core, or a pid file naming a process that
// is not one, nothing is taken over and the manager stays free to start.
func TestTakeOverNothing(t *testing.T) {
	m, err := New("/nonexistent/mihomo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if m.TakeOver(os.Getpid(), "") {
		t.Error("took over a pid with no pid file")
	}
	_ = os.WriteFile(m.pidFile(), []byte(fmt.Sprint(os.Getpid())), 0o600)
	if m.TakeOver(os.Getpid(), "") {
		t.Error("took over a process that is not a core")
	}
	if s := m.Status(); s.State != Stopped {
		t.Errorf("status %+v", s)
	}
	m.Close() // and the test process, not being a core, is left alone
}
