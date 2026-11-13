package main

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/gateway"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/settings"
)

// gatewayFixture is a UI with the gateway on (manual) and two devices
// using it, over Wi-Fi with a leased address, so that every tip shows, and
// with the router still handing out addresses.
func gatewayFixture() *UI {
	u := fixtureUI()
	u.snap.Settings = settings.Defaults.Clone()
	u.snap.Settings.Gateway, u.snap.Settings.Tun, u.snap.Settings.Paused = true, true, false
	u.snap.Takeover.Tun = Entry{Wanted: true, Status: "on"}
	u.snap.Takeover.Gateway = GatewayView{
		Wanted: true, Status: "on", Ready: true,
		Network:    gateway.Network{IP: "192.168.31.28", Iface: "en0", Router: "192.168.31.1", WiFi: true, DHCP: true},
		Forwarding: true, DNS: true, Tun: true, Awake: true, Devices: 2,
		Auto: AutoView{Supported: true, Status: "off", Servers: []string{"192.168.31.1"}, Checked: time.Now()},
	}
	return u
}

// TestHomeGateway: the board lists the gateway under the entries, and the
// takeover's detail has its guide with the address to copy.
func TestHomeGateway(t *testing.T) {
	for _, dark := range []bool{false, true} {
		u := gatewayFixture()
		suffix := map[bool]string{false: "", true: "-dark"}[dark]
		tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{Dark: dark})
		tt.Advance(time.Second)
		if !tt.HasText("网关") || !tt.HasText("手动") {
			t.Fatalf("board: %q", tt.Texts())
		}
		shot(t, tt, "gateway-home"+suffix)
		u.home.x = "takeover"
		tt.Frame()
		tt.Advance(2 * time.Second)
		shot(t, tt, "gateway-x-takeover"+suffix)
		if !tt.HasText("192.168.31.28") || !tt.HasText("内网代理") || !tt.HasText("DHCP 服务器 192.168.31.1") || !tt.HasText("重新检测") {
			t.Fatalf("takeover detail: %q", tt.Texts())
		}
		if dark {
			continue
		}
		if err := tt.Click("复制"); err != nil {
			t.Fatal(err)
		}
		if got := tt.Clipboard(); got != "192.168.31.28" {
			t.Fatalf("copied %q", got)
		}
	}
}

// TestSettingsGateway: 入站 › 局域网 has the switch; on, the guide and the
// tips that apply; off, neither.
func TestSettingsGateway(t *testing.T) {
	u := gatewayFixture()
	u.set.section = "inbound"
	tt := ui.NewTesterWith(u.settingsView, 760, 1500, ui.TesterOptions{})
	tt.Advance(time.Second)
	shot(t, tt, "gateway-settings")
	for _, s := range []string{"网关模式", "192.168.31.28", "内网代理", "DHCP 服务器 192.168.31.1", "重新检测", "改用网线", "绑定固定地址"} {
		if !tt.HasText(s) {
			t.Errorf("no %q: %q", s, tt.Texts())
		}
	}
	if tt.HasText("防火墙") {
		t.Error("the firewall tip shows with the firewall off")
	}
	// Off: the switch and what it does, nothing else.
	u.setGateway(false, func(string) {})
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("网关模式") || tt.HasText("192.168.31.28") {
		t.Errorf("off: %q", tt.Texts())
	}
	shot(t, tt, "gateway-settings-off")
}

// The guide in the other languages: nothing left in Chinese, and the
// longer lines still wrap inside the card.
func TestSettingsGatewayLanguages(t *testing.T) {
	defer i18n.Set("zh")
	for _, lang := range []string{"en", "ja", "zh-Hant"} {
		i18n.Set(lang)
		u := gatewayFixture()
		u.set.section = "inbound"
		tt := ui.NewTesterWith(u.settingsView, 760, 1500, ui.TesterOptions{})
		tt.Advance(time.Second)
		shot(t, tt, "gateway-settings-"+lang)
		if lang != "en" {
			continue
		}
		for _, txt := range tt.Texts() {
			if strings.ContainsFunc(txt, func(r rune) bool { return r >= 0x4e00 && r <= 0x9fff }) &&
				!slices.Contains(slices.Collect(maps.Values(i18n.Names)), txt) {
				t.Errorf("English shows %q", txt)
			}
		}
	}
}

// The step to automatic, as the line under the addresses says it: nothing
// in the way, on, a second server, and the question before turning off.
func TestGatewayAutoLine(t *testing.T) {
	u := gatewayFixture()
	u.home.x = "takeover"
	g := &u.snap.Takeover.Gateway
	tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
	show := func(name string, want ...string) {
		t.Helper()
		tt.Frame()
		tt.Advance(2 * time.Second)
		shot(t, tt, "gateway-auto-"+name)
		for _, w := range want {
			if !tt.HasText(w) {
				t.Errorf("%s: no %q in %q", name, w, tt.Texts())
			}
		}
	}
	g.Auto.Servers = nil
	show("free", "可以设为内网代理网关", "确认开启")

	g.Auto.Wanted, g.Auto.Status, g.Auto.Pool, g.Auto.Leases = true, "on", "192.168.31.100 – 192.168.31.249", 4
	show("on", "生效中 · 自动", "已开启", "查看设备")
	if err := tt.Click("关闭"); err != nil {
		t.Fatal(err)
	}
	show("ask", "关闭后，从这里领到地址的设备会断网", "仍然关闭")
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	show("on-again", "已开启")

	g.Auto.Status, g.Auto.Detail = "error", "192.168.31.1 也在分配地址，设备会在两边之间来回"
	show("conflict", "192.168.31.1 也在分配地址，设备会在两边之间来回", "重新检测")

	// The board says it in a word.
	g.Auto.Status, g.Auto.Detail = "on", ""
	u.home.x = ""
	show("board", "自动")
	if err := tt.Click("查看设备"); err == nil {
		t.Error("the devices link shows on the board")
	}
}

// 日志 › 设备: who uses the gateway, filtered, with each one's record.
func TestActivityDevices(t *testing.T) {
	u := gatewayFixture()
	now := time.Now()
	u.snap.Devices = []DeviceView{
		{IP: "192.168.31.101", MAC: "a4:83:e7:01:02:03", Name: "Living-Room-TV", Via: "auto", Online: true, Conns: 3, Down: 6_100_000_000, Up: 41_000_000, Seen: now,
			Events: []DeviceEvent{{At: now.Add(-2 * time.Hour), Kind: "on"}, {At: now.Add(-20 * time.Hour), Kind: "off"}, {At: now.Add(-26 * time.Hour), Kind: "on", Note: "first"}}},
		{IP: "192.168.31.40", Via: "manual", Online: true, Conns: 7, Down: 1_300_000_000, Up: 90_000_000, Seen: now, Events: []DeviceEvent{{At: now.Add(-12 * time.Hour), Kind: "on", Note: "first"}}},
		{IP: "192.168.31.102", Name: "MiTV", Via: "auto", Seen: now.Add(-2 * time.Hour), Events: []DeviceEvent{{At: now.Add(-2 * time.Hour), Kind: "off"}}},
	}
	u.page = "activity"
	u.act.showDevices()
	tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
	tt.Advance(time.Second)
	for _, w := range []string{"2 台设备在线", "设备", "Living-Room-TV", "192.168.31.101", "未命名设备", "MiTV", "3 个连接", "自动", "手动"} {
		if !tt.HasText(w) {
			t.Errorf("no %q in %q", w, tt.Texts())
		}
	}
	if err := tt.Click("Living-Room-TV"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	shot(t, tt, "activity-devices")
	if !tt.HasText("第一次经过这里") || !tt.HasText("a4:83:e7:01:02:03") {
		t.Errorf("no record of the TV: %q", tt.Texts())
	}
	if err := tt.Click("离线"); err != nil { // the filter
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if tt.HasText("Living-Room-TV") || !tt.HasText("MiTV") {
		t.Errorf("offline only: %q", tt.Texts())
	}
	// With the gateway off there is nothing to list, and it says where to turn it on.
	u.snap.Devices, u.snap.Takeover.Gateway.Wanted = nil, false
	u.act.devSt = 0
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("网关没有打开。在「代理」页的「接管」里打开后，经过这台电脑的设备会列在这里。") {
		t.Errorf("gateway off: %q", tt.Texts())
	}
}
