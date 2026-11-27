package core

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oboo/moho/internal/mihomo"
	"github.com/oboo/moho/internal/monitor"
)

// waiting is a core Spawn started that does nothing but wait to be stopped.
type waiting struct {
	done chan struct{}
	once sync.Once
}

func (w *waiting) Pid() int    { return os.Getpid() } // not a core's: endStray leaves it be
func (w *waiting) Wait() error { <-w.done; return nil }
func (w *waiting) Stop()       { w.once.Do(func() { close(w.done) }) }

// fakeController answers as a core's controller does to what the manager
// and the monitor ask, and streams lines as the log. It counts the log
// streams opened.
func fakeController(t *testing.T, lines ...mihomo.Log) (srv *httptest.Server, streams *atomic.Int32) {
	streams = new(atomic.Int32)
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version":
			io.WriteString(w, `{"version":"fake"}`)
		case "/proxies":
			io.WriteString(w, `{"proxies":{"GLOBAL":{"name":"GLOBAL","type":"Selector"}}}`)
		case "/connections":
			io.WriteString(w, `{"downloadTotal":0,"uploadTotal":0,"connections":[]}`)
		case "/logs":
			streams.Add(1)
			if lv := r.URL.Query().Get("level"); lv != "info" {
				t.Errorf("log stream at level %q", lv)
			}
			for _, l := range lines {
				io.WriteString(w, `{"type":"`+l.Type+`","payload":"`+l.Payload+`"}`+"\n")
			}
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, streams
}

// managerOn is a manager whose cores answer at srv, and whose Spawn
// starts them.
func managerOn(t *testing.T, srv *httptest.Server) *Manager {
	dir := t.TempDir()
	bin := filepath.Join(dir, "mihomo")
	if err := os.WriteFile(bin, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := New(bin, filepath.Join(dir, "core"))
	if err != nil {
		t.Fatal(err)
	}
	m.socket, m.tcpAddr, m.secret = "", strings.TrimPrefix(srv.URL, "http://"), ""
	m.Spawn = func(string) (Process, error) { return &waiting{done: make(chan struct{})}, nil }
	return m
}

// Issue 200: the log of a core Spawn started is streamed from its
// controller once, by the manager, and serves the manager (its lines, the
// TUN device's failure) and the monitor alike, wired as the app wires
// them.
func TestSpawnedCoreLogIsStreamedOnce(t *testing.T) {
	refused := "[TCP] dial DIRECT 127.0.0.1:50000(curl) --> 127.0.0.1:1 error: connect: connection refused"
	srv, streams := fakeController(t,
		mihomo.Log{Type: "error", Payload: "Start TUN listening error: configure tun interface: Access is denied."},
		mihomo.Log{Type: "warning", Payload: refused},
	)
	m := managerOn(t, srv)
	mon := monitor.New("")
	m.OnLog = mon.Feed
	ctx, cancel := context.WithCancel(context.Background())
	var runs sync.WaitGroup
	defer func() { cancel(); runs.Wait() }()
	fed := false
	m.OnStatus = func(s Status) {
		if s.State != Running {
			return
		}
		c, err := m.Client()
		if err != nil {
			t.Errorf("running, but %v", err)
			return
		}
		run := mon.Run
		if fed = m.LogStreamed(); fed {
			run = mon.RunFed
		}
		runs.Go(func() { run(ctx, c) })
	}
	if err := m.start(); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	if !fed {
		t.Fatal("LogStreamed is false when the core runs")
	}

	// Each of these needs a stream: the manager's for the first, the
	// monitor's own (if it opened one) could give the second.
	deadline := time.Now().Add(5 * time.Second)
	failed := func() bool {
		return slices.ContainsFunc(mon.Conns(), func(c monitor.Conn) bool { return c.Status == "failed" && c.Port == 1 })
	}
	for (m.Status().TunError == "" || !failed()) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if s := m.Status(); s.TunError != "configure tun interface: Access is denied." {
		t.Errorf("TUN error %q", s.TunError)
	}
	if !failed() {
		t.Errorf("the monitor has no failed dial: %+v; logs %+v", mon.Conns(), mon.Logs(0))
	}
	if l := mon.Logs(0); len(l) != 2 || l[1].Level != "warning" || l[1].Text != refused {
		t.Errorf("monitor logs %+v", l)
	}
	if l := m.Logs(); len(l) != 2 || !strings.HasPrefix(l[1], "level=warning msg=") {
		t.Errorf("manager logs %q", l)
	}
	time.Sleep(200 * time.Millisecond) // a stream of the monitor's own would have been opened by now
	if n := streams.Load(); n != 1 {
		t.Errorf("%d log streams, want 1", n)
	}

	m.Stop()
	if m.LogStreamed() {
		t.Error("LogStreamed with no core")
	}
}

// Without OnLog no one takes the log from the manager: whoever streams it
// keeps streaming it.
func TestLogStreamedWantsOnLog(t *testing.T) {
	srv, streams := fakeController(t, mihomo.Log{Type: "info", Payload: "hello"})
	m := managerOn(t, srv)
	if err := m.start(); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	if m.LogStreamed() {
		t.Error("LogStreamed without OnLog")
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(m.Logs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if l := m.Logs(); len(l) != 1 || l[0] != `level=info msg="hello"` || streams.Load() != 1 {
		t.Errorf("logs %q from %d streams", l, streams.Load())
	}
}
