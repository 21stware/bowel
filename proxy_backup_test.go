package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/mcp"
	"github.com/oboo/moho/internal/settings"
	"github.com/oboo/moho/internal/sysproxy"
)

// TestProxyBackup: enabling keeps what the system had, without Moho's own
// leftovers; disabling puts that back and leaves no file behind. All of it
// through fakes: the machine's proxy is never touched.
func TestProxyBackup(t *testing.T) {
	port, err := core.FreePort() // never the running app's port
	if err != nil {
		t.Fatal(err)
	}
	before := sysproxy.Saved{Services: []sysproxy.Service{
		{Name: "Wi-Fi", Web: sysproxy.Endpoint{On: true, Host: "proxy.corp", Port: 8080}, Bypass: []string{"corp"}},
		{Name: "Ethernet", Web: sysproxy.Endpoint{Host: "127.0.0.1", Port: port}, Bypass: []string{"*.local"}},
	}}
	var enabled, disabled, snapshots int
	var restored []sysproxy.Saved
	saved := osProxy
	defer func() { osProxy = saved }()
	osProxy.enable = func(host string, p int, _ []string) error {
		if host != "127.0.0.1" || p != port {
			t.Errorf("enable %s:%d", host, p)
		}
		enabled++
		return nil
	}
	osProxy.disable = func() error { disabled++; return nil }
	osProxy.snapshot = func() (sysproxy.Saved, error) { snapshots++; return before, nil }
	osProxy.restore = func(_ context.Context, s sysproxy.Saved) error { restored = append(restored, s); return nil }

	a := &App{dataDir: t.TempDir()}
	s := settings.Settings{MixedPort: port}
	if err := a.enableProxy(s); err != nil {
		t.Fatal(err)
	}
	if enabled != 1 || snapshots != 1 || !exists(a.proxyBackup()) {
		t.Fatalf("enable: enabled %d, snapshots %d, backup %v", enabled, snapshots, exists(a.proxyBackup()))
	}
	if err := a.disableProxy(context.Background(), port); err != nil {
		t.Fatal(err)
	}
	want := sysproxy.Saved{Services: []sysproxy.Service{
		before.Services[0],
		{Name: "Ethernet", Bypass: sysproxy.DefaultBypass}, // Moho's leftover is cleared, with its bypass list
	}}
	if len(restored) != 1 || !reflect.DeepEqual(restored[0], want) {
		t.Fatalf("restored %+v\nwant %+v", restored, want)
	}
	if disabled != 0 || exists(a.proxyBackup()) {
		t.Fatalf("after restore: disabled %d, backup left %v", disabled, exists(a.proxyBackup()))
	}
	// With no copy kept, disabling just turns the proxies off.
	if err := a.disableProxy(context.Background(), port); err != nil {
		t.Fatal(err)
	}
	if disabled != 1 || len(restored) != 1 {
		t.Fatalf("no backup: disabled %d, restored %d", disabled, len(restored))
	}
}

func lanProxy(host string, port int, on bool) sysproxy.Saved {
	e := sysproxy.Endpoint{On: on, Host: host, Port: port}
	return sysproxy.Saved{Services: []sysproxy.Service{{Name: "LAN", Web: e, Secure: e, Bypass: []string{"corp"}}}}
}

func writeBackup(t *testing.T, a *App, s sysproxy.Saved) {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a.proxyBackup(), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readBackup(t *testing.T, a *App) sysproxy.Saved {
	t.Helper()
	b, err := os.ReadFile(a.proxyBackup())
	if err != nil {
		t.Fatal(err)
	}
	var s sysproxy.Saved
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

// retakeApp is mcpApp with takeover on a port of the test's own.
func retakeApp(t *testing.T) *App {
	t.Helper()
	a, _, _ := mcpApp(t)
	port, err := core.FreePort() // never the running app's port
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.settings.Update(func(s *settings.Settings) { s.MixedPort, s.SystemProxy, s.Paused = port, true, false }); err != nil {
		t.Fatal(err)
	}
	return a
}

// TestRetakeKeepsFirstCopy: what another program left is not what the user
// had before bowel. With no copy yet, retaking keeps one as enabling does.
func TestRetakeKeepsFirstCopy(t *testing.T) {
	a := retakeApp(t)
	first := lanProxy("proxy.corp", 8080, true)
	writeBackup(t, a, first)
	osProxy.snapshot = func() (sysproxy.Saved, error) { return lanProxy("127.0.0.1", 7890, true), nil }
	enabled := 0
	osProxy.enable = func(string, int, []string) error { enabled++; return nil }
	if err := a.retake(); err != nil {
		t.Fatal(err)
	}
	if got := readBackup(t, a); enabled != 1 || !reflect.DeepEqual(got, first) {
		t.Fatalf("enabled %d, copy %+v", enabled, got)
	}
	a.dropProxyBackup()
	if err := a.retake(); err != nil {
		t.Fatal(err)
	}
	if got := readBackup(t, a); enabled != 2 || got.Services[0].Web.Port != 7890 {
		t.Fatalf("no copy before: enabled %d, copy %+v", enabled, got)
	}
}

// TestRetakeWhileQuitting: shutdown has put the system proxy back.
func TestRetakeWhileQuitting(t *testing.T) {
	a := retakeApp(t)
	a.quitting = true
	if err := a.retake(); err != nil {
		t.Fatal(err) // the fakes fail the test if it touches the system
	}
}

// TestRestoreTurnsOffDead: a local proxy nothing serves any more comes back
// off, with its address; a remote one comes back as it was.
func TestRestoreTurnsOffDead(t *testing.T) {
	saved := osProxy
	defer func() { osProxy = saved }()
	var restored []sysproxy.Saved
	osProxy.restore = func(_ context.Context, s sysproxy.Saved) error { restored = append(restored, s); return nil }
	osProxy.listening = func(_ context.Context, _ string, port int) bool { return port == 7000 }
	a := &App{dataDir: t.TempDir()}
	for _, c := range []struct {
		host string
		port int
		on   bool
	}{{"127.0.0.1", 7890, false}, {"127.0.0.1", 7000, true}, {"proxy.corp", 8080, true}} {
		restored = nil
		writeBackup(t, a, lanProxy(c.host, c.port, true))
		if err := a.disableProxy(context.Background(), 61999); err != nil {
			t.Fatal(err)
		}
		if want := lanProxy(c.host, c.port, c.on); len(restored) != 1 || !reflect.DeepEqual(restored[0], want) {
			t.Errorf("%s:%d: restored %+v, want %+v", c.host, c.port, restored, want)
		}
	}
}

// TestEnableFailureIsOurs: when bowel could not take the system proxy, the
// status says so with the reason, and does not blame another program.
func TestEnableFailureIsOurs(t *testing.T) {
	a := retakeApp(t)
	fail := errors.New("InternetSetOption 75: The parameter is incorrect.")
	osProxy.enable = func(string, int, []string) error { return fail }
	if err := a.retake(); !errors.Is(err, fail) {
		t.Fatalf("retake: %v", err)
	}
	snap := a.snapshot()
	if sys := snap.Takeover.System; sys.Status != "error" || sys.Detail != fail.Error() {
		t.Fatalf("system: %+v", sys)
	}
	if snap.Issue == nil || snap.Issue.Action != "retake" || strings.Contains(snap.Issue.Text, "其他程序") || snap.Issue.Detail != fail.Error() {
		t.Fatalf("issue: %+v", snap.Issue)
	}
	osProxy.enable = func(string, int, []string) error { return nil }
	if err := a.retake(); err != nil {
		t.Fatal(err)
	}
	if sys := a.snapshot().Takeover.System; sys.Status != "hijacked" {
		t.Fatalf("after a good try the system's state decides (the fake changed nothing): %+v", sys)
	}
}

// TestMCPRetake: the tool follows the policy, and only acts when the system
// proxy is wanted.
func TestMCPRetake(t *testing.T) {
	a, asks, answer := mcpApp(t)
	port, err := core.FreePort()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.settings.Update(func(s *settings.Settings) { s.MixedPort, s.SystemProxy, s.Paused = port, false, false }); err != nil {
		t.Fatal(err)
	}
	srv, s := a.mcpServer(), &mcp.Session{}
	enabled := 0
	osProxy.enable = func(string, int, []string) error { enabled++; return nil }
	if _, err := callTool(t, srv, s, "retake_system_proxy", nil); err == nil || enabled != 0 || *asks != 0 {
		t.Fatalf("not wanted: err %v, enabled %d, asks %d", err, enabled, *asks)
	}
	if _, err := a.settings.Update(func(s *settings.Settings) { s.SystemProxy = true }); err != nil {
		t.Fatal(err)
	}
	setPolicy(t, a, "deny")
	if _, err := callTool(t, srv, s, "retake_system_proxy", nil); err == nil || enabled != 0 {
		t.Fatalf("denied: err %v, enabled %d", err, enabled)
	}
	setPolicy(t, a, "ask")
	answer[0] = false
	if _, err := callTool(t, srv, s, "retake_system_proxy", nil); err == nil || enabled != 0 || *asks != 1 {
		t.Fatalf("refused: err %v, enabled %d, asks %d", err, enabled, *asks)
	}
	answer[0] = true
	out, err := callTool(t, srv, s, "retake_system_proxy", nil)
	if err != nil || enabled != 1 || *asks != 2 || out["takeover"] == nil {
		t.Fatalf("allowed: %v, err %v, enabled %d, asks %d", out, err, enabled, *asks)
	}
}
