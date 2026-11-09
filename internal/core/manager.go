// Package core runs the mihomo core as a child process (a sidecar) and
// controls it through its external controller.
package core

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/mihomo"
)

type State string

const (
	Stopped  State = "stopped"
	Starting State = "starting"
	Running  State = "running"
	Failed   State = "error"
)

// Status is what the UI shows about the core.
type Status struct {
	State     State     `json:"state"`
	Version   string    `json:"version,omitempty"`
	Error     string    `json:"error,omitempty"`
	PID       int       `json:"pid,omitempty"`
	StartedAt time.Time `json:"startedAt,omitzero"`
}

// ErrClosed is what starting a core says after Close.
var ErrClosed = errors.New("the core manager is closed")

const (
	stopGrace = 3 * time.Second // for the core to exit when asked
	killGrace = time.Second     // for it to be gone once killed
)

// coreProc is a core this manager started and watches: the usual child
// of exec.Cmd, or, on Windows with TUN, the elevated core behind a
// ShellExecuteEx handle, which is no child and has no pipes (the
// controller serves the logs).
type coreProc interface {
	wait() error
	kill() error
}

// errCanceled is what the elevated start reports when the administrator
// declined the prompt.
var errCanceled = errors.New("canceled")

// Manager owns one mihomo process.
type Manager struct {
	Bin string // the mihomo executable
	Dir string // its home directory: config, geo data, cache
	// SafePaths are directories outside Dir the core may read files from
	// (mihomo refuses provider paths elsewhere).
	SafePaths []string

	// OnStatus and OnTraffic are called from the manager's goroutines.
	OnStatus  func(Status)
	OnTraffic func(mihomo.Traffic)

	mu       sync.Mutex
	startMu  sync.Mutex // held across a whole start, whose prompt can block
	cmd      *exec.Cmd
	el       coreProc // the elevated core on Windows (TUN)
	elevate  bool     // start the core elevated (Windows, TUN)
	exited   chan struct{}
	client   *mihomo.Client
	status   Status
	stopping bool
	closed   bool // by Close
	cancel   context.CancelFunc

	// The controller's address: a unix socket, or TCP with a secret on
	// Windows, chosen once per run of the app.
	socket, tcpAddr, secret string

	logs logRing
}

// New prepares a manager; it starts nothing.
func New(bin, dir string) (*Manager, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	m := &Manager{Bin: bin, Dir: dir, status: Status{State: Stopped}}
	socket := filepath.Join(dir, "mihomo.sock")
	// Unix socket paths are limited to ~104 bytes; fall back to TCP.
	if runtime.GOOS == "windows" || len(socket) > 100 {
		port, err := freePort()
		if err != nil {
			return nil, err
		}
		m.tcpAddr = "127.0.0.1:" + strconv.Itoa(port)
		m.secret = randomHex(16)
	} else {
		m.socket = socket
	}
	return m, nil
}

// ConfigPath is where the runtime config is written.
func (m *Manager) ConfigPath() string { return filepath.Join(m.Dir, "config.yaml") }

// ControllerFields are the config keys that point mihomo's controller
// where this manager expects it, overriding those of the profile.
func (m *Manager) ControllerFields() map[string]any {
	if m.socket != "" {
		return map[string]any{
			"external-controller":      "",
			"external-controller-unix": m.socket,
			"secret":                   "",
		}
	}
	return map[string]any{
		"external-controller": m.tcpAddr,
		"secret":              m.secret,
	}
}

// TCPAddr is the controller's TCP address when Moho fell back to TCP, ""
// on the unix socket.
func (m *Manager) TCPAddr() string {
	if m.socket != "" {
		return ""
	}
	return m.tcpAddr
}

func (m *Manager) newClient() *mihomo.Client {
	if m.socket != "" {
		return mihomo.NewUnix(m.socket)
	}
	return mihomo.NewTCP(m.tcpAddr, m.secret)
}

// SetBin changes the executable the next start uses (the setuid copy that
// TUN needs, once installed).
// SetElevate says whether the core starts elevated (Windows, TUN); it
// takes effect at the next start.
func (m *Manager) SetElevate(on bool) {
	if runtime.GOOS != "windows" {
		return
	}
	m.mu.Lock()
	m.elevate = on
	m.mu.Unlock()
}

func (m *Manager) SetBin(bin string) {
	m.mu.Lock()
	m.Bin = bin
	m.mu.Unlock()
}

// Status returns the current status.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

// Client returns the controller client, or an error while the core is not
// running.
func (m *Manager) Client() (*mihomo.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.State != Running || m.client == nil {
		return nil, errors.New(i18n.T("内核未运行", "The core is not running"))
	}
	return m.client, nil
}

func (m *Manager) setStatus(s Status) {
	m.mu.Lock()
	m.status = s
	fn := m.OnStatus
	m.mu.Unlock()
	if fn != nil {
		fn(s)
	}
}

// Test checks a config file with `mihomo -t`, without starting anything.
func (m *Manager) Test(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, m.Bin, "-t", "-d", m.Dir, "-f", path)
	cmd.Env = m.env()
	setProcAttr(cmd) // on Windows, keeps the check from opening a console window
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf(i18n.T("配置检查失败: %s", "The config check failed: %s"), lastLines(string(out), 3))
	}
	return nil
}

// Apply writes cfg as the runtime config and makes the core use it:
// a running core reloads it, a stopped one starts.
func (m *Manager) Apply(cfg []byte) error {
	tmp := filepath.Join(m.Dir, "config.next.yaml")
	if err := os.WriteFile(tmp, cfg, 0o600); err != nil {
		return err
	}
	if err := m.Test(tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, m.ConfigPath()); err != nil {
		return err
	}
	if c, err := m.Client(); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return c.Reload(ctx, m.ConfigPath())
	}
	return m.start()
}

// Restart stops the core and starts it again with the current config.
func (m *Manager) Restart() error {
	m.Stop()
	return m.start()
}

func (m *Manager) start() error {
	// One start at a time: an elevated start blocks on the administrator
	// prompt, and a second one let through meanwhile would race the first.
	m.startMu.Lock()
	defer m.startMu.Unlock()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrClosed
	}
	if m.cmd != nil || m.el != nil {
		m.mu.Unlock()
		return nil
	}
	m.stopping = false
	m.mu.Unlock()

	if _, err := os.Stat(m.Bin); err != nil {
		err = fmt.Errorf(i18n.T("找不到 mihomo 内核 (%s)，先运行 scripts/fetch-core.sh", "No mihomo core at %s; run scripts/fetch-core.sh first"), m.Bin)
		m.setStatus(Status{State: Failed, Error: err.Error()})
		return err
	}
	killStale(filepath.Join(m.Dir, "mihomo.pid"))
	if m.socket != "" {
		_ = os.Remove(m.socket)
	}
	m.setStatus(Status{State: Starting})

	exited := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	pid := 0
	closed := false
	if m.elevate { // Windows, TUN: the core runs as the administrator
		el, epid, err := startElevatedProc(m.Bin, m.Dir, m.ConfigPath(), m.safePathsEnv())
		if err != nil {
			cancel()
			if errors.Is(err, errCanceled) {
				err = errors.New(i18n.T("已取消管理员授权", "The administrator's consent was canceled"))
			}
			m.setStatus(Status{State: Failed, Error: err.Error()})
			return err
		}
		pid = epid
		_ = os.WriteFile(filepath.Join(m.Dir, "mihomo.pid"), []byte(strconv.Itoa(pid)), 0o600)
		m.mu.Lock()
		m.el, m.exited, m.cancel = el, exited, cancel
		closed = m.closed
		m.mu.Unlock()
		go func() {
			err := el.wait()
			cancel()
			m.onExit(err, exited)
		}()
	} else {
		cmd := exec.Command(m.Bin, "-d", m.Dir, "-f", m.ConfigPath())
		cmd.Dir = m.Dir
		cmd.Env = m.env()
		setProcAttr(cmd)
		pr, pw := io.Pipe()
		cmd.Stdout, cmd.Stderr = pw, pw
		if err := cmd.Start(); err != nil {
			pw.Close()
			cancel()
			m.setStatus(Status{State: Failed, Error: err.Error()})
			return err
		}
		pid = cmd.Process.Pid
		_ = os.WriteFile(filepath.Join(m.Dir, "mihomo.pid"), []byte(strconv.Itoa(pid)), 0o600)
		go m.readLogs(pr)
		m.mu.Lock()
		m.cmd, m.exited, m.cancel = cmd, exited, cancel
		closed = m.closed
		m.mu.Unlock()
		go func() {
			err := cmd.Wait()
			pw.Close()
			cancel()
			m.onExit(err, exited)
		}()
	}

	if closed { // Close came while this was starting, and saw no process
		m.Stop()
		return ErrClosed
	}

	// Wait for the controller.
	client := m.newClient()
	deadline := time.Now().Add(15 * time.Second)
	for {
		select {
		case <-exited:
			return errors.New(m.Status().Error)
		default:
		}
		c, cc := context.WithTimeout(ctx, time.Second)
		v, err := client.Version(c)
		if err == nil {
			err = proxiesLoaded(c, client)
		}
		cc()
		if err == nil {
			m.mu.Lock()
			m.client = client
			m.mu.Unlock()
			m.setStatus(Status{State: Running, Version: v.Version, PID: pid, StartedAt: time.Now()})
			if m.OnTraffic != nil { // a stream no one reads costs a decode a second
				go m.traffic(ctx, client)
			}
			return nil
		}
		if time.Now().After(deadline) {
			m.Stop()
			err := errors.New(i18n.T("内核启动超时: ", "The core took too long to start: ") + m.logs.last(2))
			m.setStatus(Status{State: Failed, Error: err.Error()})
			return err
		}
		time.Sleep(150 * time.Millisecond)
	}
}

// onExit records that the core exited: the status first, so that Stop
// and start see it once they return, then exited closes.
func (m *Manager) onExit(err error, exited chan struct{}) {
	defer close(exited)
	m.mu.Lock()
	stopping := m.stopping
	m.cmd, m.el, m.client = nil, nil, nil
	m.mu.Unlock()
	_ = os.Remove(filepath.Join(m.Dir, "mihomo.pid"))
	if stopping {
		m.setStatus(Status{State: Stopped})
		return
	}
	msg := i18n.T("内核意外退出", "The core quit unexpectedly")
	if err != nil {
		msg += ": " + err.Error()
	}
	if l := m.logs.last(2); l != "" {
		msg += "\n" + l
	}
	m.setStatus(Status{State: Failed, Error: msg})
}

// proxiesLoaded reports whether the core has built its proxies and groups.
// mihomo opens its controller before it applies the config, so /version
// answers while /proxies is still empty and selecting a group fails with
// "Resource not found".
func proxiesLoaded(ctx context.Context, c *mihomo.Client) error {
	ps, err := c.Proxies(ctx)
	if err != nil {
		return err
	}
	if _, ok := ps["GLOBAL"]; !ok {
		return errors.New("proxies not loaded yet")
	}
	return nil
}

func (m *Manager) traffic(ctx context.Context, c *mihomo.Client) {
	for ctx.Err() == nil {
		_ = c.StreamTraffic(ctx, func(t mihomo.Traffic) {
			if fn := m.OnTraffic; fn != nil {
				fn(t)
			}
		})
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
}

// Stop ends the core and waits for it to exit: stopGrace to go by itself,
// then killGrace once killed. A process that outlasts both is given up on,
// and the next start's killStale takes care of it.
func (m *Manager) Stop() {
	m.mu.Lock()
	cmd, exited, cancel := m.cmd, m.exited, m.cancel
	el := m.el
	m.stopping = true
	m.mu.Unlock()
	if cmd == nil && el == nil {
		return
	}
	cancel()
	if cmd != nil {
		interrupt(cmd)
	} else {
		_ = el.kill()
	}
	select {
	case <-exited:
		return
	case <-time.After(stopGrace):
	}
	if cmd != nil {
		_ = cmd.Process.Kill()
	} else {
		_ = el.kill()
	}
	select {
	case <-exited:
	case <-time.After(killGrace):
	}
}

// Close stops the core for good: nothing starts it again, even an Apply
// under way. For when the app quits.
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	m.Stop()
}

// Logs returns the latest lines the core printed.
func (m *Manager) Logs() []string { return m.logs.all() }

// Subscribe calls fn with every new line until the returned function is
// called.
func (m *Manager) Subscribe(fn func(string)) (off func()) { return m.logs.subscribe(fn) }

func (m *Manager) readLogs(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		m.logs.add(sc.Text())
	}
}

// logRing keeps the last lines and fans them out.
type logRing struct {
	mu    sync.Mutex
	lines []string
	subs  map[int]func(string)
	next  int
}

const maxLines = 1000

func (r *logRing) add(line string) {
	r.mu.Lock()
	r.lines = append(r.lines, line)
	if len(r.lines) > maxLines {
		r.lines = r.lines[len(r.lines)-maxLines:]
	}
	subs := make([]func(string), 0, len(r.subs))
	for _, fn := range r.subs {
		subs = append(subs, fn)
	}
	r.mu.Unlock()
	for _, fn := range subs {
		fn(line)
	}
}

func (r *logRing) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.lines...)
}

func (r *logRing) last(n int) string {
	l := r.all()
	if len(l) > n {
		l = l[len(l)-n:]
	}
	return strings.Join(l, "\n")
}

func (r *logRing) subscribe(fn func(string)) func() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.subs == nil {
		r.subs = map[int]func(string){}
	}
	id := r.next
	r.next++
	r.subs[id] = fn
	return func() {
		r.mu.Lock()
		delete(r.subs, id)
		r.mu.Unlock()
	}
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func (m *Manager) env() []string {
	env := os.Environ()
	if len(m.SafePaths) > 0 {
		env = append(env, "SAFE_PATHS="+strings.Join(m.SafePaths, string(os.PathListSeparator)))
	}
	return env
}

// safePathsEnv is the SAFE_PATHS value, "" when there is none. The core
// started as the administrator gets its environment from the shell that
// ShellExecuteEx opens, not from us, so startElevatedProc passes this
// along on its command line instead.
func (m *Manager) safePathsEnv() string {
	if len(m.SafePaths) == 0 {
		return ""
	}
	return strings.Join(m.SafePaths, string(os.PathListSeparator))
}

// FreePort returns a loopback TCP port nothing listens on now.
func FreePort() (int, error) { return freePort() }

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
