package main

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"
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
	tt := ui.NewTester(u.view, 1060, 1500) // room for the exit choice above the services, in any system's fonts
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

// TestRoutesAddGeoRule: a rule pasted as mihomo writes it adds its GeoSite
// list, which YouTube ships with, so it asks to move it first; and the
// GeoSite kind says what to type.
func TestRoutesAddGeoRule(t *testing.T) {
	u := routesFixture()
	u.selectGroup("ai")
	u.routes.svc = "claude"
	tt := ui.NewTester(u.view, 1060, 1500)
	tt.Advance(time.Second)

	typeInto(t, tt, "新规则", "GEOSITE,youtube,Proxy")
	if !hasTextPart(tt, "添加后：youtube · GeoSite 域名库") {
		t.Errorf("no preview: %q", tt.Texts())
	}
	shot(t, tt, "routes-geosite")
	tt.Key(0, ui.KeyEnter)
	tt.Advance(100 * time.Millisecond)
	if !hasTextPart(tt, "一条规则只能属于一个服务") {
		t.Fatalf("no conflict with YouTube: %q", tt.Texts())
	}
	if err := tt.Click("移过来"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(100 * time.Millisecond)
	s := u.shownSettings()
	if sv, ok := routes.FindRule(routes.Build(s.Routing, s.Rules), "GEOSITE,youtube"); !ok || sv.ID != "claude" {
		t.Errorf("after moving: %+v %v", sv, ok)
	}

	kind, ok := tt.AccessByRole(ui.RolePopUpButton, "匹配方式")
	if !ok {
		t.Fatalf("no kind picker: %q", tt.Texts())
	}
	tt.ClickAt(kind.Bounds.X+kind.Bounds.W/2, kind.Bounds.Y+kind.Bounds.H/2)
	tt.Frame()
	opts := map[string]ui.AccessInfo{}
	for _, a := range tt.AccessAll() {
		if a.Role == ui.RoleOption {
			opts[a.Name] = a
		}
	}
	geoip, ok := opts["GeoIP 地址库"]
	if _, site := opts["GeoSite 域名库"]; !ok || !site {
		t.Fatalf("kinds offered: %v", slices.Collect(maps.Keys(opts)))
	}
	tt.ClickAt(geoip.Bounds.X+geoip.Bounds.W/2, geoip.Bounds.Y+geoip.Bounds.H/2)
	tt.Advance(100 * time.Millisecond)
	if !hasTextPart(tt, "写 GeoIP 列表的名字") {
		t.Errorf("no word on GeoIP lists: %q", tt.Texts())
	}
	shot(t, tt, "routes-geoip-kind")
}

// TestRoutesEditRule: a rule of the user's is edited in its row: its value
// and kind keep it in its service and place, another route group puts it in
// that group's own rules, a rule another service has is refused, and
// Escape leaves it as it was.
func TestRoutesEditRule(t *testing.T) {
	u := routesFixture()
	mine := routes.UserRule{Kind: routes.KindSuffix, Value: "chat.example.com", Route: "ai", Service: "claude"}
	other := routes.UserRule{Kind: routes.KindDomain, Value: "x.example.com", Route: "ai", Service: "claude"}
	u.snap.Settings.Rules = []routes.UserRule{mine, other}
	u.selectGroup("ai")
	u.routes.svc = "claude"
	tt := ui.NewTester(u.view, 1060, 1500)
	tt.Advance(time.Second)

	edit := func(value string) {
		t.Helper()
		if err := tt.Click("编辑"); err != nil {
			t.Fatalf("%v: %q", err, tt.Texts())
		}
		tt.Frame()
		f, ok := tt.AccessByRole(ui.RoleTextField, "编辑规则")
		if !ok || f.Value != value {
			t.Fatalf("no field holding the rule: %+v %v", f, ok)
		}
	}
	rules := func() []routes.UserRule { return u.shownSettings().Rules }

	edit("chat.example.com")
	if !hasTextPart(tt, "保存后") {
		t.Errorf("no preview: %q", tt.Texts())
	}
	u.routes.editVal, u.routes.editKind = "", routes.KindKeyword
	typeInto(t, tt, "编辑规则", "chat")
	shot(t, tt, "routes-edit-rule")
	tt.Key(0, ui.KeyEnter)
	tt.Advance(100 * time.Millisecond)
	want := routes.UserRule{Kind: routes.KindKeyword, Value: "chat", Route: "ai", Service: "claude"}
	if got := rules(); len(got) != 2 || got[0] != want || got[1] != other {
		t.Fatalf("after Enter: %+v", got)
	}
	if _, ok := tt.AccessByRole(ui.RoleTextField, "编辑规则"); ok {
		t.Error("the row is still being edited")
	}

	edit("chat")
	u.routes.editRoute = "dev"
	tt.Frame()
	if err := tt.Click("保存"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(100 * time.Millisecond)
	want = routes.UserRule{Kind: routes.KindKeyword, Value: "chat", Route: "dev"}
	if got := rules(); len(got) != 2 || got[0] != want {
		t.Fatalf("to another group: %+v", got)
	}

	// x.example.com is left in Claude; netflix.com is Netflix's.
	edit("x.example.com")
	u.routes.editVal, u.routes.editKind = "netflix.com", routes.KindSuffix
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Advance(100 * time.Millisecond)
	if !hasTextPart(tt, "一条规则只能属于一个服务") || hasTextPart(tt, "移过来") {
		t.Fatalf("no refusal, or one offering to move: %q", tt.Texts())
	}
	shot(t, tt, "routes-edit-conflict")
	if got := rules(); !slices.Equal(got, []routes.UserRule{want, other}) {
		t.Fatalf("a refused edit changed the rules: %+v", got)
	}

	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if _, ok := tt.AccessByRole(ui.RoleTextField, "编辑规则"); ok || u.routes.editing != "" {
		t.Error("Escape did not end editing")
	}
	if got := rules(); !slices.Equal(got, []routes.UserRule{want, other}) {
		t.Fatalf("after Escape: %+v", got)
	}

	// The groups to pick from are in the sidebar's order: 其他流量 last.
	edit("x.example.com")
	sel, ok := tt.AccessByRole(ui.RolePopUpButton, "路由组")
	if !ok {
		t.Fatalf("no route select: %q", tt.Texts())
	}
	tt.ClickAt(sel.Bounds.X+sel.Bounds.W/2, sel.Bounds.Y+sel.Bounds.H/2)
	tt.Frame()
	var opts []ui.AccessInfo
	for _, a := range tt.AccessAll() {
		if a.Role == ui.RoleOption {
			opts = append(opts, a)
		}
	}
	slices.SortFunc(opts, func(a, b ui.AccessInfo) int { return cmp.Compare(a.Bounds.Y, b.Bounds.Y) })
	if len(opts) < 2 || opts[len(opts)-1].Name != "其他流量" {
		var names []string
		for _, o := range opts {
			names = append(names, o.Name)
		}
		t.Errorf("route options %q, want 其他流量 last", names)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()

	u.selectGroup("ai")
	if u.routes.editing != "" {
		t.Error("another group kept the edit")
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

// TestStrategyRegionStripWidth: the region strip inside the Routes tab's
// Node strategy card uses the card's actual width across window sizes (not a
// fixed width), leaving no dead gap before Search nodes when regions overflow.
func TestStrategyRegionStripWidth(t *testing.T) {
	checkBar := func(t *testing.T, tt *ui.Tester, label string) {
		t.Helper()
		all, ok := tt.AccessByRole(ui.RoleButton, "全部")
		if !ok {
			t.Fatalf("%s: no 全部 chip: %q", label, tt.Texts())
		}
		var strip ui.Rect
		var foundStrip bool
		cy := all.Bounds.Y + all.Bounds.H/2
		for _, a := range tt.AccessAll() {
			if a.Role == ui.RoleScroll && a.Bounds.Y <= cy && cy <= a.Bounds.Y+a.Bounds.H && (!foundStrip || a.Bounds.H < strip.H) {
				strip = a.Bounds
				foundStrip = true
			}
		}
		if !foundStrip || strip.H >= 80 {
			t.Fatalf("%s: no region scroll strip around y=%.1f (got %+v)", label, cy, strip)
		}
		btn, ok := tt.AccessByRole(ui.RoleButton, "全部测速")
		if !ok {
			t.Fatalf("%s: no 全部测速 button: %q", label, tt.Texts())
		}
		searchLeft := btn.Bounds.X - regionBarGap - regionSearchW
		foundField := false
		for _, a := range tt.AccessAll() {
			if a.Role == ui.RoleTextField && a.Bounds.Y <= cy && cy <= a.Bounds.Y+a.Bounds.H && a.Bounds.X >= searchLeft && a.Bounds.X+a.Bounds.W <= btn.Bounds.X {
				foundField = true
				break
			}
		}
		if !foundField {
			t.Fatalf("%s: no search field between %.1f and %.1f", label, searchLeft, btn.Bounds.X)
		}
		gap := searchLeft - (strip.X + strip.W)
		if gap < regionBarGap-1 || gap > 100 {
			t.Errorf("%s: strip %+v to searchLeft %.1f leaves gap %.1f (want within [%.0f, 100])", label, strip, searchLeft, gap, float32(regionBarGap-1))
		}
	}

	for _, mode := range []string{routes.ModeOrder, routes.ModeFixed} {
		u, _ := scopeFixture()
		u.routes.pol = false
		u.changePolicy("ai", func(q *routes.Policy) { q.Mode = mode })
		tt := ui.NewTester(u.view, 1060, 760)
		tt.Advance(time.Second)
		if err := tt.Click("节点策略"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(200 * time.Millisecond)
		checkBar(t, tt, mode+"-open-1060")

		tt.SetSize(1920, 760)
		tt.Advance(200 * time.Millisecond)
		checkBar(t, tt, mode+"-resize-1920")

		tt.SetSize(960, 760)
		tt.Advance(200 * time.Millisecond)
		checkBar(t, tt, mode+"-resize-960")
	}

	// Starting already open on frame 1 converges to the real width and stays idle.
	u, _ := scopeFixture()
	u.changePolicy("ai", func(q *routes.Policy) { q.Mode = routes.ModeOrder })
	var builds int
	tt := ui.NewTester(func(c *ui.Context) { builds++; u.view(c) }, 1920, 760)
	tt.Advance(200 * time.Millisecond)
	checkBar(t, tt, "already-open-1920")
	builds = 0
	tt.Advance(time.Second)
	if builds > 0 {
		t.Errorf("already-open-1920: %d builds in an idle second, want 0", builds)
	}
}

// scopeFixture is a group on 自动最快, among more regions than fit a line.
func scopeFixture() (*UI, []string) {
	u := routesFixture()
	u.selectGroup("ai")
	u.routes.pol = true
	proto := u.snap.Nodes[0]
	var regions []string
	for _, cc := range []string{"AE", "AR", "AU", "BR", "CA", "CH", "DE", "ES", "FR", "GB", "ID", "IN", "IT", "KR", "MO", "MX", "MY", "NL", "PH", "RU", "SE", "TH", "TR", "TW", "UA", "VN", "ZA"} {
		n := proto
		pl := *proto.Where.Place
		pl.Country = cc
		n.Name, n.Display, n.Where.Place = cc+"-1", cc+"-1", &pl
		u.snap.Nodes = append(u.snap.Nodes, n)
		regions = append(regions, cc)
	}
	return u, regions
}

// TestScopeChipsTakeEveryClick: a chip of 自动最快's range chooses and
// un-chooses from its top to its bottom edge, at any window width: nothing
// over it takes the press, and choosing moves no chip.
func TestScopeChipsTakeEveryClick(t *testing.T) {
	for _, width := range []int{1060, 760} {
		u, regions := scopeFixture()
		u.home.regionsMore = true // every region out, as the most crowded row is
		tt := ui.NewTester(u.view, width, 760)
		tt.Advance(time.Second)

		bounds := func(label string) ui.Rect {
			t.Helper()
			a, ok := tt.AccessByRole(ui.RoleButton, label)
			if !ok {
				t.Fatalf("width %d: no chip %q: %q", width, label, tt.Texts())
			}
			return a.Bounds
		}
		var providers, provLabels, regLabels []string
		for _, pv := range u.snap.Providers {
			if !pv.Enabled {
				continue
			}
			n := 0
			for _, v := range u.snap.Nodes {
				if v.Provider == pv.ID {
					n++
				}
			}
			providers = append(providers, pv.ID)
			provLabels = append(provLabels, fmt.Sprintf("%s %d", pv.Name, n))
		}
		for _, r := range regions {
			regLabels = append(regLabels, r+" 1")
		}

		lines := map[float32]bool{}
		var regionsEnd float32
		for _, l := range regLabels {
			b := bounds(l)
			if b.X < 0 || b.X+b.W > float32(width) {
				t.Errorf("width %d: %q cut off: %+v", width, l, b)
			}
			lines[b.Y] = true
			regionsEnd = max(regionsEnd, b.Y+b.H)
		}
		if len(lines) < 2 {
			t.Errorf("width %d: regions did not wrap, lines at %v", width, lines)
		}
		for _, l := range provLabels {
			if b := bounds(l); b.Y < regionsEnd {
				t.Errorf("width %d: %q at %v overlaps the regions, which end at %v", width, l, b.Y, regionsEnd)
			}
		}

		// click presses a chip at dy down it, the pointer having come to
		// rest over it first, as a person's does.
		click := func(b ui.Rect, dy float32) {
			x, y := b.X+b.W/2, b.Y+dy
			tt.Move(x, y)
			tt.Advance(50 * time.Millisecond)
			tt.Press(x, y)
			tt.Release(x, y)
			tt.Advance(50 * time.Millisecond)
		}
		// Clicks on and off at each height, the watched chips kept in place.
		exercise := func(label string, chosen func() bool, watch []string) {
			t.Helper()
			was := map[string]ui.Rect{}
			for _, l := range watch {
				was[l] = bounds(l)
			}
			b := bounds(label)
			for _, dy := range []float32{1, b.H / 2, b.H - 1} {
				for _, want := range []bool{true, false} {
					click(b, dy)
					if chosen() != want {
						t.Errorf("width %d: click %q at %v down: chosen %v, want %v", width, label, dy, !want, want)
					}
					for _, l := range watch {
						if now := bounds(l); now != was[l] {
							t.Errorf("width %d: click %q moved %q: %+v to %+v", width, label, l, was[l], now)
						}
					}
				}
			}
		}
		// A chosen region narrows the other row's counts and a chosen source
		// this row's, so only the row clicked keeps its labels throughout.
		for i, l := range regLabels {
			exercise(l, func() bool { return slices.Contains(u.policy("ai").Regions, regions[i]) }, regLabels)
		}
		for i, l := range provLabels {
			exercise(l, func() bool { return slices.Contains(u.policy("ai").Providers, providers[i]) }, provLabels)
		}
		shot(t, tt, fmt.Sprintf("routes-auto-many-%d", width))
	}
}

// TestScopeChipsKeepTheirWidth: a chip is as wide with a narrowed count as
// with the full one, so that choosing in one row moves nothing in the other.
// A smaller count is not narrower on screen: 0 is wider than 1.
func TestScopeChipsKeepTheirWidth(t *testing.T) {
	u, regions := scopeFixture()
	u.home.regionsMore = true
	tt := ui.NewTester(u.view, 760, 760)
	tt.Advance(time.Second)
	chips := func(count string) []ui.Rect {
		t.Helper()
		var out []ui.Rect
		for _, r := range regions {
			a, ok := tt.AccessByRole(ui.RoleButton, r+count)
			if !ok {
				t.Fatalf("no chip %q: %q", r+count, tt.Texts())
			}
			out = append(out, a.Bounds)
		}
		return out
	}
	was := chips(" 1")
	// The fixture's regions have their one node from the first source:
	// with another chosen, each counts none.
	other := slices.IndexFunc(u.snap.Providers, func(pv ProviderView) bool { return pv.Enabled && pv.ID != u.snap.Nodes[0].Provider })
	if other < 0 {
		t.Fatal("the fixture has one source only")
	}
	u.changePolicy("ai", func(q *routes.Policy) { q.Providers = []string{u.snap.Providers[other].ID} })
	tt.Frame()
	tt.Advance(50 * time.Millisecond)
	for i, now := range chips(" 0") {
		if now != was[i] {
			t.Errorf("%s: %+v with one node, %+v with none", regions[i], was[i], now)
		}
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

func ruleSetFixture() (*UI, string) {
	u := routesFixture()
	var id string
	u.snap.Settings.Routing = routes.Plan{}
	id = u.snap.Settings.Routing.AddRuleSet(nil, "ai", "global", "https://raw.example/rules/global.yaml")
	u.snap.RuleSets = map[string]RuleSetView{id: {Count: 349, Skipped: 2, UpdatedAt: time.Now().Add(-2 * time.Hour),
		Preview: []routes.Rule{{Text: "DOMAIN-SUFFIX,anthropic.com", Kind: routes.KindSuffix, Value: "anthropic.com"}}}}
	return u, id
}

// TestRuleSetRow: a rule set reads as a service, says how it stands, and
// has none of the controls for rules one writes by hand.
func TestRuleSetRow(t *testing.T) {
	u, id := ruleSetFixture()
	u.selectGroup("ai")
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(time.Second)
	for _, want := range []string{"global", "349 条规则", "2 小时前更新", "raw.example", "规则集"} {
		if !hasTextPart(tt, want) {
			t.Errorf("row lacks %q: %q", want, tt.Texts())
		}
	}
	shot(t, tt, "ruleset-row")

	if err := tt.Click("global"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(100 * time.Millisecond)
	for _, want := range []string{"https://raw.example/rules/global.yaml", "349 条规则，跳过 2 条不支持的", "每天自动更新", "*.anthropic.com", "还有 348 条", "更新", "停用", "删除", "移到…"} {
		if !hasTextPart(tt, want) {
			t.Errorf("editor lacks %q: %q", want, tt.Texts())
		}
	}
	if _, ok := tt.AccessByRole(ui.RoleTextField, "新规则"); ok {
		t.Error("a list someone else keeps has an input for new rules")
	}
	shot(t, tt, "ruleset-open")

	v := u.snap.RuleSets[id]
	v.Error = "HTTP 404"
	u.snap.RuleSets[id] = v
	tt = ui.NewTester(u.view, 1060, 760)
	tt.Advance(time.Second)
	if !hasTextPart(tt, "更新失败，继续使用上次的 349 条规则：HTTP 404") {
		t.Errorf("no failure: %q", tt.Texts())
	}
	delete(u.snap.RuleSets, id)
	tt = ui.NewTester(u.view, 1060, 760)
	tt.Advance(time.Second)
	if !hasTextPart(tt, "本地副本不见了") {
		t.Errorf("no word on the lost file: %q", tt.Texts())
	}
}

// TestRuleSetImportPanel: the panel opens from the group, knows a source it
// already has, and goes away.
func TestRuleSetImportPanel(t *testing.T) {
	u, _ := ruleSetFixture()
	u.selectGroup("ai")
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(time.Second)
	if err := tt.Click("＋ 导入规则集"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(100 * time.Millisecond)
	if !hasTextPart(tt, "粘贴规则集链接") {
		t.Fatalf("no panel: %q", tt.Texts())
	}
	shot(t, tt, "ruleset-import")
	typeInto(t, tt, "规则集来源", "https://raw.example/rules/global.yaml")
	if !hasTextPart(tt, "已经导入到") {
		t.Errorf("no word on the duplicate: %q", tt.Texts())
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(100 * time.Millisecond)
	if hasTextPart(tt, "已经导入到") || u.routes.rs.open {
		t.Error("the panel stayed")
	}
}

// TestDefaultExitChoice: 其他流量 comes last, chooses its exit before its
// node strategy, and has no strategy while it goes direct.
func TestDefaultExitChoice(t *testing.T) {
	for _, direct := range []bool{false, true} {
		u := routesFixture()
		for i := range u.snap.Routes {
			if u.snap.Routes[i].ID == routes.Default {
				u.snap.Routes[i].Direct = direct
			}
		}
		u.snap.Settings.Routing.ChangePolicy(routes.Default, func(q *routes.Policy) { q.Direct = direct })
		tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
		tt.Advance(time.Second)
		if err := tt.Click("其他流量"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
		for _, want := range []string{"走节点", "直接连接", "不经过节点，用本机网络", "以上规则都没命中"} {
			if !hasTextPart(tt, want) {
				t.Errorf("direct=%v lacks %q: %q", direct, want, tt.Texts())
			}
		}
		if got := hasTextPart(tt, "节点策略"); got == direct {
			t.Errorf("direct=%v: node strategy shown is %v", direct, got)
		}
		if got := hasTextPart(tt, "当前网络打不开的网站"); got != direct {
			t.Errorf("direct=%v: warning shown is %v", direct, got)
		}
		shot(t, tt, map[bool]string{false: "routes-other", true: "routes-other-direct"}[direct])

		u = fixtureUI()
		for i := range u.snap.Routes {
			if u.snap.Routes[i].ID == routes.Default {
				u.snap.Routes[i].Direct = direct
			}
		}
		u.home.x = "routes"
		tt = ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
		tt.Advance(2 * time.Second)
		if !hasTextPart(tt, "以上规则都没命中") || !hasTextPart(tt, "其他流量") {
			t.Errorf("home list: %q", tt.Texts())
		}
		shot(t, tt, map[bool]string{false: "home-routes", true: "home-routes-direct"}[direct])
	}
}

// TestGroupExitChoice: a route group other than 直连 and 拦截 chooses its
// exit as 其他流量 does. Going direct hides its node strategy and keeps it;
// it stays a group that can be renamed and deleted.
func TestGroupExitChoice(t *testing.T) {
	u := routesFixture()
	u.snap.Settings.Routing.SetStrategy("ai", routes.ModeFixed, "hk")
	tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
	tt.Advance(time.Second)
	if err := tt.Click("AI 服务"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	for _, want := range []string{"走节点", "不经过节点，用本机网络", "节点策略"} {
		if !hasTextPart(tt, want) {
			t.Errorf("on a node, lacks %q: %q", want, tt.Texts())
		}
	}
	if err := tt.Click("不经过节点，用本机网络"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if q := u.shownSettings().Routing.Policy("ai"); !q.Direct || q.Mode != routes.ModeFixed || q.Fixed != "hk" {
		t.Errorf("the policy after going direct: %+v", q)
	}
	if slices.Contains(tt.Texts(), "节点策略") || !hasTextPart(tt, "节点策略会保留") || hasTextPart(tt, "当前网络打不开的网站") {
		t.Errorf("direct: %q", tt.Texts())
	}
	if _, ok := tt.AccessByRole(ui.RoleTextField, "路由组名字"); !ok {
		t.Error("a group that goes direct can't be renamed")
	}
	if !hasTextPart(tt, "删除路由组") {
		t.Errorf("a group that goes direct can't be deleted: %q", tt.Texts())
	}
	shot(t, tt, "routes-group-direct")
	if err := tt.Click("走节点"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if q := u.shownSettings().Routing.Policy("ai"); q.Direct || q.Fixed != "hk" {
		t.Errorf("the policy back on a node: %+v", q)
	}
	for _, name := range []string{"直连", "拦截"} {
		if err := tt.Click(name); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
		if hasTextPart(tt, "走节点") || hasTextPart(tt, "不经过节点，用本机网络") {
			t.Errorf("%s chooses an exit: %q", name, tt.Texts())
		}
	}
}

// dragOnto drags the row showing from onto the row showing to.
func dragOnto(t *testing.T, tt *ui.Tester, from, to string) {
	t.Helper()
	a, ok := tt.Find(from)
	if !ok {
		t.Fatalf("no %q: %q", from, tt.Texts())
	}
	b, ok := tt.Find(to)
	if !ok {
		t.Fatalf("no %q: %q", to, tt.Texts())
	}
	ax, ay := a.X+a.W/2, a.Y+a.H/2
	bx, by := b.X+b.W/2, b.Y+b.H/2
	tt.Press(ax, ay)
	tt.Frame()
	for i := 1; i <= 4; i++ {
		tt.Move(ax+(bx-ax)*float32(i)/4, ay+(by-ay)*float32(i)/4)
		tt.Frame()
	}
	tt.Release(bx, by)
	tt.Advance(200 * time.Millisecond)
}

// TestDragToReorder: a route group dragged onto another takes its place,
// and so does a rule set within its group; 直连 and 拦截 move too, only
// 其他流量 neither moves nor takes a place. The list says what the order
// means.
func TestDragToReorder(t *testing.T) {
	u := routesFixture()
	plan := &u.snap.Settings.Routing
	g := plan.AddGroup(nil, "OpenAI")
	first := plan.AddRuleSet(nil, "ai", "宽的规则集", "https://x.example/a.yaml")
	second := plan.AddRuleSet(nil, "ai", "窄的规则集", "https://x.example/b.yaml")
	tt := ui.NewTesterWith(u.view, 1060, 860, ui.TesterOptions{})
	tt.Advance(time.Second)
	if !hasTextPart(tt, "规则集从上到下匹配") {
		t.Errorf("no word on what the order means: %q", tt.Texts())
	}
	order := func() []string {
		var out []string
		for _, c := range u.shownSettings().Routing.Groups {
			out = append(out, c.ID)
		}
		return out
	}
	dragOnto(t, tt, "OpenAI", "AI 服务")
	if got, want := order(), []string{routes.Default, g, "ai", "media", "dev", "direct", routes.Reject}; !slices.Equal(got, want) {
		t.Errorf("after dragging up: %v, want %v", got, want)
	}
	shot(t, tt, "routes-reordered")
	dragOnto(t, tt, "OpenAI", "开发")
	if got, want := order(), []string{routes.Default, "ai", "media", "dev", g, "direct", routes.Reject}; !slices.Equal(got, want) {
		t.Errorf("after dragging down: %v, want %v", got, want)
	}
	dragOnto(t, tt, "拦截", "AI 服务")
	if got, want := order(), []string{routes.Default, routes.Reject, "ai", "media", "dev", g, "direct"}; !slices.Equal(got, want) {
		t.Errorf("after dragging 拦截 to the top: %v, want %v", got, want)
	}
	dragOnto(t, tt, "OpenAI", "直连")
	if got, want := order(), []string{routes.Default, routes.Reject, "ai", "media", "dev", "direct", g}; !slices.Equal(got, want) {
		t.Errorf("after dragging past 直连: %v, want %v", got, want)
	}
	shot(t, tt, "routes-reordered-fixed")
	before := order()
	dragOnto(t, tt, "其他流量", "流媒体")
	dragOnto(t, tt, "流媒体", "其他流量")
	if got := order(); !slices.Equal(got, before) {
		t.Errorf("the default route moved or gave up its place: %v", got)
	}
	dragOnto(t, tt, "OpenAI", "直连") // back where the rest of the test has it
	dragOnto(t, tt, "拦截", "直连")
	if got, want := order(), []string{routes.Default, "ai", "media", "dev", g, "direct", routes.Reject}; !slices.Equal(got, want) {
		t.Fatalf("back again: %v, want %v", got, want)
	}

	if err := tt.Click("AI 服务"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	sets := func() []string {
		var out []string
		for _, l := range routes.Lines(routes.Build(u.shownSettings().Routing, nil), nil) {
			if l.Set != "" {
				out = append(out, l.Set)
			}
		}
		return out
	}
	if got := sets(); !slices.Equal(got, []string{first, second}) {
		t.Fatalf("sets before: %v", got)
	}
	dragOnto(t, tt, "窄的规则集", "宽的规则集")
	if got := sets(); !slices.Equal(got, []string{second, first}) {
		t.Errorf("sets after dragging the second onto the first: %v", got)
	}
	a, _ := tt.Find("窄的规则集")
	b, _ := tt.Find("宽的规则集")
	if a.Y >= b.Y {
		t.Errorf("the list still shows the old order: %v %v", a, b)
	}
	dragOnto(t, tt, "窄的规则集", "Claude") // a service and a set are not one another's place
	if got := sets(); !slices.Equal(got, []string{second, first}) {
		t.Errorf("sets after a drag onto a service: %v", got)
	}
}

// TestHomeRoutesDrag: on 今日's route board the groups are dragged to
// reorder them as in the tab, 直连 and 拦截 too, and shown in the new
// order at once; a press that doesn't move still opens a group.
func TestHomeRoutesDrag(t *testing.T) {
	u := routesFixture()
	u.page, u.home.x = "", "routes"
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(2 * time.Second)
	above := func(a, b string) bool {
		t.Helper()
		ra, ok := tt.Find(a)
		rb, ok2 := tt.Find(b)
		if !ok || !ok2 {
			t.Fatalf("no %q or %q: %q", a, b, tt.Texts())
		}
		return ra.Y < rb.Y
	}
	order := func() []string { return u.shownSettings().Routing.GroupOrder() }
	dragOnto(t, tt, "拦截", "AI 服务")
	tt.Advance(time.Second) // the names glide to their rows
	if got, want := order(), []string{routes.Default, routes.Reject, "ai", "media", "dev", "direct"}; !slices.Equal(got, want) {
		t.Errorf("after dragging 拦截 onto AI 服务: %v, want %v", got, want)
	}
	if !above("拦截", "AI 服务") {
		t.Error("拦截 isn't shown above AI 服务")
	}
	dragOnto(t, tt, "直连", "流媒体")
	tt.Advance(time.Second)
	if got, want := order(), []string{routes.Default, routes.Reject, "ai", "direct", "media", "dev"}; !slices.Equal(got, want) {
		t.Errorf("after dragging 直连 onto 流媒体: %v, want %v", got, want)
	}
	shot(t, tt, "home-routes-dragged")
	if u.home.sub != "" {
		t.Errorf("a drag opened group %q", u.home.sub)
	}
	dragOnto(t, tt, "AI 服务", "其他流量")
	if got, want := order(), []string{routes.Default, routes.Reject, "ai", "direct", "media", "dev"}; !slices.Equal(got, want) {
		t.Errorf("after a drop on 其他流量: %v", got)
	}
	r, _ := tt.Find("AI 服务")
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	if u.home.sub != "ai" {
		t.Errorf("a click no longer opens the group: sub %q", u.home.sub)
	}
}

// inView tells whether the box is wholly inside a window h tall.
func inView(r ui.Rect, h float32) bool { return r.H > 0 && r.Y >= 0 && r.Y+r.H <= h }

// TestRoutesOpeningKeepsTheRow: opening a service below the open one
// closes that one's editor, and the clicked row stays under the pointer.
func TestRoutesOpeningKeepsTheRow(t *testing.T) {
	u := routesFixture()
	u.selectGroup("ai")
	u.routes.svc = "openai"
	tt := ui.NewTester(u.view, 1060, 1400) // tall enough for Claude's editor once open, in any system's fonts
	tt.Advance(time.Second)
	tt.Wheel(700, 500, 0, 700) // OpenAI's editor above, partly scrolled away
	tt.Advance(time.Second)
	before, ok := tt.Find("Claude")
	if !ok || !inView(before, 1400) {
		t.Fatalf("Claude not in view: %v %v", before, ok)
	}
	tt.ClickAt(before.X+before.W/2, before.Y+before.H/2)
	tt.Advance(time.Second)
	if u.routes.svc != "claude" {
		t.Fatalf("open: %q", u.routes.svc)
	}
	after, _ := tt.Find("Claude")
	if d := after.Y - before.Y; d < -1 || d > 1 {
		t.Errorf("the row moved from y %.0f to %.0f", before.Y, after.Y)
	}
}

// TestRoutesMineStaysPut: 我添加的 opened from elsewhere comes into view,
// and its first rule doesn't move it to the top while the group shows.
// Shown again, the group lists it first.
func TestRoutesMineStaysPut(t *testing.T) {
	u := routesFixture()
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(time.Second)
	u.openRoutes("ai", routes.MineID("ai"))
	tt.Frame()
	tt.Advance(time.Second)
	before, ok := tt.Find("我添加的")
	if !ok || !inView(before, 760) {
		t.Fatalf("我添加的 not in view: %v %v", before, ok)
	}
	typeInto(t, tt, "新规则", "example.org")
	tt.Key(0, ui.KeyEnter)
	tt.Advance(time.Second)
	if got := u.shownSettings().Rules; len(got) != 1 || got[0].Service != "" || got[0].Route != "ai" {
		t.Fatalf("rules: %+v", got)
	}
	if after, _ := tt.Find("我添加的"); after.Y < before.Y-1 || after.Y > before.Y+1 {
		t.Errorf("我添加的 moved from y %.0f to %.0f", before.Y, after.Y)
	}
	if !inView(must(tt.Find("*.example.org")), 760) {
		t.Errorf("the new rule is out of view: %q", tt.Texts())
	}

	u.selectGroup(routes.Default)
	tt.Frame()
	u.selectGroup("ai")
	tt.Frame()
	tt.Frame()
	mine, _ := tt.Find("我添加的")
	openai, _ := tt.Find("OpenAI")
	if mine.Y >= openai.Y {
		t.Errorf("shown again, 我添加的 (y %.0f) is not above OpenAI (y %.0f)", mine.Y, openai.Y)
	}
}

// TestRoutesNewServiceInView: a new service, listed last, comes into view.
func TestRoutesNewServiceInView(t *testing.T) {
	u := routesFixture()
	u.selectGroup("ai")
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(time.Second)
	if err := tt.Click("＋ 新建服务"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if r, ok := tt.Find("新服务"); !ok || !inView(r, 760) {
		t.Errorf("新服务 not in view: %v %v", r, ok)
	}
}

func must(r ui.Rect, ok bool) ui.Rect { return r }

// TestPickNodes: 自动最快's range can be nodes ticked by hand. 挑选 lists
// every node, a click ticks one instead of fixing on it, and once done
// the list has only the ticked ones (#129).
func TestPickNodes(t *testing.T) {
	u := routesFixture()
	u.selectGroup("ai")
	u.routes.pol = true
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(time.Second)
	if err := tt.Click("挑选…"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Advance(100 * time.Millisecond)
	if !hasTextPart(tt, "勾选的节点就是范围") {
		t.Errorf("picking: %q", tt.Texts())
	}
	a, b := u.snap.Nodes[0], u.snap.Nodes[1]
	for _, n := range []NodeView{a, b} {
		if err := tt.Click(n.Display); err != nil {
			t.Fatal(err)
		}
		tt.Advance(100 * time.Millisecond)
	}
	q := u.policy("ai")
	if q.Mode != routes.ModeAuto || !slices.Equal(q.Nodes, []string{a.Name, b.Name}) {
		t.Fatalf("picked: %+v", q)
	}
	shot(t, tt, "routes-auto-picking")
	if !hasTextPart(tt, "已选 2") {
		t.Errorf("chip: %q", tt.Texts())
	}
	if err := tt.Click("完成"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(100 * time.Millisecond)
	shot(t, tt, "routes-auto-picked")
	// The list's rows: each node's name is a text of its own there, where
	// the rest of the page has it after a flag or a strategy.
	for _, n := range u.snap.Nodes {
		if want := n.Name == a.Name || n.Name == b.Name; slices.Contains(tt.Texts(), n.Display) != want {
			t.Errorf("%s shown: %v", n.Display, !want)
		}
	}
	// A pick that is gone is said.
	u.changePolicy("ai", func(q *routes.Policy) { q.Nodes = append(q.Nodes, "gone‹x") })
	tt = ui.NewTester(u.view, 1060, 760)
	tt.Advance(time.Second)
	if !hasTextPart(tt, "有 1 个已不在订阅里") {
		t.Errorf("gone: %q", tt.Texts())
	}
	if w := u.policyWord(u.route("ai")); !strings.Contains(w, "3 个节点") {
		t.Errorf("in words: %q", w)
	}
}

// TestPickTestsWhatShows (#282): a click on a node of a ranged 自动最快
// fixes the group on it, and the list then holds the nodes out of the range
// too, which were never tested on the group's main target. The rows that
// show are tested (keepMeasured), and that is all: here every one of them
// fails, as nodes out of an AI group's range do on its API, and a failure
// still stays above the untested rows instead of bringing one more into
// view. It did bring one, and the list tested all its nodes, a screenful
// at a time, never standing still.
func TestPickTestsWhatShows(t *testing.T) {
	u, _ := scopeFixture()
	cc := u.snap.Nodes[0].Where.Place.Country
	inRange := 0
	for i := range u.snap.Nodes {
		n := &u.snap.Nodes[i]
		// Every route's main target has its entry, as the snapshot gives
		// them; the fixture's copies of a node share one map.
		n.Routes = map[string]NodeTest{"ai": {}}
		if n.Where.Place != nil && n.Where.Place.Country == cc {
			n.Routes["ai"] = NodeTest{Delay: 150 + 10*i, Tested: true}
			inRange++
		}
	}
	u.changePolicy("ai", func(q *routes.Policy) { q.Mode, q.Regions = routes.ModeAuto, []string{cc} })
	tt := ui.NewTester(u.view, 1060, 900)
	tt.Advance(time.Second)
	tt.Frame()
	if got := len(u.seen["window"].pings); got != inRange {
		t.Fatalf("自动最快 drew %d rows, want the %d in its range", got, inRange)
	}
	pick := u.seen["window"].pings[0]
	// What selectNode does to the plan, without the core it needs.
	u.changePolicy("ai", func(q *routes.Policy) { q.Mode, q.Fixed = routes.ModeFixed, pick })
	var rounds []int
	for range len(u.snap.Nodes) {
		tt.Frame()
		tt.Frame() // a list's rows are in the frame after the one that built them
		shown, n := u.seen["window"].pings, 0
		for i := range u.snap.Nodes {
			v := &u.snap.Nodes[i]
			if slices.Contains(shown, v.Name) && !v.Routes["ai"].Tested {
				v.Routes["ai"] = NodeTest{Tested: true} // a failure
				n++
			}
		}
		if n == 0 {
			break
		}
		rounds = append(rounds, n)
	}
	if len(rounds) != 1 || rounds[0] == 0 {
		t.Errorf("tests after the pick, frame by frame: %v; want the rows in view once (%d nodes, %d in range)", rounds, len(u.snap.Nodes), inRange)
	}
}

// TestScopeExceptProviders (#272): picked sources can be the ones left out.
// 排除所选 shows once one is picked, crosses the picked ones out and counts
// the others' nodes; 全部 starts over at 只用.
func TestScopeExceptProviders(t *testing.T) {
	u, _ := scopeFixture()
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(time.Second)
	if tt.HasText("排除所选") {
		t.Error("排除所选 shows with no source picked")
	}
	pv := u.snap.Providers[slices.IndexFunc(u.snap.Providers, func(pv ProviderView) bool { return pv.Enabled })]
	u.changePolicy("ai", func(q *routes.Policy) { q.Providers = []string{pv.ID} })
	tt.Frame()
	tt.Advance(50 * time.Millisecond)
	if err := tt.Click("排除所选"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Frame()
	q := u.policy("ai")
	if !q.ExceptProviders || !slices.Equal(q.Providers, []string{pv.ID}) {
		t.Fatalf("after 排除所选: %+v", q)
	}
	for _, n := range u.snap.Nodes {
		if q.FromProvider(n.Provider) == (n.Provider == pv.ID) {
			t.Errorf("node %s of %s: in range %v", n.Name, n.Provider, q.FromProvider(n.Provider))
		}
	}
	shot(t, tt, "routes-scope-except")
	// The 全部 of 来源: the one in the row of 排除所选.
	ex, _ := tt.Find("排除所选")
	var all []ui.Rect
	for _, a := range tt.AccessAll() {
		if a.Role == ui.RoleButton && a.Name == "全部" {
			all = append(all, a.Bounds)
		}
	}
	at := slices.IndexFunc(all, func(r ui.Rect) bool { return r.Y <= ex.Y+ex.H/2 && ex.Y+ex.H/2 <= r.Y+r.H })
	if at < 0 {
		t.Fatalf("no 全部 in the row of 排除所选: %v", all)
	}
	tt.ClickAt(all[at].X+all[at].W/2, all[at].Y+all[at].H/2)
	tt.Frame()
	if q := u.policy("ai"); q.ExceptProviders || len(q.Providers) > 0 {
		t.Errorf("after 全部: %+v", q)
	}
}
