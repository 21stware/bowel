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

// gatewayFixture is a UI with the gateway on and two devices using it,
// over Wi-Fi with a leased address, so that every tip shows.
func gatewayFixture() *UI {
	u := fixtureUI()
	u.snap.Settings = settings.Defaults.Clone()
	u.snap.Settings.Gateway, u.snap.Settings.Tun, u.snap.Settings.Paused = true, true, false
	u.snap.Takeover.Tun = Entry{Wanted: true, Status: "on"}
	u.snap.Takeover.Gateway = GatewayView{
		Wanted: true, Status: "on", Ready: true,
		Network:    gateway.Network{IP: "192.168.31.28", Iface: "en0", Router: "192.168.31.1", WiFi: true, DHCP: true},
		Forwarding: true, DNS: true, Tun: true, Awake: true, Devices: 2,
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
		if !tt.HasText("网关") || !tt.HasText("生效中 · 2 台设备在用") {
			t.Fatalf("board: %q", tt.Texts())
		}
		shot(t, tt, "gateway-home"+suffix)
		u.home.x = "takeover"
		tt.Frame()
		tt.Advance(2 * time.Second)
		shot(t, tt, "gateway-x-takeover"+suffix)
		if !tt.HasText("192.168.31.28") || !tt.HasText("在其他设备上填这两项：") {
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
	// i18n.T, for the tip names the computer as this system's interface does.
	for _, s := range []string{"网关模式", "192.168.31.28", "DNS 53 端口", "改用网线", "绑定固定地址", "关掉 IPv6", i18n.T("阻止 Mac 自动休眠", "keeps the Mac from sleeping")} {
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
