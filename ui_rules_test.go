package main

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

// TestActivityRouteMenu: 让它走… offers the site first, the one name in a
// submenu, and, for a connection a rule of the user's matched, taking it
// out.
func TestActivityRouteMenu(t *testing.T) {
	u := actUI()
	u.snap.Settings.Rules = []routes.UserRule{{Kind: routes.KindSuffix, Value: "anthropic.com", Route: "ai"}}
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(time.Second)
	if err := tt.Click("api.anthropic.com:443"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("你的规则") {
		t.Errorf("a connection the user's rule matched isn't marked: %q", tt.Texts())
	}
	if err := tt.Click("让它走…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	m := tt.Menu()
	shot(t, tt, "activity-route-menu")
	for _, want := range []string{"让 anthropic.com 及其子域名 走：", "其他流量", "AI 服务", "直连", "只让 api.anthropic.com 走…", "删除这条规则", "管理规则…"} {
		if !slices.Contains(m, want) {
			t.Errorf("menu lacks %q: %q", want, m)
		}
	}
	if err := tt.ChooseMenuItem("只让 api.anthropic.com 走…", "开发"); err != nil {
		t.Error(err)
	}
}

// TestRoutesShowMine: the route list counts the user's rules in, and
// offers to manage them.
func TestRoutesShowMine(t *testing.T) {
	u := fixtureUI()
	for i := range u.snap.Routes {
		if u.snap.Routes[i].ID == "ai" {
			u.snap.Routes[i].Mine = 2
			u.snap.Routes[i].Rules += 2
		}
	}
	u.home.x = "routes"
	tt := ui.NewTester(u.view, 1180, 760)
	tt.Advance(2 * time.Second)
	if !hasTextPart(tt, "含你加的 2 条") {
		t.Errorf("no count of the user's rules: %q", tt.Texts())
	}
	shot(t, tt, "home-routes-mine")
	if err := tt.Click("管理路由组"); err != nil {
		t.Fatal(err)
	}
	if u.page != "routes" {
		t.Errorf("管理路由组 went to %q", u.page)
	}
}

// TestSettingsValidateRules: a saved rule must parse back to itself, point
// at a route that exists, and be there once.
func TestSettingsValidateRules(t *testing.T) {
	ok := routes.UserRule{Kind: routes.KindSuffix, Value: "openai.com", Route: "ai"}
	for name, rules := range map[string][]routes.UserRule{
		"route":     {{Kind: routes.KindSuffix, Value: "openai.com", Route: "nowhere"}},
		"twice":     {ok, {Kind: routes.KindSuffix, Value: "openai.com", Route: "dev"}},
		"malformed": {{Kind: routes.KindSuffix, Value: "OpenAI.com", Route: "ai"}},
		"kind":      {{Kind: "regex", Value: "x", Route: "ai"}},
		"app":       {{Kind: routes.KindApp, Value: "a,b", Route: "ai"}},
		"app twice": {{Kind: routes.KindApp, Value: "Telegram.exe", Route: "ai"}, {Kind: routes.KindApp, Value: "telegram.exe", Route: "dev"}},
	} {
		s := settings.Defaults.Clone()
		s.Rules = rules
		if s.Validate() == nil {
			t.Errorf("%s: accepted %+v", name, rules)
		}
	}
	s := settings.Defaults.Clone()
	s.Rules = []routes.UserRule{ok, {Kind: routes.KindIP, Value: "10.0.0.0/8", Route: "direct"}, {Kind: routes.KindApp, Value: "Telegram.exe", Route: "ai"}}
	if err := s.Validate(); err != nil {
		t.Error(err)
	}
}

// withAppRules sets whether rules for applications work, for the test.
func withAppRules(t *testing.T, on bool) {
	old := routes.AppRules
	routes.AppRules = on
	t.Cleanup(func() { routes.AppRules = old })
}

// TestActivityAppMenu: where the core can tell applications apart, 让它走…
// also offers to send everything of the connection's application; where it
// can't, it doesn't.
func TestActivityAppMenu(t *testing.T) {
	app := routes.AppOf("/Applications/Claude.app/Contents/MacOS/Claude")
	item := "让 " + app + " 的所有连接走…"
	open := func(t *testing.T) *ui.Tester {
		u := actUI()
		tt := ui.NewTester(u.view, 1060, 760)
		tt.Advance(time.Second)
		if err := tt.Click("api.anthropic.com:443"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
		if err := tt.Click("让它走…"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(300 * time.Millisecond)
		return tt
	}
	withAppRules(t, false)
	if m := open(t).Menu(); slices.Contains(m, item) {
		t.Errorf("the menu offers a rule for the app where it can't work: %q", m)
	}
	withAppRules(t, true)
	tt := open(t)
	if m := tt.Menu(); !slices.Contains(m, item) {
		t.Fatalf("menu lacks %q: %q", item, m)
	}
	shot(t, tt, "activity-route-menu-app")
	if err := tt.ChooseMenuItem(item, "开发"); err != nil {
		t.Error(err)
	}
}

// TestRoutesAddAppRule: 应用 is a kind of its own in the editor, where it
// can work; what is typed is a program's name, not a domain.
func TestRoutesAddAppRule(t *testing.T) {
	withAppRules(t, false)
	if slices.Contains(routeKinds(), routes.KindApp) {
		t.Error("the editor offers 应用 where it can't work")
	}
	withAppRules(t, true)
	if !slices.Contains(routeKindLabels(), "应用") {
		t.Fatalf("the editor's kinds: %q", routeKindLabels())
	}
	u := routesFixture()
	u.selectGroup("ai")
	u.routes.svc = routes.MineID("ai")
	u.routes.kind = routes.KindApp
	tt := ui.NewTester(u.view, 1060, 1400) // 我添加的 is the last of the group's services
	tt.Advance(time.Second)
	if !hasTextPart(tt, "只管这台电脑上的应用") {
		t.Errorf("no word on what an app rule covers: %q", tt.Texts())
	}
	typeInto(t, tt, "新规则", "Telegram.exe")
	if !hasTextPart(tt, "添加后：Telegram.exe · 应用") {
		t.Errorf("no preview: %q", tt.Texts())
	}
	shot(t, tt, "routes-app-rule")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Frame()
	got := u.shownSettings().Rules
	if want := (routes.UserRule{Kind: routes.KindApp, Value: "Telegram.exe", Route: "ai"}); len(got) != 1 || got[0] != want {
		t.Fatalf("rules after Enter: %+v", got)
	}
	tt.Advance(100 * time.Millisecond)
	if !tt.HasText("Telegram.exe") || !tt.HasText("应用") {
		t.Errorf("the rule isn't listed with its kind: %q", tt.Texts())
	}

	// With process matching off the core never learns who connects.
	u.setOpt(func(s *settings.Settings) { s.FindProcess = "off" })
	tt = ui.NewTester(u.view, 1060, 1400)
	tt.Advance(time.Second)
	if !hasTextPart(tt, "按应用的规则不会生效") {
		t.Errorf("no warning with process matching off: %q", tt.Texts())
	}
}

// TestAppRuleWorks: a new rule for an application is refused where it
// could never match; other rules are not its business.
func TestAppRuleWorks(t *testing.T) {
	app := routes.UserRule{Kind: routes.KindApp, Value: "Telegram.exe", Route: "ai"}
	withAppRules(t, false)
	if appRuleWorks(app) == nil {
		t.Error("accepted a rule for an app where the core can't name apps")
	}
	if err := appRuleWorks(routes.UserRule{Kind: routes.KindSuffix, Value: "openai.com", Route: "ai"}); err != nil {
		t.Error(err)
	}
	withAppRules(t, true)
	if err := appRuleWorks(app); err != nil {
		t.Error(err)
	}
}
