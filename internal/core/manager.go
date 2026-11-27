// Package core runs the mihomo core as a child process (a sidecar) and
// controls it through its external controller.
package core

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	mrand "math/rand/v2"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/mihomo"
	"github.com/oboo/moho/internal/nowindow"
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
	State   State  `json:"state"`
	Version string `json:"version,omitempty"`
	// Error is why the core failed or, while it runs, the proxy port it
	// could not open.
	Error string `json:"error,omitempty"`
	// TunError is what the running core said when it could not bring the
	// TUN device up, "" when it said nothing of the kind or the device
	// came up since.
	TunError  string    `json:"tunError,omitempty"`
	PID       int       `json:"pid,omitempty"`
	StartedAt time.Time `json:"startedAt,omitzero"`
}

// ErrClosed is what starting a core says after Close.
var ErrClosed = errors.New("the core manager is closed")

// StartError is a core that did not come up, as against a config it
// refused or a reload that failed: the one failure another executable may
// not have. It reads as the error it holds.
type StartError struct{ Err error }

func (e *StartError) Error() string { return e.Err.Error() }
func (e *StartError) Unwrap() error { return e.Err }

const (
	stopGrace = 3 * time.Second // for the core to exit when asked
	killGrace = time.Second     // for it to be gone once killed
)

// Manager owns one mihomo process.
type Manager struct {
	Bin string // the mihomo executable
	Dir string // its home directory: config, geo data, cache
	// SafePaths are directories outside Dir the core may read files from
	// (mihomo refuses provider paths elsewhere).
	SafePaths []string

	// ReloadWait is how long Apply waits for a running core to answer a
	// reload; 0 is 15 s. See Loading for what happens after.
	ReloadWait time.Duration

	// OnStatus and OnTraffic are called from the manager's goroutines.
	OnStatus func(Status)
	// OnListenError is called when Status's Error changes while the core
	// runs, which unlike OnStatus says the core itself did not change.
	OnListenError func()
	OnTraffic     func(mihomo.Traffic)
	// OnLog, if set, is called from the manager's goroutine with each entry
	// of the log it streams from the controller of a core Spawn started
	// (see LogStreamed).
	OnLog func(mihomo.Log)

	// Spawn, if set, is asked first to start the core at bin some other
	// way than as a child of the app, and returns nil when bin is not its
	// to start. Windows runs the copy TUN needs elevated, through a UAC
	// prompt, which a child of the app could not be. Such a core prints
	// to no one: its log is read from the controller instead.
	Spawn func(bin string) (Process, error)
	// Source, if set, names the core that stands for the one at bin until
	// that one runs: it checks configs, and its file is the one that must
	// be there. On Windows the copy TUN runs on is made from the bundled
	// core by the start that runs it, and before that may be missing or
	// another version's.
	Source func(bin string) string
	// Attach, if set, is asked first for a core that is already running,
	// left by another run of the app, as one Spawn started: a Process the
	// manager can wait for and end, or nil when it is an ordinary process
	// (one the manager started, which prints to its log file). The other
	// half of Spawn: what Spawn started is not the app's to end as a
	// process.
	Attach func(pid int) Process

	// applyMu makes Apply one at a time: two at once share the file the
	// next config is written to, and one would rename it from under the
	// other (a command that arrives while the first config is applied).
	applyMu sync.Mutex

	mu        sync.Mutex
	cmd       Process
	spawned   bool // cmd is Spawn's: its log comes from the controller
	exited    chan struct{}
	client    *mihomo.Client
	status    Status
	listenErr string // the proxy port the running core could not open
	tunErr    string // why the running core could not bring the TUN device up
	loading   bool   // see Loading
	loaded    []byte // the config the running core loaded in full; nil when not known (see Loaded)
	timing    ApplyTiming
	stopping  bool
	closed    bool // by Close
	detached  bool // by Detach: the core runs on, and is not this manager's
	cancel    context.CancelFunc

	// The controller's address: a unix socket, or TCP with a secret on
	// Windows, kept from run to run (tcpController).
	socket, tcpAddr, secret string

	logs logRing
}

// Process is a running core.
type Process interface {
	Pid() int
	// Wait returns once the core has exited.
	Wait() error
	// Stop asks the core to end, and may be called again to insist.
	Stop()
}

// child is the core as a child of the app. macOS starts it another way
// (startCore in proc_darwin.go).
type child struct{ cmd *exec.Cmd }

// startChild runs bin as a child of the app, its output going to out.
func startChild(bin string, args []string, dir string, env []string, out io.Writer) (Process, error) {
	c := nowindow.Command(bin, args...)
	c.Dir = dir
	c.Env = env
	setProcAttr(c)
	c.Stdout, c.Stderr = out, out
	if err := c.Start(); err != nil {
		return nil, err
	}
	return child{c}, nil
}

func (c child) Pid() int    { return c.cmd.Process.Pid }
func (c child) Wait() error { return c.cmd.Wait() }
func (c child) Stop()       { interrupt(c.cmd) }
func (c child) kill()       { _ = c.cmd.Process.Kill() }

// New prepares a manager; it starts nothing.
func New(bin, dir string) (*Manager, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	m := &Manager{Bin: bin, Dir: dir, status: Status{State: Stopped}}
	socket := filepath.Join(dir, "mihomo.sock")
	// Unix socket paths are limited to ~104 bytes; fall back to TCP.
	if runtime.GOOS == "windows" || len(socket) > 100 {
		if err := m.tcpController(); err != nil {
			return nil, err
		}
	} else {
		m.socket = socket
	}
	return m, nil
}

// controllerFile keeps the TCP controller's address and secret, so that
// the next run of the app can reach a core this one left running (Detach).
// A unix socket is at the same path every run already.
const controllerFile = "controller.json"

// tcpController chooses the TCP controller's address: the last run's while
// the core it named still runs or its port is free, else a new one, which
// is kept for the next run.
func (m *Manager) tcpController() error {
	type saved struct{ Addr, Secret string }
	path := filepath.Join(m.Dir, controllerFile)
	var s saved
	if b, err := os.ReadFile(path); err == nil && json.Unmarshal(b, &s) == nil && s.Addr != "" && s.Secret != "" {
		free := false
		if l, err := net.Listen("tcp", s.Addr); err == nil {
			l.Close()
			free = true
		}
		if free || liveCore(m.pidFile()) != 0 {
			m.tcpAddr, m.secret = s.Addr, s.Secret
			return nil
		}
	}
	port, err := freePort()
	if err != nil {
		return err
	}
	m.tcpAddr, m.secret = "127.0.0.1:"+strconv.Itoa(port), randomHex(16)
	b, _ := json.Marshal(saved{m.tcpAddr, m.secret})
	return os.WriteFile(path, b, 0o600)
}

// pidFile names the running core, for the next run of the app: one this
// run left behind (a crash, or Detach) is ended (endStray) or taken over
// (TakeOver) by it.
func (m *Manager) pidFile() string { return filepath.Join(m.Dir, "mihomo.pid") }

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
func (m *Manager) SetBin(bin string) {
	m.mu.Lock()
	m.Bin = bin
	m.mu.Unlock()
}

// source is the core whose file stands for Bin's (see Source).
func (m *Manager) source() string {
	if m.Source != nil {
		return m.Source(m.Bin)
	}
	return m.Bin
}

// BinPath is the executable the next start uses.
func (m *Manager) BinPath() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Bin
}

// Status returns the current status.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.status
	if s.State == Running {
		s.Error = m.listenErr
		s.TunError = m.tunErr
	}
	return s
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
	cmd := nowindow.CommandContext(ctx, m.source(), "-t", "-d", m.Dir, "-f", path)
	cmd.Env = m.env()
	setProcAttr(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		why := coreSaid(string(out))
		if why == "" {
			// It printed nothing: say how it ended, so the reason isn't blank.
			why = err.Error()
			if ctx.Err() != nil {
				why = i18n.T("内核 30 秒内没有给出结果", "The core gave no answer within 30 seconds")
			}
		}
		return fmt.Errorf(i18n.T("配置检查失败: %s", "The config check failed: %s"), why)
	}
	return nil
}

// Check says whether the core would accept cfg, touching nothing.
func (m *Manager) Check(cfg []byte) error {
	tmp := filepath.Join(m.Dir, "config.check.yaml")
	if err := os.WriteFile(tmp, cfg, 0o600); err != nil {
		return err
	}
	defer os.Remove(tmp)
	return m.Test(tmp)
}

// Apply writes cfg as the runtime config and makes the core use it:
// a running core reloads it, a stopped one starts.
//
// config.yaml is only ever replaced by a config the core has accepted or
// that passed Test. A running core is the check: it parses a reload in
// full before it changes anything, and refuses with what `mihomo -t`
// would say, staying on the config it has. That spares a second process
// that loads the geodata again for every switch flipped (#60). When the
// reload is not plainly accepted (refused, or no answer in time), Test
// says whether the config is at fault.
func (m *Manager) Apply(cfg []byte) error {
	m.applyMu.Lock()
	defer m.applyMu.Unlock()
	var t ApplyTiming
	defer func() {
		m.mu.Lock()
		m.timing = t
		m.mu.Unlock()
	}()
	tmp := filepath.Join(m.Dir, "config.next.yaml")
	if err := os.WriteFile(tmp, cfg, 0o600); err != nil {
		return err
	}
	test := func() error {
		defer since(&t.Check, time.Now())
		return m.Test(tmp)
	}
	c, err := m.Client()
	if err != nil { // no core to ask
		if err := test(); err != nil {
			return err
		}
		if err := os.Rename(tmp, m.ConfigPath()); err != nil {
			return err
		}
		m.mu.Lock()
		m.loading = false
		m.mu.Unlock()
		defer since(&t.Start, time.Now())
		if err := m.start(); err != nil {
			return err
		}
		m.setLoaded(cfg)
		return nil
	}
	wait := m.ReloadWait
	if wait <= 0 {
		wait = 15 * time.Second // what the client waits for any answer
	}
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	m.mu.Lock()
	listenErr := m.listenErr
	m.mu.Unlock()
	m.setListenErr("") // a reload tries a port again, and says so if it fails
	m.setTunErr("")    // and the TUN device
	began := time.Now()
	err = c.Reload(ctx, tmp)
	since(&t.Reload, began)
	slow := false
	if err != nil {
		if terr := test(); terr != nil {
			m.setListenErr(listenErr) // the core is as it was
			return terr
		}
		// The core answers a reload only once it has fetched every remote
		// file the config names for the first time (a rule set, a
		// subscription of the config's own), and one it can't reach takes
		// it 20 s. The config has passed the check, so a core that
		// outlasts the wait and still answers is loading it, not refusing
		// it: Loading says so until a reload is answered again.
		slow = isTimeout(err) && m.answers(c)
	}
	if rerr := os.Rename(tmp, m.ConfigPath()); rerr != nil {
		return rerr
	}
	m.mu.Lock()
	m.loading = slow
	m.mu.Unlock()
	if slow {
		m.setLoaded(nil) // not in full yet
		return nil
	}
	if err != nil {
		m.setLoaded(nil) // the core may have it, or not
		return err
	}
	m.setLoaded(cfg)
	return nil
}

// Loaded reports whether the running core has cfg loaded already, in full
// and with nothing of it failed that a reload would try again (a port it
// could not open, its TUN device). Applying cfg again would change nothing
// but make the core build everything anew, and with that test every node of
// every provider once more: mihomo checks a provider's nodes on each URL its
// groups test whenever it loads the provider, lazy or not (#200). A change
// that only the core's files carry (a provider's nodes) is not in cfg, so
// what applies one must not ask this.
func (m *Manager) Loaded(cfg []byte) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.client != nil && m.loaded != nil && !m.loading && m.listenErr == "" && m.tunErr == "" && bytes.Equal(m.loaded, cfg)
}

func (m *Manager) setLoaded(cfg []byte) {
	m.mu.Lock()
	m.loaded = slices.Clone(cfg)
	m.mu.Unlock()
}

// ApplyTiming is where the last Apply spent its time. Check is zero when
// the running core accepted the config itself.
type ApplyTiming struct {
	Check  time.Duration // `mihomo -t`
	Reload time.Duration // the running core's answer to the reload
	Start  time.Duration // starting a core that was not running
}

// Timing is the last Apply's.
func (m *Manager) Timing() ApplyTiming {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.timing
}

func since(d *time.Duration, t time.Time) { *d = time.Since(t) }

// Loading says whether the core was still fetching what the last config
// names when Apply stopped waiting for it.
func (m *Manager) Loading() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loading
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout())
}

func (m *Manager) answers(c *mihomo.Client) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := c.Version(ctx)
	return err == nil
}

var coreLogLine = regexp.MustCompile(`level=(error|fatal) msg=("(?:[^"\\]|\\.)*")`)

// coreSaid is why the core refused a config, out of what `mihomo -t`
// printed: the messages of its error lines without their timestamps, or
// failing that the last lines as they are.
func coreSaid(out string) string {
	var msgs []string
	for _, m := range coreLogLine.FindAllStringSubmatch(out, -1) {
		if s, err := strconv.Unquote(m[2]); err == nil {
			msgs = append(msgs, s)
		}
	}
	if len(msgs) == 0 {
		return lastLines(out, 3)
	}
	return strings.Join(msgs, "\n")
}

// Restart stops the core and starts it again with the current config.
func (m *Manager) Restart() error {
	m.Stop()
	return m.start()
}

func (m *Manager) start() error {
	err := m.run()
	if err != nil && !errors.Is(err, ErrClosed) {
		return &StartError{err}
	}
	return err
}

func (m *Manager) run() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrClosed
	}
	if m.cmd != nil {
		m.mu.Unlock()
		return nil
	}
	m.stopping = false
	m.loaded = nil // what Apply started it with, it says once it has
	m.mu.Unlock()

	if _, err := os.Stat(m.source()); err != nil {
		err = fmt.Errorf(i18n.T("找不到 mihomo 内核 (%s)，先运行 scripts/fetch-core.sh", "No mihomo core at %s; run scripts/fetch-core.sh first"), m.source())
		m.setStatus(Status{State: Failed, Error: err.Error()})
		return err
	}
	m.endStray()
	m.setListenErr("")
	m.setTunErr("")
	if m.socket != "" {
		_ = os.Remove(m.socket)
	}
	m.setStatus(Status{State: Starting})

	fail := func(err error) error {
		m.setStatus(Status{State: Failed, Error: err.Error()})
		return err
	}
	var cmd Process
	var tail *logTail
	if m.Spawn != nil {
		p, err := m.Spawn(m.Bin)
		if err != nil {
			return fail(err)
		}
		cmd = p // nil when bin is not Spawn's to start
	}
	if cmd == nil {
		// Into a file and not a pipe to the app: a core that outlives the
		// app (Detach) would be killed by its next line on a pipe no one
		// reads.
		f, err := openLog(m.logPath())
		if err != nil {
			return fail(err)
		}
		p, err := startCore(m.Bin, []string{"-d", m.Dir, "-f", m.ConfigPath()}, m.Dir, m.env(), f)
		f.Close()
		if err != nil {
			return fail(err)
		}
		cmd, tail = p, m.newTail(false)
	}
	_ = os.WriteFile(m.pidFile(), []byte(strconv.Itoa(cmd.Pid())), 0o600)
	return m.watch(cmd, tail)
}

// watch takes cmd, a core that has just started or is being taken over,
// as the manager's, and waits for its controller. tail reads what the core
// prints to its log file; without one the core prints to no one (Spawn's),
// and its log is streamed from the controller.
func (m *Manager) watch(cmd Process, tail *logTail) error {
	spawned := tail == nil
	var pw *io.PipeWriter
	if spawned {
		var pr *io.PipeReader
		pr, pw = io.Pipe()
		go m.readLogs(pr)
	}
	exited := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.cmd, m.spawned, m.exited, m.cancel = cmd, spawned, exited, cancel
	closed := m.closed
	m.mu.Unlock()
	if tail != nil {
		go tail.follow(ctx)
	}

	go func() {
		err := cmd.Wait()
		cancel()
		if pw != nil {
			pw.Close()
		}
		// The status changes before exited closes, so that Stop and start
		// see it once they return.
		defer close(exited)
		m.mu.Lock()
		if m.detached { // no longer this manager's: the next run's, or gone with it
			m.mu.Unlock()
			return
		}
		stopping := m.stopping
		m.cmd, m.spawned, m.client, m.loaded = nil, false, nil, nil
		m.mu.Unlock()
		if tail != nil {
			tail.read() // its last words, for the message below
		}
		_ = os.Remove(m.pidFile())
		if stopping {
			m.setStatus(Status{State: Stopped})
		} else {
			msg := i18n.T("内核意外退出", "The core quit unexpectedly")
			if err != nil {
				msg += ": " + err.Error()
			}
			if l := m.logs.last(2); l != "" {
				msg += "\n" + l
			}
			m.setStatus(Status{State: Failed, Error: msg})
		}
	}()

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
			m.setStatus(Status{State: Running, Version: v.Version, PID: cmd.Pid(), StartedAt: time.Now()})
			if spawned {
				go m.streamLogs(ctx, client, pw)
			}
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
// and the next start's endStray takes care of it.
func (m *Manager) Stop() {
	m.mu.Lock()
	cmd, exited, cancel := m.cmd, m.exited, m.cancel
	m.stopping = true
	m.mu.Unlock()
	if cmd == nil {
		return
	}
	cancel()
	cmd.Stop()
	select {
	case <-exited:
		return
	case <-time.After(stopGrace):
	}
	insist(cmd)
	select {
	case <-exited:
	case <-time.After(killGrace):
	}
}

// insist ends a core that did not go when asked: killed where it can be,
// else asked a second time, which is how the others insist.
func insist(p Process) {
	if k, ok := p.(interface{ kill() }); ok {
		k.kill()
	} else {
		p.Stop()
	}
}

// Close stops the core for good: nothing starts it again, even an Apply
// under way. For when the app quits.
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	stray := m.cmd == nil && !m.detached
	m.mu.Unlock()
	m.Stop()
	if stray {
		// A core an earlier run left that this one never came to start
		// over or take over: quitting leaves no core behind.
		m.endStray()
	}
}

// endStray ends a core a previous run of the app left behind (after a
// crash, or one of another version than this one takes over), when the
// pid file still names a core. It waits for it to be gone, as Stop does:
// the core that starts next needs the ports this one still holds.
func (m *Manager) endStray() {
	pid := liveCore(m.pidFile())
	if pid == 0 {
		return
	}
	p, _ := m.attach(pid)
	p.Stop()
	if !gone(pid, stopGrace) {
		insist(p)
		if !gone(pid, killGrace) {
			return // the pid file stays, for the next try
		}
	}
	_ = os.Remove(m.pidFile())
}

// attach is the running core pid as a Process, and whether it is an
// ordinary one, which prints to the log file.
func (m *Manager) attach(pid int) (p Process, plain bool) {
	if m.Attach != nil {
		if p := m.Attach(pid); p != nil {
			return p, false
		}
	}
	return adopted{pid}, true
}

// gone waits up to d for the process to end, and says whether it did.
func gone(pid int, d time.Duration) bool {
	for end := time.Now().Add(d); alive(pid); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(end) {
			return false
		}
	}
	return true
}

// Left is the pid of the core running in dir, as its pid file names it,
// and 0 when none runs: for telling whether a core that was let go of
// (Detach) is still there.
func Left(dir string) int { return liveCore(filepath.Join(dir, "mihomo.pid")) }

// Detach lets go of the running core without ending it, so that the next
// run of the app can take it over (TakeOver) and nothing it carries is
// cut: for a restart into a new version. It returns the core's pid, and
// the manager is then closed as by Close, though the core runs on. It
// returns 0, having changed nothing, when there is no running core to
// leave.
func (m *Manager) Detach() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.cmd == nil || m.client == nil || m.status.State != Running {
		return 0
	}
	pid := m.cmd.Pid()
	m.closed, m.detached = true, true
	m.cancel() // the log, the traffic and the controller's streams
	m.cmd, m.spawned, m.client, m.loaded = nil, false, nil, nil
	m.status = Status{State: Stopped}
	return pid
}

// TakeOver makes the core that a previous run left running (Detach) this
// manager's, and reports whether it did. pid is the core that run named,
// and want the version the core must state ("" for any): a core of another
// version than the one this manager would start is left alone, for the
// next start to replace (endStray). It is for before anything has been
// started. The config the core has is not known: the next Apply reloads
// it.
func (m *Manager) TakeOver(pid int, want string) bool {
	m.mu.Lock()
	busy := m.closed || m.cmd != nil
	m.mu.Unlock()
	if busy || pid <= 1 || liveCore(m.pidFile()) != pid {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c := m.newClient()
	v, err := c.Version(ctx)
	if err != nil || proxiesLoaded(ctx, c) != nil || (want != "" && v.Version != want) {
		return false
	}
	m.mu.Lock()
	m.stopping, m.loaded, m.loading = false, nil, false
	m.mu.Unlock()
	m.setListenErr("")
	m.setTunErr("")
	p, plain := m.attach(pid)
	var tail *logTail
	if plain {
		tail = m.newTail(true) // what it said before is the last run's
	}
	return m.watch(p, tail) == nil
}

// logPath is the file the core prints to.
func (m *Manager) logPath() string { return filepath.Join(m.Dir, "core.log") }

// openLog empties the log file and opens it for a core to append to, so
// that each of its lines goes at the end, also after logTail emptied it.
// It is emptied apart: Windows appends only on a handle that cannot write
// elsewhere in the file, and one that truncates can.
func openLog(path string) (*os.File, error) {
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
}

// maxLogFile is the size at which the core's log file is emptied: the
// last lines are kept in memory, and the file is only how they get here.
const maxLogFile = 4 << 20

// logTail reads what the core appends to its log file.
type logTail struct {
	m    *Manager
	mu   sync.Mutex
	off  int64
	rest []byte // a line not ended yet
}

// newTail reads the log file from its start, or with fromEnd only what is
// added from now on.
func (m *Manager) newTail(fromEnd bool) *logTail {
	t := &logTail{m: m}
	if st, err := os.Stat(m.logPath()); err == nil && fromEnd {
		t.off = st.Size()
	}
	return t
}

// follow reads the lines as they come, until ctx ends.
func (t *logTail) follow(ctx context.Context) {
	tick := time.NewTicker(150 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			t.read()
		}
	}
}

// read takes in the lines added since it last ran.
func (t *logTail) read() {
	t.mu.Lock()
	defer t.mu.Unlock()
	path := t.m.logPath()
	st, err := os.Stat(path)
	if err != nil {
		return
	}
	if st.Size() < t.off { // emptied
		t.off, t.rest = 0, nil
	}
	if st.Size() == t.off {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	buf := make([]byte, min(st.Size()-t.off, 1<<20))
	n, _ := f.ReadAt(buf, t.off)
	t.off += int64(n)
	data := append(t.rest, buf[:n]...)
	for {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			break
		}
		t.m.takeLine(strings.TrimRight(string(data[:i]), "\r"))
		data = data[i+1:]
	}
	t.rest = append([]byte(nil), data...)
	// The core appends, so its next line starts the emptied file.
	if t.off > maxLogFile && os.Truncate(path, 0) == nil {
		t.off, t.rest = 0, nil
	}
}

// Logs returns the latest lines the core printed.
func (m *Manager) Logs() []string { return m.logs.all() }

// Subscribe calls fn with every new line until the returned function is
// called.
func (m *Manager) Subscribe(fn func(string)) (off func()) { return m.logs.subscribe(fn) }

// LogStreamed reports whether the manager streams the running core's log
// from its controller and passes each entry to OnLog: a core Spawn
// started, which prints to no one. Whoever would stream that log too
// should take it from OnLog instead, or the core encodes and sends every
// entry twice for as long as it runs (#200). It holds from the OnStatus
// call that says the core is running.
func (m *Manager) LogStreamed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cmd != nil && m.spawned && m.OnLog != nil
}

// streamLogs writes what the controller says of a core that prints to no
// one where readLogs reads it, and passes it to OnLog, until ctx ends (the
// core exited or is being stopped). What the core said before its
// controller answered is lost.
func (m *Manager) streamLogs(ctx context.Context, c *mihomo.Client, w io.Writer) {
	for ctx.Err() == nil {
		_ = c.StreamLogs(ctx, "info", func(l mihomo.Log) {
			fmt.Fprintf(w, "level=%s msg=%q\n", l.Type, l.Payload)
			if fn := m.OnLog; fn != nil {
				fn(l)
			}
		})
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
}

func (m *Manager) readLogs(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		m.takeLine(sc.Text())
	}
}

// takeLine is one line of what the core said, from its log file or its
// controller.
func (m *Manager) takeLine(line string) {
	m.logs.add(line)
	m.noteListenError(line)
	m.noteTunError(line)
}

// tunListenFailed is what mihomo logs for a TUN device it cannot bring
// up (no right to make one: a core that is not the installed copy, or
// that Windows' task did not start). Like a port it cannot open, it goes
// on without it. tunListening is its line for a device that came up.
var (
	tunListenFailed = regexp.MustCompile(`Start TUN listening error: (.*?)"?\s*$`)
	tunListening    = regexp.MustCompile(`\[TUN\] Tun adapter listening at`)
)

func (m *Manager) noteTunError(line string) {
	if g := tunListenFailed.FindStringSubmatch(line); g != nil {
		m.setTunErr(g[1])
	} else if tunListening.MatchString(line) {
		m.setTunErr("")
	}
}

func (m *Manager) setTunErr(msg string) {
	m.mu.Lock()
	changed := m.tunErr != msg
	m.tunErr = msg
	fn := m.OnListenError
	m.mu.Unlock()
	if changed && fn != nil {
		fn()
	}
}

// TunUp asks the running core whether its TUN device is up: its answer
// is the listener as it is, whatever the config asked for. known is false
// when there is no core to ask or it did not answer.
func (m *Manager) TunUp(ctx context.Context) (up, known bool) {
	c, err := m.Client()
	if err != nil {
		return false, false
	}
	cfg, err := c.Config(ctx)
	if err != nil {
		return false, false
	}
	return cfg.Tun.Enable, true
}

// proxyListenFailed is what mihomo logs for a proxy port it cannot open. It
// goes on without the port, its controller answering as usual, so this line
// is all there is to tell that nothing is listening. The DNS listener and
// the like are not the proxy, and have their own line.
var proxyListenFailed = regexp.MustCompile(`(?:Start (?:Mixed\(http\+socks\)|HTTP|SOCKS) server error|Listener bowel-port-\d+ listen err): listen \S+ \S*:(\d+): bind:`)

func (m *Manager) noteListenError(line string) {
	if g := proxyListenFailed.FindStringSubmatch(line); g != nil {
		m.setListenErr(fmt.Sprintf(i18n.T("端口 %s 被其他程序占用或被系统保留，代理没有在监听。关掉那个程序后重启 bowel，或换一个端口", "Port %s is taken by another program or reserved by the system, so the proxy isn't listening. Close that program and restart bowel, or choose another port"), g[1]))
	}
}

func (m *Manager) setListenErr(msg string) {
	m.mu.Lock()
	changed := m.listenErr != msg
	m.listenErr = msg
	fn := m.OnListenError
	m.mu.Unlock()
	if changed && fn != nil {
		fn()
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

// FreePort returns a loopback port nothing listens on now, by TCP or UDP.
func FreePort() (int, error) { return freePort() }

// listenUDP is net.ListenPacket; a test has it refuse ports.
var listenUDP = net.ListenPacket

// freePortTries is how many ports freePort tries before it gives up.
const freePortTries = 32

// freePort takes a loopback TCP port from the system and keeps it only if
// UDP can have it too, since a mixed listener such as the location
// probe's needs both. Windows reserves blocks of ports for UDP alone
// (Hyper-V, WSL and Docker ask for them) and still hands out the same
// numbers for TCP; the core then fails to listen on them. Windows also
// hands its ports out one after another, even across processes, and such
// a block can run several hundred long, so asking it again would only
// walk through the block. Each later try is drawn at random from the
// dynamic range instead.
func freePort() (int, error) {
	var err error
	port := 0
	for range freePortTries {
		var p int
		if p, err = bindBoth(port); err == nil {
			return p, nil
		}
		port = 49152 + mrand.IntN(65536-49152)
	}
	return 0, fmt.Errorf("no loopback port binds for both tcp and udp: %w", err)
}

// bindBoth listens on port of 127.0.0.1 for TCP, 0 letting the system
// choose, and then for UDP on that same port while it holds the first. It
// releases both and returns the port.
func bindBoth(port int) (int, error) {
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return 0, err
	}
	defer l.Close()
	port = l.Addr().(*net.TCPAddr).Port
	u, err := listenUDP("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return 0, err
	}
	u.Close()
	return port, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
