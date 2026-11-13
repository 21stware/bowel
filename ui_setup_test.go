package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/settings"
)

// setupFixture is a window opened with nothing: no sources, no nodes.
func setupFixture() *UI {
	u := fixtureUI()
	u.snap.Providers, u.snap.Nodes = nil, nil
	u.snap.Takeover.On = false
	u.setup.on = true
	return u
}

func TestSetupFirstStep(t *testing.T) {
	for _, dark := range []bool{false, true} {
		u := setupFixture()
		u.prov.focus = true
		tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{Dark: dark})
		tt.Advance(2 * time.Second)
		suffix := map[bool]string{false: "", true: "-dark"}[dark]
		shot(t, tt, "setup-add"+suffix)
		for _, s := range []string{"欢迎使用 bowel", "连接两端世界", "扫码", "选择文件…", "添加", "稍后再添加", "设置"} {
			if !tt.HasText(s) {
				t.Errorf("first step lacks %q: %q", s, tt.Texts())
			}
		}
		// The tabs and the state wait until there is something to show.
		for _, s := range []string{"代理", "日志", "未接管"} {
			if tt.HasText(s) {
				t.Errorf("first step shows %q", s)
			}
		}
		if !tt.Focused("添加提供商") {
			t.Errorf("the input does not have the focus")
		}
	}
}

func TestSetupTellsWhatItWillAdd(t *testing.T) {
	u := setupFixture()
	u.prov.detect = func(string) (string, error) { return "subscription", nil }
	u.prov.focus = true
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(2 * time.Second)
	tt.Type("https://sub.example.com/x")
	tt.Advance(time.Second)
	shot(t, tt, "setup-typed")
	if !tt.HasText("将添加为") || !tt.HasText("订阅") {
		t.Errorf("no detection under the card: %q", tt.Texts())
	}
	// A failure stays under the card until the text changes.
	u.setup.err, u.setup.errFor = "订阅下载失败：403 Forbidden", u.prov.input
	tt.Frame()
	tt.Advance(time.Second)
	shot(t, tt, "setup-error")
	if !tt.HasText("订阅下载失败：403 Forbidden") {
		t.Errorf("no failure under the card: %q", tt.Texts())
	}
	tt.Type("y")
	tt.Advance(time.Second)
	if tt.HasText("订阅下载失败：403 Forbidden") {
		t.Errorf("the failure outlived its text")
	}
}

func TestSetupAddingFlows(t *testing.T) {
	u := setupFixture()
	u.prov.input = "https://sub.example.com/x"
	u.snap.Pending = []string{"add"}
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(1500 * time.Millisecond)
	tt.Advance(300 * time.Millisecond)
	shot(t, tt, "setup-adding")
	if !tt.HasText("添加中…") {
		t.Errorf("adding lacks its word: %q", tt.Texts())
	}
}

// A source added moves on to taking over; taking over ends the first run.
func TestSetupSecondStep(t *testing.T) {
	for _, dark := range []bool{false, true} {
		u := setupFixture()
		full := fixture()
		tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{Dark: dark})
		tt.Advance(2 * time.Second)
		u.snap.Providers, u.snap.Nodes = full.Providers[:1], full.Nodes
		tt.Frame()
		tt.Advance(2 * time.Second)
		suffix := map[bool]string{false: "", true: "-dark"}[dark]
		shot(t, tt, "setup-ready"+suffix)
		for _, s := range []string{"个节点已就绪", "来自 机场 A", "系统代理", "TUN", "暂不接管，先进入主界面"} {
			if !tt.HasText(s) {
				t.Errorf("second step lacks %q: %q", s, tt.Texts())
			}
		}
		u.snap.Takeover.On = true
		tt.Frame()
		tt.Advance(2 * time.Second)
		if u.setup.on || tt.HasText("暂不接管，先进入主界面") {
			t.Errorf("taking over did not end the first run")
		}
		if !tt.HasText("日志") {
			t.Errorf("the tabs did not come back: %q", tt.Texts())
		}
	}
}

// A complete config added in the first run is asked about there: its routing
// would otherwise go unmentioned until someone read its row.
func TestSetupAsksAboutAConfigsRouting(t *testing.T) {
	u := setupFixture()
	full := fixture()
	tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
	tt.Advance(2 * time.Second)
	u.snap.Providers, u.snap.Nodes = full.Providers[:1], full.Nodes
	tt.Frame()
	tt.Advance(2 * time.Second)
	if tt.HasText("按它的路由分流") || tt.HasText("使用它们") {
		t.Errorf("a list of nodes is asked about: %q", tt.Texts())
	}
	u.snap.Providers[0].Routing = ProviderRouting{Groups: 2, Rules: 5}
	u.provAdded(u.snap.Providers[0])
	tt.Frame()
	tt.Advance(2 * time.Second)
	shot(t, tt, "setup-ready-routing")
	for _, s := range []string{"要按它的路由分流，还是只用它的节点？", "按它的路由分流", "只用节点", "系统代理", "TUN"} {
		if !tt.HasText(s) {
			t.Errorf("second step lacks %q: %q", s, tt.Texts())
		}
	}
	u.snap.Providers[0].Routing.InUse = true
	tt.Frame()
	tt.Advance(time.Second)
	shot(t, tt, "setup-ready-routing-used")
	if !tt.HasText("正在使用它的 2 个路由组") || tt.HasText("只用节点") || !u.setup.on {
		t.Errorf("in use: %q", tt.Texts())
	}
}

func TestSetupSkip(t *testing.T) {
	u := setupFixture()
	u.page = "providers"
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(2 * time.Second)
	if err := tt.Click("稍后再添加"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(2 * time.Second)
	if u.setup.on || u.page != "home" || !tt.HasText("日志") {
		t.Errorf("skip did not open the board: on=%v page=%q", u.setup.on, u.page)
	}
}

// The tab shortcuts have no tabs to go to.
func TestSetupKeepsTheWindow(t *testing.T) {
	u := setupFixture()
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(time.Second)
	tt.Key(ui.Cmd, ui.Key3)
	tt.Advance(time.Second)
	if u.page != "home" || !u.setup.on {
		t.Errorf("⌘3 moved behind the first run: page=%q on=%v", u.page, u.setup.on)
	}
}

// Every skin, every language and the smallest window.
func TestSetupLooks(t *testing.T) {
	defer i18n.Set("zh")
	for _, k := range skins {
		for _, lang := range i18n.Languages {
			i18n.Set(lang)
			u := setupFixture()
			u.snap.Settings = settings.Defaults.Clone()
			skinPick(&u.snap.Settings, k.id)
			tt := ui.NewTesterWith(u.view, 960, 660, ui.TesterOptions{Dark: k.fixed() == "dark"})
			tt.Advance(2 * time.Second)
			shot(t, tt, "setup-"+k.id+"-"+lang)
		}
	}
}
