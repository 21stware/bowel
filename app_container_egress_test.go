package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
	"gopkg.in/yaml.v3"
)

// TestCoreOptionsInContainer: in a container every extra port is open as
// the mixed port is, the settings untouched; elsewhere each is as set.
func TestCoreOptionsInContainer(t *testing.T) {
	o := core.DefaultOptions.Clone()
	o.ExtraPorts = []core.ExtraPort{{Port: 7901, Route: "ai"}, {Port: 7902, Route: "media", Lan: true}}
	t.Setenv(containerEnv, "")
	if got := coreOptions(o); !slices.Equal(got.ExtraPorts, o.ExtraPorts) {
		t.Errorf("outside a container: %+v", got.ExtraPorts)
	}
	t.Setenv(containerEnv, "1")
	got := coreOptions(o)
	if !got.ExtraPorts[0].Lan || !got.ExtraPorts[1].Lan {
		t.Errorf("in a container: %+v", got.ExtraPorts)
	}
	if o.ExtraPorts[0].Lan {
		t.Error("the settings' own list was changed")
	}
}

// TestContainerLockByPorts: in a container, where no system proxy carries
// anything, a lock is held by what comes in, takeover on or off: the core
// is not told to send everything but the locked groups direct, and no
// system proxy is tried for it. With TUN set, TUN carries the rest, and a
// lock it doesn't carry says so in a container's words.
func TestContainerLockByPorts(t *testing.T) {
	a := lockApp(t, "/tun")
	ports := lockedSettings(settings.Defaults.Clone())
	ports.SystemProxy, ports.Tun = false, false

	t.Setenv(containerEnv, "")
	if !a.guarding(ports) || !a.proxyWanted(ports) || portsHold(ports) {
		t.Fatal("outside a container, a lock with takeover off is the system proxy's")
	}
	t.Setenv(containerEnv, "1")
	if a.guarding(ports) || a.proxyWanted(ports) || !portsHold(ports) {
		t.Errorf("container: guarding %v proxyWanted %v portsHold %v", a.guarding(ports), a.proxyWanted(ports), portsHold(ports))
	}
	paused := ports.Clone()
	paused.Paused = true
	if a.guarding(paused) || !portsHold(paused) {
		t.Error("container, takeover off: the ports still hold the lock")
	}
	tun := ports.Clone()
	tun.Tun = true
	if portsHold(tun) {
		t.Error("container with TUN: held only while TUN carries traffic")
	}

	// The main port's rules stay takeover's: a group that isn't locked
	// goes by its node, and what no rule takes by the default route.
	in := core.Input{Routes: routes.Build(ports.Routing, nil), Mode: "rule", MixedPort: 7899, ProbePort: 7900, TestURL: "x",
		Guard: a.guarding(ports), Options: ports.Options}
	b, err := core.BuildConfig(in)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct{ Rules []string }
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	if last := cfg.Rules[len(cfg.Rules)-1]; last != "MATCH,"+routes.Group(routes.Template()[0]) {
		t.Errorf("container, locked, no TUN: the rest goes to %s", last)
	}

	unheld := lockEntry(true, false, false, false, "")
	if e := containerLock(unheld); e.Status != "error" || !strings.Contains(e.Detail, "TUN") || strings.Contains(e.Detail, "系统代理") {
		t.Errorf("container, TUN not carrying: %+v", e)
	}
	if containerLock(nil) != nil {
		t.Error("nothing locked: no entry")
	}
	t.Setenv(containerEnv, "")
	if e := containerLock(lockEntry(true, false, false, false, "")); !strings.Contains(e.Detail, "系统代理") {
		t.Errorf("outside a container the words are the desktop's: %+v", e)
	}
}

// apply is s with the environment's egress settings applied.
func apply(env map[string]string, s settings.Settings) (settings.Settings, []string) {
	var lines []string
	for _, c := range envEgress(func(k string) string { return env[k] }, s) {
		if c.change != nil {
			c.change(&s)
		}
		lines = append(lines, c.line)
	}
	return s, lines
}

func TestEnvExtraPorts(t *testing.T) {
	base := settings.Defaults.Clone()
	for v, want := range map[string][]core.ExtraPort{
		"7901=ai, 7902:media":                      {{Port: 7901, Route: "ai"}, {Port: 7902, Route: "media"}},
		"7901=ai\n7902=dev":                        {{Port: 7901, Route: "ai"}, {Port: 7902, Route: "dev"}},
		`[{"port":7901,"route":"Streaming Asia"}]`: {{Port: 7901, Route: "Streaming Asia"}},
		`[{"port":7901,"route":"ai","lan":true}]`:  {{Port: 7901, Route: "ai", Lan: true}},
		"none": {},
	} {
		s, lines := apply(map[string]string{"BOWEL_EXTRA_PORTS": v}, base.Clone())
		if !slices.Equal(s.ExtraPorts, want) || s.ExtraPorts == nil || len(lines) != 1 {
			t.Errorf("%q: %+v %q", v, s.ExtraPorts, lines)
		}
	}
	for _, v := range []string{"7901", "ai=7901", "70000=ai", "7901=", "[7901]"} {
		s, lines := apply(map[string]string{"BOWEL_EXTRA_PORTS": v}, base.Clone())
		if s.ExtraPorts != nil || len(lines) != 1 || !strings.Contains(lines[0], "ignored") {
			t.Errorf("%q: %+v %q", v, s.ExtraPorts, lines)
		}
	}
	_, lines := apply(map[string]string{"BOWEL_EXTRA_PORTS": "7901=ai 7902=nope"}, base.Clone())
	if len(lines) != 1 || !strings.Contains(lines[0], "7902=nope (no such route group") || strings.Contains(lines[0], "7901=ai (") {
		t.Errorf("a group that isn't there: %q", lines)
	}
}

// TestEnvLock: BOWEL_LOCK locks the groups it names and unlocks the rest,
// but never unlocks anything when one of its entries is wrong.
func TestEnvLock(t *testing.T) {
	base := settings.Defaults.Clone()
	base.Routing.SetLocked("media", true)
	locked := func(s settings.Settings) (out []string) {
		for _, id := range []string{"ai", "media", "dev"} {
			if s.Routing.Locked(id) {
				out = append(out, id)
			}
		}
		return out
	}
	for v, want := range map[string][]string{
		"ai dev":    {"ai", "dev"},
		"AI,ai":     {"ai"},
		"ai, nope":  {"ai", "media"}, // a typo: media stays locked
		"default":   {"media"},
		"direct ai": {"ai", "media"},
		"none":      nil,
	} {
		s, lines := apply(map[string]string{"BOWEL_LOCK": v}, base.Clone())
		if got := locked(s); !slices.Equal(got, want) || len(lines) != 1 {
			t.Errorf("%q: locked %v, want %v; %q", v, got, want, lines)
		}
	}
	if _, lines := apply(map[string]string{"BOWEL_LOCK": "ai, nope"}, base.Clone()); !strings.Contains(lines[0], "nope") || !strings.Contains(lines[0], "none is unlocked") {
		t.Errorf("the wrong entry isn't told: %q", lines)
	}
	im := base.Clone()
	im.ImportedRouting = "p1"
	if s, lines := apply(map[string]string{"BOWEL_LOCK": "ai"}, im); s.Routing.Locked("ai") || !strings.Contains(lines[0], "ignored") {
		t.Errorf("under a provider's routing: %v %q", s.Routing.Locked("ai"), lines)
	}
}

func TestEnvTun(t *testing.T) {
	paused := settings.Defaults.Clone()
	paused.Paused, paused.SystemProxy = true, true
	s, _ := apply(map[string]string{"BOWEL_TUN": "on"}, paused)
	if !s.Tun || s.Paused || s.SystemProxy {
		t.Errorf("on: tun %v paused %v systemProxy %v", s.Tun, s.Paused, s.SystemProxy)
	}
	if s, _ = apply(map[string]string{"BOWEL_TUN": "false"}, s); s.Tun {
		t.Error("off")
	}
	if s, lines := apply(map[string]string{"BOWEL_TUN": "maybe"}, settings.Defaults.Clone()); s.Tun || !strings.Contains(lines[0], "ignored") {
		t.Errorf("not a boolean: %q", lines)
	}
	if _, lines := apply(map[string]string{}, settings.Defaults.Clone()); len(lines) != 0 {
		t.Errorf("no environment: %q", lines)
	}
}

// TestEnvSettingsJSON: BOWEL_SETTINGS sets what `bowel settings set`
// does, with its checks; not what has a variable or command of its own,
// nor what agents may do. The specific variables win over it.
func TestEnvSettingsJSON(t *testing.T) {
	dir := t.TempDir()
	st, err := settings.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{settings: st}
	env := map[string]string{"BOWEL_SETTINGS": `{"sniff": true, "authentication": ["u:secret"], "extraPorts": [{"port": 7905, "route": "dev"}]}`,
		"BOWEL_EXTRA_PORTS": "7901=ai"}
	a.applyEnvEgress(func(k string) string { return env[k] })
	s := st.Get()
	if !s.Sniff || !slices.Equal(s.Authentication, []string{"u:secret"}) || !slices.Equal(s.ExtraPorts, []core.ExtraPort{{Port: 7901, Route: "ai"}}) {
		t.Errorf("sniff %v authentication %v ports %+v", s.Sniff, s.Authentication, s.ExtraPorts)
	}
	if c := a.envSettingsJSON(env["BOWEL_SETTINGS"]); strings.Contains(c.line, "secret") || !strings.Contains(c.line, "authentication") {
		t.Errorf("the line shows a value: %q", c.line)
	}
	for _, v := range []string{`{"mcpPolicy": "allow"}`, `{"tun": true}`, `{"sniff": "yes"}`, `{"nope": 1}`, `sniff=true`, `[]`} {
		if c := a.envSettingsJSON(v); c.change != nil || !strings.Contains(c.line, "ignored") {
			t.Errorf("%s: %q", v, c.line)
		}
	}
	if st.Get().MCPPolicy == "allow" {
		t.Error("the environment set what agents may do")
	}
}

// TestApplyEnvEgress: each setting is applied and saved on its own, so a
// port the settings refuse leaves the lock and TUN in place.
func TestApplyEnvEgress(t *testing.T) {
	dir := t.TempDir()
	st, err := settings.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{settings: st}
	mixed := st.Get().MixedPort
	env := map[string]string{"BOWEL_EXTRA_PORTS": "7901=ai " + itoa(mixed) + "=media", "BOWEL_LOCK": "ai", "BOWEL_TUN": "1"}
	a.applyEnvEgress(func(k string) string { return env[k] })
	again, err := settings.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := again.Get()
	if len(s.ExtraPorts) != 0 || !s.Routing.Locked("ai") || !s.Tun {
		t.Errorf("ports %+v, ai locked %v, tun %v", s.ExtraPorts, s.Routing.Locked("ai"), s.Tun)
	}
	env["BOWEL_EXTRA_PORTS"] = "7901=ai"
	a.applyEnvEgress(func(k string) string { return env[k] })
	if got := st.Get().ExtraPorts; !slices.Equal(got, []core.ExtraPort{{Port: 7901, Route: "ai"}}) {
		t.Errorf("ports %+v", got)
	}
}
