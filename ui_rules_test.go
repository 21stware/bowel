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
	for _, want := range []string{"让 anthropic.com 及其子域名 走：", "默认", "AI 服务", "直连", "只让 api.anthropic.com 走…", "删除这条规则", "管理规则…"} {
		if !slices.Contains(m, want) {
			t.Errorf("menu lacks %q: %q", want, m)
		}
	}
	if err := tt.ChooseMenuItem("只让 api.anthropic.com 走…", "开发"); err != nil {
		t.Error(err)
	}
}

// TestRulesSection: 设置 > 规则 lists the user's rules by route, says what
// a typed rule will do before it is added, adds it on Enter, and tests an
// address.
func TestRulesSection(t *testing.T) {
	for _, dark := range []bool{false, true} {
		u := fixtureUI()
		u.snap.Settings = settings.Defaults.Clone()
		u.snap.Settings.Rules = []routes.UserRule{
			{Kind: routes.KindSuffix, Value: "openai.com", Route: "direct"},
			{Kind: routes.KindKeyword, Value: "tracker", Route: "default"},
			{Kind: routes.KindIP, Value: "10.0.0.0/8", Route: "direct"},
		}
		u.set.section = "rules"
		tt := ui.NewTesterWith(u.settingsView, 760, 640, ui.TesterOptions{Dark: dark})
		tt.Advance(time.Second)
		for _, want := range []string{"你的规则 · 3", "openai.com", "tracker", "10.0.0.0/8", "内置规则", "测试一个地址"} {
			if !tt.HasText(want) {
				t.Errorf("rules section lacks %q: %q", want, tt.Texts())
			}
		}
		if dark {
			shot(t, tt, "settings-rules-list-dark")
			continue
		}
		f, ok := tt.AccessByRole(ui.RoleTextField, "新规则")
		if !ok {
			t.Fatal("no new-rule field")
		}
		tt.ClickAt(f.Bounds.X+f.Bounds.W/2, f.Bounds.Y+f.Bounds.H/2)
		tt.Frame()
		tt.Type("https://chat.example.com/x")
		tt.Frame()
		if !hasTextPart(tt, "chat.example.com 及其子域名 将走「默认」") {
			t.Errorf("no preview of the typed rule: %q", tt.Texts())
		}
		shot(t, tt, "settings-rules-typing")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		tt.Frame()
		got := u.shownSettings().Rules
		if len(got) != 4 || got[0] != (routes.UserRule{Kind: routes.KindSuffix, Value: "chat.example.com", Route: "default"}) {
			t.Errorf("rules after Enter: %+v", got)
		}

		// A bad one says why, and adds nothing.
		tt.Type("not a domain")
		tt.Frame()
		if !hasTextPart(tt, "不是有效的域名") {
			t.Errorf("no error for a bad rule: %q", tt.Texts())
		}
		tt.Key(ui.Cmd, ui.KeyA)
		tt.Key(0, ui.KeyBackspace)

		tf, ok := tt.AccessByRole(ui.RoleTextField, "测试地址")
		if !ok {
			t.Fatal("no test field")
		}
		tt.ClickAt(tf.Bounds.X+tf.Bounds.W/2, tf.Bounds.Y+tf.Bounds.H/2)
		tt.Frame()
		tt.Type("api.openai.com")
		tt.Frame()
		if !tt.HasText("走「直连」") || !tt.HasText("你的规则") {
			t.Errorf("test of api.openai.com: %q", tt.Texts())
		}
		shot(t, tt, "settings-rules-test")
		tt.Key(ui.Cmd, ui.KeyA)
		tt.Type("api.anthropic.com")
		tt.Frame()
		if !tt.HasText("走「AI 服务」") || !hasTextPart(tt, "DOMAIN-SUFFIX,anthropic.com") {
			t.Errorf("test of api.anthropic.com: %q", tt.Texts())
		}
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
	if err := tt.Click("管理规则"); err != nil {
		t.Fatal(err)
	}
	if u.set.section != "rules" || u.set.opened != 1 {
		t.Errorf("管理规则 opened %d windows at %q", u.set.opened, u.set.section)
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
	} {
		s := settings.Defaults.Clone()
		s.Rules = rules
		if s.Validate() == nil {
			t.Errorf("%s: accepted %+v", name, rules)
		}
	}
	s := settings.Defaults.Clone()
	s.Rules = []routes.UserRule{ok, {Kind: routes.KindIP, Value: "10.0.0.0/8", Route: "direct"}}
	if err := s.Validate(); err != nil {
		t.Error(err)
	}
}
