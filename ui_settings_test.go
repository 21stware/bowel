package main

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

// TestSettingsButton: the gear is the title bar's last control, and it
// and ⌘, open the settings window, leaving the main window's page alone.
func TestSettingsButton(t *testing.T) {
	u := fixtureUI()
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(time.Second)
	r, ok := tt.Find("设置")
	if !ok {
		t.Fatalf("no settings button: %q", tt.Texts())
	}
	pill, _ := tt.Find("正常")
	if r.Y > 50 || r.X < 900 || r.X <= pill.X {
		t.Errorf("settings button at %+v, state pill at %+v", r, pill)
	}
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if u.set.opened != 1 || u.page != "home" || tt.HasText("mihomo 内核") {
		t.Errorf("after the button: opened %d, page %q", u.set.opened, u.page)
	}
	tt.Key(ui.Cmd, ui.KeyComma)
	tt.Frame()
	if u.set.opened != 2 || u.page != "home" {
		t.Errorf("after ⌘,: opened %d, page %q", u.set.opened, u.page)
	}
}

func TestSettingsWindow(t *testing.T) {
	for _, dark := range []bool{false, true} {
		u := fixtureUI()
		u.snap.Settings = settings.Defaults.Clone()
		u.snap.ConfigPath = "/Users/me/Library/Application Support/bowel/core/config.yaml"
		tt := ui.NewTesterWith(u.settingsView, 760, 560, ui.TesterOptions{Dark: dark})
		tt.Advance(time.Second)
		if !tt.HasText("通用") || !tt.HasText("语言") || tt.HasText("混合端口") {
			t.Fatalf("settings window opens on %q", tt.Texts())
		}
		suffix := map[bool]string{false: "", true: "-dark"}[dark]
		// Every section from the sidebar; the sidebar on the left, the
		// section's options on the right.
		for _, sec := range settingsSections {
			if err := tt.Click(sec.name()); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if u.set.section != sec.id {
				t.Fatalf("clicking %s chose %q", sec.name(), u.set.section)
			}
			if r, _ := tt.Find(sec.name()); r.X > 190 {
				t.Errorf("sidebar item %s at %+v", sec.name(), r)
			}
			shot(t, tt, "settings-"+sec.id+suffix)
		}
		if dark {
			continue
		}

		// A choice applies at once.
		if err := tt.Click("内核"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if err := tt.Click("warning"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if got := u.shownSettings().LogLevel; got != "warning" {
			t.Errorf("log level %q after clicking warning", got)
		}

		// So does a switch.
		sw, ok := tt.AccessByRole(ui.RoleSwitch, "IPv6")
		if !ok || !sw.Checked {
			t.Fatalf("IPv6 switch: %+v %v", sw, ok)
		}
		tt.ClickAt(sw.Bounds.X+sw.Bounds.W/2, sw.Bounds.Y+sw.Bounds.H/2)
		tt.Frame()
		tt.Frame()
		if u.shownSettings().IPv6 {
			t.Error("IPv6 still on after clicking its switch")
		}

		// A text option is kept on Enter, not while typing.
		if err := tt.Click("入站"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		f, ok := tt.AccessByRole(ui.RoleTextField, "混合端口")
		if !ok {
			t.Fatal("no port field")
		}
		tt.ClickAt(f.Bounds.X+f.Bounds.W/2, f.Bounds.Y+f.Bounds.H/2)
		tt.Frame()
		tt.Key(ui.Cmd, ui.KeyA)
		tt.Type("7900")
		tt.Frame()
		if u.shownSettings().MixedPort != 7899 {
			t.Error("the port changed while typing")
		}
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		tt.Frame()
		if got := u.shownSettings().MixedPort; got != 7900 {
			t.Errorf("port %d after Enter", got)
		}

		// A number that is not one is refused, and nothing changes.
		f, _ = tt.AccessByRole(ui.RoleTextField, "HTTP 端口")
		tt.ClickAt(f.Bounds.X+f.Bounds.W/2, f.Bounds.Y+f.Bounds.H/2)
		tt.Frame()
		tt.Key(ui.Cmd, ui.KeyA)
		tt.Type("abc")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if got := u.shownSettings().HTTPPort; got != 0 {
			t.Errorf("HTTP port %d after typing abc", got)
		}
		tt.Key(ui.Cmd, ui.KeyA)
		tt.Type("7901")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if got := u.shownSettings().HTTPPort; got != 7901 {
			t.Errorf("HTTP port %d after typing 7901", got)
		}

		// A drop-down.
		if err := tt.Click("内核"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if _, ok := tt.AccessByRole(ui.RoleComboBox, "TLS 指纹"); !ok {
			if _, ok := tt.Access("TLS 指纹"); !ok {
				t.Error("no fingerprint drop-down")
			}
		}

		// Restoring the defaults keeps the ports.
		if err := tt.Click("关于"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		b, ok := tt.AccessByRole(ui.RoleButton, "恢复默认")
		if !ok {
			t.Fatal("no restore button")
		}
		tt.ClickAt(b.Bounds.X+b.Bounds.W/2, b.Bounds.Y+b.Bounds.H/2)
		tt.Frame()
		got := u.shownSettings()
		if got.HTTPPort != 7901 || got.MixedPort != 7900 || !got.IPv6 || got.LogLevel != "info" {
			t.Errorf("after restoring: http %d mixed %d ipv6 %v log %s", got.HTTPPort, got.MixedPort, got.IPv6, got.LogLevel)
		}
	}
}

// TestSettingsSections: every section says what it is about and has its
// options, in both languages.
func TestSettingsSections(t *testing.T) {
	defer i18n.Set("zh")
	want := map[string][]string{
		"general":    {"开机启动", "静默启动"},
		"inbound":    {"HTTP 端口", "SOCKS 端口", "允许的地址", "认证"},
		"core":       {"进程匹配", "TLS 指纹", "出站网卡", "保活间隔", "自动测速间隔", "切换容差", "健康检查间隔"},
		"dns":        {"Fake-IP 网段", "引导服务器", "备用服务器", "域名策略", "遵循规则", "Hosts", "清除 DNS 缓存"},
		"sniffer":    {"域名嗅探", "覆盖目标地址", "QUIC", "跳过"},
		"tun":        {"自动选择网卡", "端点独立 NAT", "MTU", "DNS 劫持"},
		"geo":        {"自动更新", "更新间隔", "立即更新", "GeoSite"},
		"controller": {"地址", "密钥"},
		"rules":      {"新规则", "测试一个地址", "内置规则", "AI 服务"},
	}
	for _, lang := range i18n.Languages {
		i18n.Set(lang)
		u := fixtureUI()
		u.snap.Settings = settings.Defaults.Clone()
		tt := ui.NewTesterWith(u.settingsView, 760, 560, ui.TesterOptions{})
		tt.Advance(time.Second)
		for _, sec := range settingsSections {
			if err := tt.Click(sec.name()); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if lang == "zh" {
				for _, w := range want[sec.id] {
					if !tt.HasText(w) {
						t.Errorf("%s lacks %q: %q", sec.id, w, tt.Texts())
					}
				}
			}
			for _, txt := range tt.Texts() {
				if lang == "en" && strings.ContainsFunc(txt, func(r rune) bool { return r >= 0x4e00 && r <= 0x9fff }) &&
					!slices.Contains(slices.Collect(maps.Values(i18n.Names)), txt) {
					t.Errorf("%s in English shows %q", sec.id, txt)
				}
			}
		}
	}
	// The dashboards' controller cannot be set while Moho's own is on TCP.
	i18n.Set("zh")
	u := fixtureUI()
	u.snap.Settings = settings.Defaults.Clone()
	u.snap.ControllerTCP = "127.0.0.1:53111"
	tt := ui.NewTesterWith(u.settingsView, 760, 560, ui.TesterOptions{})
	tt.Advance(time.Second)
	if err := tt.Click("外部控制"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if f, ok := tt.AccessByRole(ui.RoleTextField, "地址"); !ok || !f.Disabled {
		t.Errorf("controller address field: %+v %v", f, ok)
	}
}

// TestSettingsList: a list option takes an entry on Enter, as a chip, and
// its button takes it out.
func TestSettingsList(t *testing.T) {
	u := fixtureUI()
	u.snap.Settings = settings.Defaults.Clone()
	tt := ui.NewTesterWith(u.settingsView, 760, 1400, ui.TesterOptions{})
	tt.Advance(time.Second)
	if err := tt.Click("入站"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	lf, ok := tt.AccessByRole(ui.RoleTextField, "不走代理")
	if !ok {
		t.Fatalf("no bypass field")
	}
	tt.ClickAt(lf.Bounds.X+lf.Bounds.W/2, lf.Bounds.Y+lf.Bounds.H/2)
	tt.Frame()
	tt.Type("*.corp.example")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Frame()
	got := u.shownSettings().Bypass
	if !slices.Contains(got, "*.corp.example") || len(got) != len(settings.Defaults.Bypass)+1 {
		t.Fatalf("bypass after Enter: %q", got)
	}
	if err := tt.Click("Remove localhost"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Frame()
	tt.Frame()
	if got := u.shownSettings().Bypass; slices.Contains(got, "localhost") {
		t.Errorf("bypass after removing localhost: %q", got)
	}
	shot(t, tt, "settings-inbound-tall")
}

func TestResetCore(t *testing.T) {
	s := settings.Defaults.Clone()
	s.HTTPPort, s.Authentication, s.ExtController = 7901, []string{"a:b"}, "127.0.0.1:9090"
	s.Sniff, s.TunMTU, s.LogLevel, s.Language = true, 1400, "debug", "en"
	s.ResetCore()
	if s.HTTPPort != 7901 || len(s.Authentication) != 1 || s.ExtController == "" || s.Language != "en" {
		t.Errorf("reset lost what it keeps: %+v", s)
	}
	if s.Sniff || s.TunMTU != 9000 || s.LogLevel != "info" {
		t.Errorf("reset kept options: sniff %v mtu %d log %s", s.Sniff, s.TunMTU, s.LogLevel)
	}
}

func TestSettingsFile(t *testing.T) {
	dir := t.TempDir()
	// Saved before the new options existed: they come in at their defaults.
	_ = os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"mixedPort": 7911, "ipv6": false, "bypass": ["x.lan"]}`), 0o600)
	st, err := settings.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := st.Get()
	if s.MixedPort != 7911 || s.IPv6 || s.TunMTU != 9000 || s.URLTestInterval != 300 || !s.SniffParsePureIP || len(s.DNSHijack) != 2 {
		t.Errorf("opened: %+v", s)
	}
	if !slices.Equal(settings.Defaults.Bypass[:1], []string{"localhost"}) {
		t.Errorf("opening changed the defaults: %q", settings.Defaults.Bypass)
	}
	if err := s.Validate(); err != nil {
		t.Errorf("opened settings: %v", err)
	}
}

func TestSettingsValidate(t *testing.T) {
	if err := settings.Defaults.Validate(); err != nil {
		t.Fatalf("defaults: %v", err)
	}
	for name, fn := range map[string]func(*settings.Settings){
		"port":       func(s *settings.Settings) { s.MixedPort = 70000 },
		"log level":  func(s *settings.Settings) { s.LogLevel = "loud" },
		"dns mode":   func(s *settings.Settings) { s.DNSMode = "magic" },
		"stack":      func(s *settings.Settings) { s.TunStack = "lwip" },
		"test url":   func(s *settings.Settings) { s.TestURL = "gstatic.com" },
		"no dns":     func(s *settings.Settings) { s.Nameservers = nil },
		"bad bypass": func(s *settings.Settings) { s.Bypass = []string{"a b"} },
	} {
		s := settings.Defaults.Clone()
		fn(&s)
		if s.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if got := settings.SplitList(" a, b，c\nd ;e "); !slices.Equal(got, []string{"a", "b", "c", "d", "e"}) {
		t.Errorf("SplitList: %q", got)
	}
}

func TestBuildConfigOptions(t *testing.T) {
	in := core.Input{Routes: routes.Template(), Mode: "rule", MixedPort: 7899, ProbePort: 7900, TestURL: "x",
		Options: core.Options{IPv6: false, UnifiedDelay: true, DNS: true, DNSMode: "redir-host", Nameservers: []string{"1.1.1.1"}}}
	out, err := core.BuildConfig(in)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"ipv6: false", "unified-delay: true", "tcp-concurrent: false", "enhanced-mode: redir-host", "- 1.1.1.1"} {
		if !strings.Contains(s, want) {
			t.Errorf("config lacks %q", want)
		}
	}
	if strings.Contains(s, "fake-ip-range") || strings.Contains(s, "tun:") {
		t.Errorf("unexpected fake-ip or tun:\n%s", s)
	}
	in.Tun, in.TunStack, in.StrictRoute = true, "gvisor", true
	out, _ = core.BuildConfig(in)
	s = string(out)
	for _, want := range []string{"stack: gvisor", "strict-route: true", "listen: 127.0.0.1:1053"} {
		if !strings.Contains(s, want) {
			t.Errorf("tun config lacks %q", want)
		}
	}
}

func TestSettingsLanguage(t *testing.T) {
	defer i18n.Set("zh")
	u := fixtureUI()
	u.snap.Settings = settings.Defaults.Clone()
	tt := ui.NewTesterWith(u.settingsView, 760, 560, ui.TesterOptions{})
	tt.Advance(time.Second)
	if err := tt.Click("English"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if got := u.shownSettings().Language; got != "en" {
		t.Fatalf("language %q after clicking English", got)
	}
	if !tt.HasText("General") || !tt.HasText("Language") || tt.HasText("语言") {
		t.Fatalf("English settings show %q", tt.Texts())
	}
	for _, l := range []struct{ name, lang, word string }{
		{"日本語", "ja", "言語"}, {"繁體中文", "zh-Hant", "語言"}, {"简体中文", "zh", "语言"},
	} {
		if err := tt.Click(l.name); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
		if !tt.HasText(l.word) || i18n.Lang() != l.lang || u.shownSettings().Language != l.lang {
			t.Fatalf("in %s: %q", l.name, tt.Texts())
		}
	}
}
