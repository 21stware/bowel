package main

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

func routesFixture() *UI {
	u := fixtureUI()
	u.snap.Settings = settings.Defaults.Clone()
	u.page = "routes"
	return u
}

func typeInto(t *testing.T, tt *ui.Tester, label, text string) {
	t.Helper()
	f, ok := tt.AccessByRole(ui.RoleTextField, label)
	if !ok {
		t.Fatalf("no field %q: %q", label, tt.Texts())
	}
	tt.ClickAt(f.Bounds.X+f.Bounds.W/2, f.Bounds.Y+f.Bounds.H/2)
	tt.Frame()
	tt.Type(text)
	tt.Frame()
}

// TestRoutesTab: the tab lists the groups, shows one's services, and opens
// a service's rules.
func TestRoutesTab(t *testing.T) {
	for _, dark := range []bool{false, true} {
		u := routesFixture()
		tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{Dark: dark})
		tt.Advance(time.Second)
		if err := tt.Click("AI 服务"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
		for _, want := range []string{"测试一个地址", "节点策略", "Claude", "OpenAI", "＋ 新建路由组", "＋ 新建服务", "删除路由组"} {
			if !hasTextPart(tt, want) {
				t.Errorf("tab lacks %q: %q", want, tt.Texts())
			}
		}
		shot(t, tt, map[bool]string{false: "routes-group", true: "routes-group-dark"}[dark])
		if err := tt.Click("Claude"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
		if !hasTextPart(tt, "claude.ai") {
			t.Errorf("open service: %q", tt.Texts())
		}
		shot(t, tt, map[bool]string{false: "routes-service", true: "routes-service-dark"}[dark])
	}
}

// TestRoutesRejectGroup: the group that refuses says so, takes services and
// rules, and has no node strategy and no way to delete it.
func TestRoutesRejectGroup(t *testing.T) {
	u := routesFixture()
	tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
	tt.Advance(time.Second)
	if err := tt.Click("拦截"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	for _, want := range []string{"已拦截", "命中这里规则的连接会被拒绝", "＋ 新建服务"} {
		if !hasTextPart(tt, want) {
			t.Errorf("group lacks %q: %q", want, tt.Texts())
		}
	}
	for _, not := range []string{"删除路由组", "今日"} {
		if hasTextPart(tt, not) {
			t.Errorf("group has %q: %q", not, tt.Texts())
		}
	}
	shot(t, tt, "routes-reject")
}

// TestRoutesAddRule: a typed rule says what it will do, joins the open
// service, and one that's in another service asks first.
func TestRoutesAddRule(t *testing.T) {
	u := routesFixture()
	u.selectGroup("ai")
	u.routes.svc = "claude"
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(time.Second)

	typeInto(t, tt, "新规则", "https://chat.example.com/x")
	if !hasTextPart(tt, "添加后") {
		t.Errorf("no preview: %q", tt.Texts())
	}
	shot(t, tt, "routes-typing")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Frame()
	got := u.shownSettings().Rules
	want := routes.UserRule{Kind: routes.KindSuffix, Value: "chat.example.com", Route: "ai", Service: "claude"}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("rules after Enter: %+v", got)
	}

	tt.Type("netflix.com")
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Advance(100 * time.Millisecond)
	if !hasTextPart(tt, "一条规则只能属于一个服务") {
		t.Fatalf("no conflict: %q", tt.Texts())
	}
	shot(t, tt, "routes-conflict")
	if err := tt.Click("移过来"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(100 * time.Millisecond)
	s := u.shownSettings()
	sv, ok := routes.FindRule(routes.Build(s.Routing, s.Rules), "DOMAIN-SUFFIX,netflix.com")
	if !ok || sv.ID != "claude" {
		t.Errorf("after moving: %+v %v", sv, ok)
	}
}

// TestRoutesNewAndDelete: a new group opens ready to be named, and deleting
// one asks, then hands its services over.
func TestRoutesNewAndDelete(t *testing.T) {
	u := routesFixture()
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(time.Second)
	if err := tt.Click("＋ 新建路由组"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(100 * time.Millisecond)
	if u.routes.group != "g1" {
		t.Fatalf("selected %q", u.routes.group)
	}
	shot(t, tt, "routes-new-group")
	if err := tt.Click("删除路由组"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(100 * time.Millisecond)
	if !tt.HasText("服务交给") {
		t.Errorf("delete doesn't ask: %q", tt.Texts())
	}
	if err := tt.Click("确定删除"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(100 * time.Millisecond)
	if s := u.shownSettings(); slices.ContainsFunc(s.Routing.Groups, func(g routes.GroupConf) bool { return g.ID == "g1" }) {
		t.Errorf("still there: %+v", s.Routing.Groups)
	}
	if u.routes.group != routes.Default {
		t.Errorf("left on %q", u.routes.group)
	}
}

// TestRoutesTester: an address shows group, service and rule.
func TestRoutesTester(t *testing.T) {
	u := routesFixture()
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(time.Second)
	typeInto(t, tt, "测试地址", "api.anthropic.com")
	for _, want := range []string{"Claude", "DOMAIN-SUFFIX,anthropic.com", "查看"} {
		if !hasTextPart(tt, want) {
			t.Errorf("lacks %q: %q", want, tt.Texts())
		}
	}
	shot(t, tt, "routes-tester")
}

// TestStrategyPicker: each strategy shows its own controls, in the card and
// in 代理's expansion.
func TestStrategyPicker(t *testing.T) {
	u := routesFixture()
	u.selectGroup("ai")
	u.routes.pol = true
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(time.Second)
	if !hasTextPart(tt, "地区") || !hasTextPart(tt, "来源") || !hasTextPart(tt, "定时测速") {
		t.Errorf("auto: %q", tt.Texts())
	}
	shot(t, tt, "routes-auto")
	if err := tt.Click("顺序备用"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(100 * time.Millisecond)
	if !hasTextPart(tt, "备用顺序 · 1") || !hasTextPart(tt, "点击加入") {
		t.Errorf("order: %q", tt.Texts())
	}
	if err := tt.Click(u.snap.Nodes[0].Display); err != nil {
		t.Fatal(err)
	}
	tt.Advance(100 * time.Millisecond)
	if q := u.policy("ai"); q.Mode != routes.ModeOrder || len(q.Order) != 2 {
		t.Errorf("after adding: %+v", q)
	}
	shot(t, tt, "routes-order")
	if err := tt.Click("固定节点"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(100 * time.Millisecond)
	shot(t, tt, "routes-fixed")
	if q := u.policy("ai"); q.Mode != routes.ModeFixed || len(q.Order) != 2 {
		t.Errorf("after fixed: %+v", q)
	}
}

// TestRoutesFixAction: a broken group's notice opens its strategy, in
// place on this tab and in 代理's expansion elsewhere.
func TestRoutesFixAction(t *testing.T) {
	u := routesFixture()
	u.issueAction("fix:ai")
	if u.routes.group != "ai" || !u.routes.pol {
		t.Errorf("on the tab: %+v", u.routes.group)
	}
	u.page = "home"
	u.issueAction("fix:media")
	if u.home.x != "routes" || u.home.sub != "media" {
		t.Errorf("elsewhere: %q %q", u.home.x, u.home.sub)
	}
	u.issueAction("fix:default")
	if u.home.x != "hero" {
		t.Errorf("default: %q", u.home.x)
	}
}
