package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

func lockedSettings(s settings.Settings) settings.Settings {
	s = s.Clone()
	s.Routing.SetLocked("ai", true)
	return s
}

// lockApp is an app whose TUN has its installation, on a core that has
// not run.
func lockApp(t *testing.T, bin string) *App {
	t.Helper()
	saved := osTun
	t.Cleanup(func() { osTun = saved })
	osTun.installed = func(string) bool { return true }
	osTun.needsRule = func(string) bool { return false }
	m, err := core.New(bin, filepath.Join(t.TempDir(), "core"))
	if err != nil {
		t.Fatal(err)
	}
	return &App{core: m, coreBin: "/bundle", tunBin: "/tun"}
}

// The system proxy points at the core for takeover by it, and for the exit
// lock while takeover is off; never under a provider's own routing, which
// has none of the groups. TUN alone that works needs no system proxy.
func TestProxyWantedByTheLock(t *testing.T) {
	a := lockApp(t, "/tun")
	for _, c := range []struct {
		name        string
		s           settings.Settings
		guard, want bool
	}{
		{"on, system proxy", settings.Settings{SystemProxy: true}, false, true},
		{"on, TUN only", settings.Settings{Tun: true}, false, false},
		{"paused", settings.Settings{SystemProxy: true, Paused: true}, false, false},
		{"paused, locked", lockedSettings(settings.Settings{SystemProxy: true, Paused: true}), true, true},
		{"no entry, locked", lockedSettings(settings.Settings{}), true, true},
		{"TUN only, locked", lockedSettings(settings.Settings{Tun: true}), false, false},
		{"paused, locked, imported", func() settings.Settings {
			s := lockedSettings(settings.Settings{Paused: true})
			s.ImportedRouting = "p1"
			return s
		}(), false, false},
	} {
		if got := a.guarding(c.s); got != c.guard {
			t.Errorf("%s: guarding %v, want %v", c.name, got, c.guard)
		}
		if got := a.proxyWanted(c.s); got != c.want {
			t.Errorf("%s: proxyWanted %v, want %v", c.name, got, c.want)
		}
	}
}

// Takeover by TUN alone, a group locked, and TUN lost: nothing would bring
// that group's traffic to the core, so the system proxy is wanted until
// TUN is back. Guard stays off: the group goes by its node, as takeover
// has it.
func TestLockFallsBackWhenTunIsLost(t *testing.T) {
	tunOnly := lockedSettings(settings.Settings{Tun: true})
	running, stopped := core.Status{State: core.Running}, core.Status{State: core.Stopped}

	// The core TUN runs on did not start: the bundled one runs without TUN.
	a := lockApp(t, "/tun")
	if a.lockFallback(tunOnly, stopped) || a.proxyWanted(tunOnly) {
		t.Fatal("fell back before anything was lost")
	}
	if !a.leaveTunCore(&core.StartError{Err: errors.New("the task didn't start the core")}) {
		t.Fatal("the TUN core was not left")
	}
	if !a.tunLost(tunOnly, running) || !a.lockFallback(tunOnly, running) || !a.proxyWanted(tunOnly) {
		t.Error("TUN's core did not start and the lock has nothing to hold by")
	}
	if a.guarding(tunOnly) {
		t.Error("takeover is on: the core is not to send the rest direct")
	}
	for name, s := range map[string]settings.Settings{
		"no group locked":     {Tun: true},
		"system proxy on too": lockedSettings(settings.Settings{Tun: true, SystemProxy: true}),
		"paused":              lockedSettings(settings.Settings{Tun: true, Paused: true}),
		"a provider's routing": func() settings.Settings {
			s := tunOnly.Clone()
			s.ImportedRouting = "p1"
			return s
		}(),
	} {
		if a.lockFallback(s, running) {
			t.Errorf("%s: fell back", name)
		}
	}
	// Trying TUN again ends it.
	a.retryTunCore()
	if a.lockFallback(tunOnly, stopped) || a.proxyWanted(tunOnly) {
		t.Error("still fallen back after TUN was tried again")
	}

	// A core that runs and could not bring the device up.
	a = lockApp(t, "/tun")
	if down := (core.Status{State: core.Running, TunError: "Access is denied"}); !a.lockFallback(tunOnly, down) {
		t.Error("the device is down and the lock has nothing to hold by")
	}
	if a.lockFallback(tunOnly, running) {
		t.Error("fell back with nothing said against the device")
	}
	// A core that is not running at all; one that is starting has lost nothing yet.
	if !a.lockFallback(tunOnly, core.Status{State: core.Failed}) || a.lockFallback(tunOnly, core.Status{State: core.Starting}) {
		t.Error("failed core: fallback; starting core: none")
	}

	// TUN with no installation.
	osTun.installed = func(string) bool { return false }
	if !a.lockFallback(tunOnly, running) {
		t.Error("TUN has no installation and the lock has nothing to hold by")
	}
}

// The takeover's word on the lock follows what holds it.
func TestLockEntry(t *testing.T) {
	if e := lockEntry(false, true, false, false, ""); e != nil {
		t.Errorf("no group locked: %+v", e)
	}
	if e := lockEntry(true, false, false, true, ""); e != nil {
		t.Errorf("takeover carries it: %+v", e)
	}
	if e := lockEntry(true, true, false, true, ""); e == nil || e.Status != "on" || e.Detail != "" {
		t.Errorf("guard, held: %+v", e)
	}
	if e := lockEntry(true, true, false, false, ""); e == nil || e.Status != "error" || !strings.Contains(e.Detail, "拦不住") {
		t.Errorf("guard, not held: %+v", e)
	}
	e := lockEntry(true, false, true, true, "")
	if e == nil || e.Status != "on" || !strings.Contains(e.Detail, "TUN") || !strings.Contains(e.Detail, "系统代理") {
		t.Fatalf("fallback, held: %+v", e)
	}
	if is := lockIssue(e); is == nil || is.Level != "warn" || is.Detail != e.Detail {
		t.Errorf("fallback, held: issue %+v", is)
	}
	e = lockEntry(true, false, true, false, "")
	if e == nil || e.Status != "error" || !strings.Contains(e.Detail, "直连") {
		t.Fatalf("fallback, not held: %+v", e)
	}
	if e := lockEntry(true, false, true, false, "没有权限"); e == nil || e.Status != "error" || e.Detail != "没有权限" {
		t.Errorf("fallback, the write failed: %+v", e)
	}
	// Takeover by the system proxy, which another program took.
	if e := lockEntry(true, false, false, false, ""); e == nil || e.Status != "error" {
		t.Errorf("hijacked: %+v", e)
	}
}

// With takeover on, a lock that doesn't hold as set is the issue: an error
// when nothing holds it, a warning while the system proxy stands in.
func TestJudgeSaysLockNotHeld(t *testing.T) {
	a := &App{}
	s := lockedSettings(settings.Settings{Tun: true, Mode: "rule"})
	cs := core.Status{State: core.Running}
	snap := Snapshot{Nodes: []NodeView{{Name: "n"}}}
	tk := Takeover{On: true, Lock: lockEntry(true, false, true, false, "")}
	if st, is := a.judge(true, s, cs, tk, snap, nil); st != StateError || is == nil || is.Detail != tk.Lock.Detail {
		t.Errorf("not held: %v %+v", st, is)
	}
	tk.Lock = lockEntry(true, false, true, true, "")
	if st, is := a.judge(true, s, cs, tk, snap, nil); st != StateDegraded || is == nil || is.Level != "warn" {
		t.Errorf("held by the system proxy: %v %+v", st, is)
	}
	tk.Lock = nil
	if st, is := a.judge(true, s, cs, tk, snap, nil); st != StateOK || is != nil {
		t.Errorf("held: %v %+v", st, is)
	}
}

// In a container the ports are how traffic comes in, takeover or not, so
// the status is judged as under a takeover that runs though takeover (the
// system proxy, refused there, or TUN) is off: a core that fails, a port
// that won't open and a locked group with no node are said. The ports
// hold the lock, so what is said of it is the refusing. Elsewhere,
// takeover off is off.
func TestJudgeInContainer(t *testing.T) {
	a := &App{}
	s := lockedSettings(settings.Settings{Mode: "rule"})
	running := core.Status{State: core.Running}
	judge := func(s settings.Settings, cs core.Status, snap Snapshot) (State, *Issue) {
		// The lock's entry as the snapshot makes it, with no system proxy and no TUN.
		tk := Takeover{Lock: containerLock(lockEntry(locks(s), a.guarding(s), a.lockFallback(s, cs), portsHold(s), ""))}
		return a.judge(a.takeoverOn(s), s, cs, tk, snap, nil)
	}
	noNode := Snapshot{Routes: []RouteView{{ID: "ai", Name: "AI 服务", Locked: true}}}

	t.Setenv(containerEnv, "")
	if st, is := judge(s, core.Status{State: core.Failed, Error: "boom"}, noNode); st != StateOff || is != nil {
		t.Errorf("outside a container, takeover off: %v %+v", st, is)
	}

	t.Setenv(containerEnv, "1")
	if st, is := judge(s, running, noNode); st != StateDegraded || is == nil || !strings.Contains(is.Detail, "AI 服务") || !strings.Contains(is.Detail, "拦下") {
		t.Errorf("locked group, no node at all: %v %+v", st, is)
	}
	dead := NodeView{Name: "n", Display: "n", Tested: true, Routes: map[string]NodeTest{}}
	refused := Snapshot{Nodes: []NodeView{dead}, Routes: []RouteView{{ID: "ai", Name: "AI 服务", Broken: "fixed", Locked: true, Policy: routes.Policy{Mode: routes.ModeFixed, Fixed: "n"}}}}
	if st, is := judge(s, running, refused); st != StateDegraded || is == nil || is.Detail != lockedOutSay() || is.Action != "fix:ai" {
		t.Errorf("locked group, its node down: %v %+v", st, is)
	}
	if st, is := judge(s, core.Status{State: core.Failed, Error: "exit status 1\nmore"}, noNode); st != StateError || is == nil || is.Detail != "exit status 1" {
		t.Errorf("core failed: %v %+v", st, is)
	}
	if st, is := judge(s, core.Status{State: core.Running, Error: "listen tcp :7899: bind: address already in use"}, noNode); st != StateError || is == nil || !strings.Contains(is.Detail, "7899") {
		t.Errorf("port won't open: %v %+v", st, is)
	}
	paused := s.Clone()
	paused.Paused = true
	if st, is := judge(paused, core.Status{State: core.Failed, Error: "boom"}, noNode); st != StateError || is == nil {
		t.Errorf("takeover paused, core failed: %v %+v", st, is)
	}
	if st, is := judge(settings.Settings{Mode: "rule"}, running, Snapshot{Nodes: []NodeView{{Name: "n"}}}); st != StateOK || is != nil {
		t.Errorf("nothing wrong: %v %+v", st, is)
	}
}

// A locked group with no node that works is refused, not merely down: the
// issue says the lock refuses it and how to get out, in every mode and
// before groups that are only down.
func TestJudgeSaysLockRefusesBrokenGroup(t *testing.T) {
	a := &App{}
	cs := core.Status{State: core.Running}
	dead := NodeView{Name: "n", Display: "n", Tested: true, Routes: map[string]NodeTest{}}
	plain := RouteView{ID: "streaming", Name: "流媒体", Broken: "fixed", Policy: routes.Policy{Mode: routes.ModeFixed, Fixed: "n"}}
	locked := RouteView{ID: "ai", Name: "AI 服务", Broken: "fixed", Locked: true, Policy: routes.Policy{Mode: routes.ModeFixed, Fixed: "n"}}
	snap := Snapshot{Nodes: []NodeView{dead}, Routes: []RouteView{plain, locked}}
	st, is := a.judge(true, settings.Settings{Mode: "rule"}, cs, Takeover{}, snap, nil)
	if st != StateDegraded || is == nil || !strings.Contains(is.Text, "AI 服务") || is.Detail != lockedOutSay() || is.Action != "fix:ai" {
		t.Fatalf("rule mode: %v %+v", st, is)
	}
	for _, way := range []string{"换一个能用的节点", "解除出口锁定", "不会改走直连"} {
		if !strings.Contains(is.Detail, way) {
			t.Errorf("the issue doesn't say %q: %s", way, is.Detail)
		}
	}
	// A locked group goes by its node in global mode too; the other doesn't.
	if _, is := a.judge(true, settings.Settings{Mode: "global"}, cs, Takeover{}, snap, nil); is == nil || is.Action != "fix:ai" || strings.Contains(is.Text, "另有") {
		t.Errorf("global mode: %+v", is)
	}
	// Every node down, in a group that picks among all of them.
	locked.Broken, locked.Policy = "all", routes.Policy{}
	snap.Routes = []RouteView{locked}
	if _, is := a.judge(true, settings.Settings{Mode: "rule"}, cs, Takeover{}, snap, nil); is == nil || !strings.Contains(is.Text, "节点都不通") || is.Detail != lockedOutSay() {
		t.Errorf("all down: %+v", is)
	}
	// No node at all: the rest goes direct, the locked group does not.
	snap = Snapshot{Routes: []RouteView{{ID: "ai", Name: "AI 服务", Locked: true}}}
	if _, is := a.judge(true, settings.Settings{Mode: "rule"}, cs, Takeover{}, snap, nil); is == nil || !strings.Contains(is.Detail, "AI 服务") || !strings.Contains(is.Detail, "拦下") {
		t.Errorf("no nodes: %+v", is)
	}
}

func TestNoneAlive(t *testing.T) {
	down := NodeView{Name: "a", Tested: true}
	up := NodeView{Name: "b", Tested: true, Delay: 80}
	untested := NodeView{Name: "c"}
	if noneAlive(nil, "ai") || noneAlive([]NodeView{down, up}, "ai") || noneAlive([]NodeView{down, untested}, "ai") {
		t.Error("a node that works, or was not tested, is a way out")
	}
	if !noneAlive([]NodeView{down, down}, "ai") {
		t.Error("every node failed its test")
	}
}
