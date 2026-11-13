package main

import (
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/devnet"
	"github.com/oboo/moho/internal/settings"
)

func TestDevnetSection(t *testing.T) {
	u := fixtureUI()
	u.snap.Settings = settings.Defaults.Clone()
	u.snap.Dev = DevNet{
		Env: devnet.Env{
			Tailscale: devnet.Tailscale{Installed: true, State: "Running", Iface: "utun4", Suffix: "tail17b34.ts.net",
				Peers: []devnet.Peer{{Name: "mybox", IPs: []netip.Addr{netip.MustParseAddr("100.100.1.2")}}}},
			VPNs:      []devnet.Tunnel{{Iface: "utun5", Kind: "vpn", Prefixes: []netip.Prefix{netip.MustParsePrefix("10.8.0.0/16")}}},
			Resolvers: []devnet.Resolver{{Domain: "corp.example", Servers: []string{"10.8.0.53"}}},
		},
		Warnings: []string{"utun13 上有别的代理接管了默认路由，流量会先经过它，这里的设置管不到"},
		Exports:  exportsFor(u.snap.Settings, devnet.Compile(devnet.Options{}, devnet.Env{})),
	}
	tt := ui.NewTesterWith(u.settingsView, 760, 1100, ui.TesterOptions{})
	tt.Advance(time.Second)
	if err := tt.Click("开发者网络"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, want := range []string{"Tailscale", "已连接 · utun4 · tail17b34.ts.net · 1 台设备", "corp.example", "局域网域名", "我的内网", "终端代理"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	if !strings.Contains(u.snap.Dev.Exports, "no_proxy=") || !strings.Contains(u.snap.Dev.Exports, ".ts.net") {
		t.Errorf("exports %s", u.snap.Dev.Exports)
	}
	shot(t, tt, "settings-devnet-full")
}

func TestTailscaleWord(t *testing.T) {
	for want, ts := range map[string]devnet.Tailscale{
		"未发现":  {},
		"未连接":  {Installed: true, State: "Stopped"},
		"需要登录": {Installed: true, State: "NeedsLogin"},
	} {
		if got := tailscaleWord(ts); got != want {
			t.Errorf("%+v: %s, want %s", ts, got, want)
		}
	}
}

// TestMeshRow: 直连 says what happens to Tailscale's machines and opens
// 设置 > 开发者网络; without Tailscale it says nothing.
func TestMeshRow(t *testing.T) {
	u := routesFixture()
	open := func() *ui.Tester {
		tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
		tt.Advance(time.Second)
		if err := tt.Click("直连"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
		return tt
	}
	if tt := open(); hasTextPart(tt, "Tailscale") {
		t.Errorf("a line about Tailscale without it: %q", tt.Texts())
	}
	u.snap.Dev.Env.Tailscale = devnet.Tailscale{Installed: true, State: "Running", Iface: "utun4",
		Peers: []devnet.Peer{{Name: "mybox"}, {Name: "nas"}}}
	tt := open()
	if !tt.HasText("已连接 · 2 台设备") {
		t.Fatalf("no state: %q", tt.Texts())
	}
	shot(t, tt, "routes-direct-tailscale")
	if err := tt.Click("Tailscale"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if u.set.opened != 1 || u.set.section != "devnet" {
		t.Errorf("opened %d, section %q", u.set.opened, u.set.section)
	}

	for want, c := range map[string]struct {
		s  func(*settings.Settings)
		ts devnet.Tailscale
	}{
		"没有自动直连": {func(s *settings.Settings) { s.DevOffScenarios = []string{devnet.Mesh} }, devnet.Tailscale{Installed: true, Iface: "utun4"}},
		"未连接":    {func(*settings.Settings) {}, devnet.Tailscale{Installed: true, State: "Stopped"}},
		"已连接":    {func(*settings.Settings) {}, devnet.Tailscale{Installed: true, Iface: "utun4"}},
	} {
		s := settings.Defaults.Clone()
		c.s(&s)
		if got := meshWord(s, c.ts); got != want {
			t.Errorf("%+v: %s, want %s", c.ts, got, want)
		}
	}
}
