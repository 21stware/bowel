package routes

import (
	"slices"
	"strings"
	"testing"
)

func groupOf(t *testing.T, all []Route, id string) Route {
	t.Helper()
	for _, r := range all {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("no group %q", id)
	return Route{}
}

func serviceIn(r Route, id string) (Service, bool) {
	i := slices.IndexFunc(r.Services, func(s Service) bool { return s.ID == id })
	if i < 0 {
		return Service{}, false
	}
	return r.Services[i], true
}

func lineTexts(all []Route, users []UserRule) []string {
	var out []string
	for _, l := range Lines(all, users) {
		out = append(out, l.Text+"→"+l.Route)
	}
	return out
}

// TestBuildMatchesTemplate: with nothing changed the tree carries the
// template: the same groups, each with its services.
func TestBuildMatchesTemplate(t *testing.T) {
	all := Build(Plan{}, nil)
	var ids []string
	for _, r := range Template() {
		ids = append(ids, r.ID)
	}
	var got []string
	for _, r := range all {
		got = append(got, r.ID)
	}
	if !slices.Equal(ids, got) {
		t.Fatalf("groups %q, want %q", got, ids)
	}
	if _, ok := serviceIn(groupOf(t, all, "ai"), "claude"); !ok {
		t.Errorf("ai has no Claude: %+v", groupOf(t, all, "ai").Services)
	}
}

// TestRulesGoByTheList: the groups go in the order of the list, and the
// first with a rule that matches takes the connection. A narrower rule of
// the user's in 直连 comes after AI 服务's broader one, which takes its
// traffic, until 直连 is dragged above AI 服务. Inside a group the more
// specific rule comes first.
func TestRulesGoByTheList(t *testing.T) {
	users := []UserRule{
		{Kind: KindSuffix, Value: "api.anthropic.com", Route: "direct"},
		{Kind: KindDomain, Value: "x.example.com", Route: "direct"},
		{Kind: KindKeyword, Value: "tracker", Route: "direct"},
	}
	all := Build(Plan{}, users)
	lines := lineTexts(all, users)
	at := func(s string) int { return slices.Index(lines, s) }
	short, long := at("DOMAIN-SUFFIX,anthropic.com→ai"), at("DOMAIN-SUFFIX,api.anthropic.com→direct")
	exact, kw, cn := at("DOMAIN,x.example.com→direct"), at("DOMAIN-KEYWORD,tracker→direct"), at("GEOSITE,cn→direct")
	if short < 0 || long < 0 || exact < 0 || kw < 0 || cn < 0 {
		t.Fatalf("missing lines: %q", lines)
	}
	if !(short < exact && exact < long && long < kw && kw < cn) {
		t.Errorf("order: AI 服务's anthropic.com %d, then 直连's: exact %d, long %d, keyword %d, geo %d", short, exact, long, kw, cn)
	}
	if r, _, mine, _ := Explain("api.anthropic.com", users, all); r != "ai" || mine {
		t.Errorf("api.anthropic.com: %s %v, want AI 服务, listed above 直连", r, mine)
	}
	var p Plan
	if !p.MoveGroup("direct", "ai") {
		t.Fatal("直连 didn't move up")
	}
	if r, _, mine, _ := Explain("api.anthropic.com", users, Build(p, users)); r != "direct" || !mine {
		t.Errorf("with 直连 above AI 服务: %s %v", r, mine)
	}
}

// TestGooglePlayExcepted: Play's download hosts are under googleapis.cn,
// which the mainland list also holds. 国内网站 has them as its exception,
// shown with it: they go to the default route right before its rules,
// wherever 直连 is listed, and not while it is off.
func TestGooglePlayExcepted(t *testing.T) {
	var up Plan
	up.MoveGroup("direct", "ai")
	for name, p := range map[string]Plan{"as shipped": {}, "直连 first": up} {
		all := Build(p, nil)
		cn, ok := serviceIn(groupOf(t, all, "direct"), "cn")
		if !ok || !slices.Equal(cn.Except, []string{"googleapis.cn", "xn--ngstr-lra8j.com"}) {
			t.Fatalf("%s: 国内网站 %+v", name, cn)
		}
		lines := lineTexts(all, nil)
		at := func(s string) int { return slices.Index(lines, s) }
		play, list, ai := at("DOMAIN-SUFFIX,googleapis.cn→default"), at("GEOSITE,cn→direct"), at("DOMAIN-SUFFIX,openai.com→ai")
		if play < 0 || list < 0 || ai < 0 || play > list || (play < ai) != (name == "直连 first") {
			t.Errorf("%s: googleapis.cn at %d, the mainland list at %d, openai.com at %d", name, play, list, ai)
		}
		if r, rule, _, sure := Explain("play.googleapis.cn", nil, all); r != Default || rule != "DOMAIN-SUFFIX,googleapis.cn" || !sure {
			t.Errorf("%s: play.googleapis.cn: %s %s %v", name, r, rule, sure)
		}
	}
	var off Plan
	off.ToggleService("cn")
	if lines := lineTexts(Build(off, nil), nil); slices.Contains(lines, "DOMAIN-SUFFIX,googleapis.cn→default") {
		t.Error("an exception of 国内网站 while it is off")
	}
}

// TestNoDuplicateLines: a user's rule in a built-in service is one line.
func TestNoDuplicateLines(t *testing.T) {
	users := []UserRule{{Kind: KindSuffix, Value: "claude.example", Route: "ai", Service: "claude"}}
	all := Build(Plan{}, users)
	n := 0
	for _, l := range lineTexts(all, users) {
		if l == "DOMAIN-SUFFIX,claude.example→ai" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("the rule appears %d times", n)
	}
	s, ok := serviceIn(groupOf(t, all, "ai"), "claude")
	if !ok || !slices.ContainsFunc(s.Rules, func(r Rule) bool { return r.Value == "claude.example" && !r.Builtin }) {
		t.Errorf("not in its service: %+v", s)
	}
}

// TestOffServiceLeavesLines: a service off gives up its rules, so its
// traffic falls through; one rule of a service can be off on its own.
func TestOffServiceLeavesLines(t *testing.T) {
	var p Plan
	p.ToggleService("claude")
	if l := lineTexts(Build(p, nil), nil); slices.Contains(l, "DOMAIN-SUFFIX,claude.ai→ai") {
		t.Errorf("an off service's rule stayed: %q", l)
	}
	p.ToggleService("claude")
	p.ToggleRule("claude", "DOMAIN-SUFFIX,claude.ai")
	l := lineTexts(Build(p, nil), nil)
	if slices.Contains(l, "DOMAIN-SUFFIX,claude.ai→ai") || !slices.Contains(l, "DOMAIN-SUFFIX,anthropic.com→ai") {
		t.Errorf("one rule off: %q", l)
	}
}

// TestMoveSplitDelete: services move with their rules, a split makes a new
// group that keeps them, and deleting a group hands them on.
func TestMoveSplitDelete(t *testing.T) {
	users := []UserRule{{Kind: KindSuffix, Value: "mine.example", Route: "ai", Service: "claude"}}
	var p Plan
	p.MoveService(users, "claude", "dev")
	all := Build(p, users)
	if _, ok := serviceIn(groupOf(t, all, "dev"), "claude"); !ok {
		t.Fatal("not moved")
	}
	active := Active(all, users)
	if len(active) != 1 || active[0].Route != "dev" {
		t.Errorf("a moved service's own rule goes to %+v", active)
	}
	if l := lineTexts(all, users); !slices.Contains(l, "DOMAIN-SUFFIX,claude.ai→dev") {
		t.Errorf("built-in rule didn't move: %q", l)
	}

	g := p.SplitService(users, "claude")
	all = Build(p, users)
	if _, ok := serviceIn(groupOf(t, all, g), "claude"); !ok {
		t.Fatalf("not split into %q", g)
	}
	if !p.DeleteGroup(users, g, "media") {
		t.Fatal("delete refused")
	}
	all = Build(p, users)
	if _, ok := serviceIn(groupOf(t, all, "media"), "claude"); !ok {
		t.Error("the services weren't handed on")
	}
	if p.DeleteGroup(users, Default, "media") || p.DeleteGroup(users, "direct", "media") {
		t.Error("默认 or 直连 was deleted")
	}
}

// TestCustomGroups: ids are the smallest free, names unique, and a new
// group goes before 直连.
func TestCustomGroups(t *testing.T) {
	var p Plan
	a := p.AddGroup(nil, "新路由组")
	b := p.AddGroup(nil, "新路由组")
	if a != "g1" || b != "g2" {
		t.Fatalf("ids %q %q", a, b)
	}
	all := Build(p, nil)
	if groupOf(t, all, a).Label == groupOf(t, all, b).Label {
		t.Error("two groups share a name")
	}
	if n := len(all); all[n-2].ID != b || all[n-1].ID != "direct" {
		t.Errorf("last are %q, %q", all[n-2].ID, all[n-1].ID)
	}
	p.DeleteGroup(nil, a, Default)
	if c := p.AddGroup(nil, "x"); c != "g1" {
		t.Errorf("freed id not reused: %q", c)
	}
	if GroupNameError(all, a, "") == "" || GroupNameError(all, a, "其他流量") == "" || GroupNameError(all, a, "全新") != "" {
		t.Error("group name check")
	}
}

// TestStrategiesKeepTheirSettings: switching strategy loses nothing, and
// picking a node under any strategy fixes on it.
func TestStrategiesKeepTheirSettings(t *testing.T) {
	var p Plan
	p.ChangePolicy("ai", func(q *Policy) {
		q.Mode, q.Regions, q.Order = ModeAuto, []string{"JP"}, []string{"a", "b"}
	})
	p.SetStrategy("ai", ModeFixed, "cur")
	q := p.Policy("ai")
	if q.Mode != ModeFixed || q.Fixed != "cur" || !slices.Equal(q.Regions, []string{"JP"}) || len(q.Order) != 2 {
		t.Errorf("fixed: %+v", q)
	}
	p.SetStrategy("ai", ModeAuto, "other")
	p.PickNode("ai", "n")
	q = p.Policy("ai")
	if q.Mode != ModeFixed || q.Fixed != "n" || !slices.Equal(q.Regions, []string{"JP"}) {
		t.Errorf("picked: %+v", q)
	}
}

// TestClaim: a rule belongs to one service, so adding it elsewhere takes
// it from where a built-in had it.
func TestClaim(t *testing.T) {
	var p Plan
	r := UserRule{Kind: KindSuffix, Value: "claude.ai", Route: "dev", Service: ""}
	p.Claim(nil, r)
	users := []UserRule{r}
	all := Build(p, users)
	s, ok := FindRule(all, "DOMAIN-SUFFIX,claude.ai")
	if !ok || s.Group != "dev" {
		t.Errorf("claim landed in %+v", s)
	}
	if l := lineTexts(all, users); slices.Contains(l, "DOMAIN-SUFFIX,claude.ai→ai") {
		t.Errorf("the built-in kept it: %q", l)
	}
}

func TestFindMatched(t *testing.T) {
	all := Build(Plan{}, nil)
	for _, c := range []struct{ typ, payload, want string }{
		{"DomainSuffix", "claude.ai", "claude"},
		{"GeoSite", "cn", "cn"},
		{"GeoIP", "CN", "cn"},
	} {
		if s, ok := FindMatched(all, c.typ, c.payload); !ok || s.ID != c.want {
			t.Errorf("%s,%s = %+v %v", c.typ, c.payload, s, ok)
		}
	}
	if _, ok := FindMatched(all, "Match", ""); ok {
		t.Error("Match has a home")
	}
}

// TestRejectGroup: a plan saved before the group existed gets it, services
// can move in, and it can be neither renamed nor deleted.
func TestRejectGroup(t *testing.T) {
	p := Plan{Groups: []GroupConf{{ID: Default}, {ID: "ai"}, {ID: "g1", Name: "拦截"}, {ID: "direct"}}}
	r := groupOf(t, Build(p, nil), Reject)
	if !r.Reject || !r.Direct {
		t.Fatalf("group %+v", r)
	}
	if err := p.Check(); err != nil {
		t.Errorf("a group the user named the same: %v", err)
	}
	p.MoveService(nil, "openai", Reject)
	if svc := groupOf(t, Build(p, nil), Reject).Services; !slices.ContainsFunc(svc, func(s Service) bool { return s.ID == "openai" }) {
		t.Errorf("service didn't move in: %+v", svc)
	}
	p.RenameGroup(Reject, "x")
	if g := groupOf(t, Build(p, nil), Reject); g.Label != "" {
		t.Errorf("renamed: %q", g.Label)
	}
	if p.DeleteGroup(nil, Reject, Default) {
		t.Error("deleted")
	}
	if err := ValidTargets(Reject, []Target{{"a", "https://a.example"}}); err == nil {
		t.Error("a group with no node took test sites")
	}
}

func withRuleSet(t *testing.T, group string) (Plan, string) {
	t.Helper()
	var p Plan
	return p, p.AddRuleSet(nil, group, "AI list", "https://x.example/ai.yaml")
}

func TestRuleSetIsAServiceOfItsGroup(t *testing.T) {
	p, id := withRuleSet(t, "ai")
	all := Build(p, nil)
	s, ok := FindSource(all, "https://x.example/ai.yaml")
	if !ok || s.ID != id || s.Group != "ai" || s.Builtin {
		t.Fatalf("%+v %v", s, ok)
	}
	if got, ok := FindMatched(all, "RuleSet", RuleSetProvider(id)); !ok || got.ID != id {
		t.Errorf("a connection the core matched by the set: %+v %v", got, ok)
	}
	p.MoveService(nil, id, "media")
	if s, _ := FindSource(Build(p, nil), "https://x.example/ai.yaml"); s.Group != "media" {
		t.Errorf("moved to %s", s.Group)
	}
	p.ToggleService(id)
	if lines := Lines(Build(p, nil), nil); slices.ContainsFunc(lines, func(l Line) bool { return l.Set == id }) {
		t.Error("a set switched off still has a line")
	}
	if !p.Clone().findService(id).Off || p.findService(id).Source == "" {
		t.Errorf("%+v", p.findService(id))
	}
	if users := p.RemoveService(nil, id); len(users) != 0 || p.findService(id).ID != "" {
		t.Error("not removed")
	}
}

// A rule set goes with its group, after what the group ships: after every
// line of the groups above it, before every line of those below, the
// user's rules in them too.
func TestLinesOrderWithARuleSet(t *testing.T) {
	users := []UserRule{{Kind: KindKeyword, Value: "corp", Route: "direct"}}
	p, id := withRuleSet(t, "media")
	lines := Lines(Build(p, users), users)
	set := slices.IndexFunc(lines, func(l Line) bool { return l.Set == id })
	if set < 0 || lines[set].Text != "RULE-SET,"+RuleSetProvider(id)+",no-resolve" || lines[set].Route != "media" {
		t.Fatalf("no line for the set: %+v", lines)
	}
	for i, l := range lines {
		if l.Set != "" {
			continue
		}
		if above := l.Route == "ai" || l.Route == "media"; above != (i < set) {
			t.Errorf("%s of %s at %d, the set of 流媒体 at %d", l.Text, l.Route, i, set)
		}
	}
}

// 测试一个地址 looks inside the rule sets, which apply where their group is
// listed: a set in 流媒体 doesn't take what AI 服务, above it, ships, and
// does once 流媒体 is dragged above AI 服务.
func TestExplainLooksInsideRuleSets(t *testing.T) {
	p, id := withRuleSet(t, "media")
	match := func(set, host, ip string) (string, bool) {
		switch {
		case set != id:
		case host == "api.anthropic.com":
			return "DOMAIN-SUFFIX,anthropic.com", true
		case host == "cdn.example.net":
			return "DOMAIN-SUFFIX,example.net", true
		}
		return "", false
	}
	all := Build(p, nil)
	r, rule, set, mine, sure := ExplainSets("cdn.example.net", nil, all, match)
	if r != "media" || rule != "DOMAIN-SUFFIX,example.net" || set != id || mine || !sure {
		t.Errorf("what ships nowhere: %s %s %s %v %v", r, rule, set, mine, sure)
	}
	if r, rule, set, _, _ := ExplainSets("api.anthropic.com", nil, all, match); r != "ai" || rule != "DOMAIN-SUFFIX,anthropic.com" || set != "" {
		t.Errorf("Claude, in AI 服务 above the set: %s %s %s", r, rule, set)
	}
	// A rule set says nothing: on to what ships.
	if r, _, set, _, _ := ExplainSets("chat.openai.com", nil, all, match); r != "ai" || set != "" {
		t.Errorf("%s %s", r, set)
	}
	// Without a way to look inside, as on mobile.
	if r, _, _, _ := Explain("api.anthropic.com", nil, all); r != "ai" {
		t.Errorf("%s", r)
	}
	if !p.MoveGroup("media", "ai") {
		t.Fatal("流媒体 didn't move up")
	}
	if r, _, set, _, _ := ExplainSets("api.anthropic.com", nil, Build(p, nil), match); r != "media" || set != id {
		t.Errorf("with 流媒体 above AI 服务: %s %s", r, set)
	}
}

// TestRuleSetKeepsItsPlace is #294: a GFW list in a group of the user's
// below AI 服务 holds chat.com, which OpenAI ships. AI 服务 is listed first,
// so ChatGPT goes there; what OpenAI doesn't ship goes by the list; and
// dragging the group above AI 服务 lets the list have it all.
func TestRuleSetKeepsItsPlace(t *testing.T) {
	var p Plan
	g := p.AddGroup(nil, "国外穿墙")
	gfw := p.AddRuleSet(nil, g, "GFW", "https://x.example/gfw.txt")
	match := func(set, host, ip string) (string, bool) {
		if set == gfw && (host == "chat.com" || strings.HasSuffix(host, ".chat.com") || strings.HasSuffix(host, ".google.com")) {
			return "+." + host, true
		}
		return "", false
	}
	if r, _, set, _, _ := ExplainSets("ws.chat.com", nil, Build(p, nil), match); r != "ai" || set != "" {
		t.Errorf("ws.chat.com: %s %s, want AI 服务 by OpenAI's chat.com", r, set)
	}
	if r, _, set, _, _ := ExplainSets("www.google.com", nil, Build(p, nil), match); r != g || set != gfw {
		t.Errorf("www.google.com: %s %s, want the list's group", r, set)
	}
	p.MoveGroup(g, "ai")
	if r, _, set, _, _ := ExplainSets("ws.chat.com", nil, Build(p, nil), match); r != g || set != gfw {
		t.Errorf("with the group above AI 服务: %s %s", r, set)
	}
}

// TestGroupsGoDirect: a route group can go without a node and keeps its
// strategy for when it uses one again. The default route keeps its node
// groups too, for global mode; another keeps a group of its own, so that
// the core says whose the traffic was. 直连 and 拦截 have no such choice.
func TestGroupsGoDirect(t *testing.T) {
	var p Plan
	p.SetStrategy(Default, ModeFixed, "hk")
	p.ChangePolicy(Default, func(q *Policy) { q.Direct = true })
	p.SetStrategy("ai", ModeFixed, "jp")
	p.ChangePolicy("ai", func(q *Policy) { q.Direct = true })
	g := p.AddGroup(nil, "Apple")
	p.ChangePolicy(g, func(q *Policy) { q.Regions, q.Direct = []string{"US"}, true })
	for _, id := range []string{"direct", Reject} { // a hand-edited file
		p.ChangePolicy(id, func(q *Policy) { q.Direct = true })
	}
	all := Build(p, nil)
	route := func(id string) Route { return all[slices.IndexFunc(all, func(r Route) bool { return r.ID == id })] }
	def := route(Default)
	if !def.Direct || !def.NodeGroups() || def.Policy.Mode != ModeFixed || def.Policy.Fixed != "hk" {
		t.Errorf("default: %+v", def)
	}
	if def.OwnExit() {
		t.Error("the default route has a group for going direct: MATCH says whose the traffic is")
	}
	if ai := route("ai"); !ai.Direct || ai.NodeGroups() || !ai.OwnExit() || ai.Policy.Mode != ModeFixed || ai.Policy.Fixed != "jp" {
		t.Errorf("ai: %+v", ai)
	}
	if r := route(g); !r.Direct || r.NodeGroups() || !r.OwnExit() || !slices.Equal(r.Policy.Regions, []string{"US"}) {
		t.Errorf("a group of the user's: %+v", r)
	}
	for _, id := range []string{"direct", Reject} {
		if r := route(id); !r.Direct || r.NodeGroups() || r.OwnExit() || ChoosesExit(id) {
			t.Errorf("%s: %+v", id, r)
		}
	}
	p.ChangePolicy("ai", func(q *Policy) { q.Direct = false })
	if ai := Build(p, nil)[slices.IndexFunc(all, func(r Route) bool { return r.ID == "ai" })]; ai.Direct || !ai.NodeGroups() || ai.OwnExit() || ai.Policy.Fixed != "jp" {
		t.Errorf("ai back on a node: %+v", ai)
	}
	if err := p.Check(); err != nil {
		t.Error(err)
	}
}

// TestDefaultRouteName: the interface says 其他流量; the core's group keeps
// its name, and a group the user already called that doesn't break the plan.
func TestDefaultRouteName(t *testing.T) {
	var p Plan
	id := p.AddGroup(nil, "其他流量 2")
	p.group(id).Name = "其他流量"
	all := Build(p, nil)
	def := all[slices.IndexFunc(all, func(r Route) bool { return r.ID == Default })]
	if Title(def) != "其他流量" || Group(def) != "默认" {
		t.Errorf("title %q, group %q", Title(def), Group(def))
	}
	if err := p.Check(); err != nil {
		t.Error(err)
	}
}

func groupIDs(p Plan) []string {
	var out []string
	for _, g := range p.Groups {
		out = append(out, g.ID)
	}
	return out
}

func setOrder(p Plan) []string {
	var out []string
	for _, l := range Lines(Build(p, nil), nil) {
		if l.Set != "" {
			out = append(out, l.Route+"/"+l.Set)
		}
	}
	return out
}

// TestMoveGroup: a group takes another's place, the ones between shifting
// by one; 直连 and 拦截 move too, only the default route stays.
func TestMoveGroup(t *testing.T) {
	var p Plan
	g := p.AddGroup(nil, "OpenAI")
	want := func(ids ...string) {
		t.Helper()
		if got := groupIDs(p); !slices.Equal(got, ids) {
			t.Errorf("groups %v, want %v", got, ids)
		}
	}
	want(Default, Reject, "ai", "media", "dev", g, "direct")
	if !p.MoveGroup(g, "ai") {
		t.Error("moving up did nothing")
	}
	want(Default, Reject, g, "ai", "media", "dev", "direct")
	if !p.MoveGroup(g, "media") {
		t.Error("moving down did nothing")
	}
	want(Default, Reject, "ai", "media", g, "dev", "direct")
	if !p.MoveGroup("ai", "dev") {
		t.Error("moving further down did nothing")
	}
	want(Default, Reject, "media", g, "dev", "ai", "direct")
	for _, c := range [][2]string{{g, g}, {g, Default}, {Default, "ai"}, {g, "nope"}, {"nope", g}} {
		if p.MoveGroup(c[0], c[1]) {
			t.Errorf("%s moved to the place of %s", c[0], c[1])
		}
	}
	want(Default, Reject, "media", g, "dev", "ai", "direct")
	// 直连 and 拦截 move as the others do, and others past them.
	if !p.MoveGroup(Reject, "dev") {
		t.Error("拦截 didn't move down")
	}
	want(Default, "media", g, "dev", Reject, "ai", "direct")
	if !p.MoveGroup("dev", "direct") {
		t.Error("a group didn't move past 直连")
	}
	want(Default, "media", g, Reject, "ai", "direct", "dev")
	// A new group goes after the last that isn't fixed: here, at the end.
	h := p.AddGroup(nil, "Mine")
	want(Default, "media", g, Reject, "ai", "direct", "dev", h)
	if err := p.Check(); err != nil {
		t.Error(err)
	}
	// With 直连 moved up, after the last group, at the end too.
	var q Plan
	q.MoveGroup("direct", "ai")
	k := q.AddGroup(nil, "Mine")
	if got := groupIDs(q); !slices.Equal(got, []string{Default, Reject, "direct", "ai", "media", "dev", k}) {
		t.Errorf("new group after moving 直连 up: %v", got)
	}
}

// TestRuleSetsGoAsListed: rule sets apply in the order of the list, by
// their group and then their place in it, and the default route's last,
// as it is listed. Moving a group or a set changes it: of a broad set made
// first and a narrow one made later, the narrow one can be put first.
func TestRuleSetsGoAsListed(t *testing.T) {
	var p Plan
	other := p.AddRuleSet(nil, Default, "everything", "https://x.example/all.yaml")
	broad := p.AddRuleSet(nil, "ai", "AI", "https://x.example/ai.yaml")
	g := p.AddGroup(nil, "OpenAI")
	narrow := p.AddRuleSet(nil, g, "OpenAI", "https://x.example/openai.yaml")
	want := func(sets ...string) {
		t.Helper()
		if got := setOrder(p); !slices.Equal(got, sets) {
			t.Errorf("sets %v, want %v", got, sets)
		}
	}
	want("ai/"+broad, g+"/"+narrow, Default+"/"+other)
	p.MoveGroup(g, "ai")
	want(g+"/"+narrow, "ai/"+broad, Default+"/"+other)

	second := p.AddRuleSet(nil, "ai", "AI 2", "https://x.example/ai2.yaml")
	third := p.AddRuleSet(nil, "ai", "AI 3", "https://x.example/ai3.yaml")
	want(g+"/"+narrow, "ai/"+broad, "ai/"+second, "ai/"+third, Default+"/"+other)
	if !p.MoveRuleSet(third, broad) {
		t.Error("moving a set up did nothing")
	}
	want(g+"/"+narrow, "ai/"+third, "ai/"+broad, "ai/"+second, Default+"/"+other)
	if !p.MoveRuleSet(third, second) {
		t.Error("moving a set down did nothing")
	}
	want(g+"/"+narrow, "ai/"+broad, "ai/"+second, "ai/"+third, Default+"/"+other)
	svc := p.AddService(nil, "ai", "mine")
	for _, c := range [][2]string{{third, third}, {third, narrow}, {narrow, third}, {third, svc}, {svc, third}, {third, "openai"}, {"nope", third}} {
		if p.MoveRuleSet(c[0], c[1]) {
			t.Errorf("%s moved to the place of %s", c[0], c[1])
		}
	}
	want(g+"/"+narrow, "ai/"+broad, "ai/"+second, "ai/"+third, Default+"/"+other)

	// What decides between two sets is their order alone.
	inSet := func(id, host, ip string) (string, bool) {
		return "DOMAIN-SUFFIX,openai.example", (id == broad || id == narrow) && host == "api.openai.example"
	}
	if r, _, set, _, _ := ExplainSets("api.openai.example", nil, Build(p, nil), inSet); r != g || set != narrow {
		t.Errorf("with the narrow set first: %s %s", r, set)
	}
	p.MoveGroup(g, "dev")
	if r, _, set, _, _ := ExplainSets("api.openai.example", nil, Build(p, nil), inSet); r != "ai" || set != broad {
		t.Errorf("with the broad set first: %s %s", r, set)
	}
}

// TestGroupsGoInListOrder: all of a group's lines come before those of the
// groups below it, however specific, and dragging a group takes them all
// along. Inside a group the more specific rule comes first.
func TestGroupsGoInListOrder(t *testing.T) {
	var p Plan
	p.MoveGroup("dev", "ai")
	lines := Lines(Build(p, nil), nil)
	var order []string
	for i, l := range lines {
		if l.Route == Default { // 国内网站's exceptions, in 直连
			continue
		}
		if !slices.Contains(order, l.Route) {
			order = append(order, l.Route)
		} else if order[len(order)-1] != l.Route {
			t.Fatalf("%s's %q comes after %s's", l.Route, l.Text, order[len(order)-1])
		}
		if i > 0 && lines[i-1].Route == l.Route {
			ca, la := rank(lines[i-1].Text)
			cb, lb := rank(l.Text)
			if ca > cb || ca == cb && la > lb {
				t.Errorf("%q is before the more specific %q of the same group", lines[i-1].Text, l.Text)
			}
		}
	}
	if !slices.Equal(order, []string{"dev", "ai", "media", "direct"}) {
		t.Errorf("groups in the order %v", order)
	}
}

// TestAppRules: a rule for an application comes first in its group, before
// the group's rules by domain or address. It doesn't jump the groups above
// it: they take what of the program's traffic they match.
func TestAppRules(t *testing.T) {
	asOn(t, true)
	users := []UserRule{
		{Kind: KindIP, Value: "10.0.0.0/8", Route: "direct"},
		{Kind: KindApp, Value: "Telegram.exe", Route: "ai"},
		{Kind: KindApp, Value: "Google Chrome.app", Route: "direct"},
	}
	for name, plan := range map[string]Plan{
		"no set": {},
		"a set":  {Services: []ServiceConf{{ID: "s1", Custom: true, Name: "set", Group: "ai", Source: "https://example.com/a.list"}}},
	} {
		lines := lineTexts(Build(plan, users), users)
		at := func(s string) int { return slices.Index(lines, s) }
		tg, ai := at("PROCESS-NAME,Telegram.exe→ai"), at("DOMAIN-SUFFIX,openai.com→ai")
		chrome, ip := at("PROCESS-PATH-WILDCARD,*/Google Chrome.app/*→direct"), at("IP-CIDR,10.0.0.0/8,no-resolve→direct")
		if tg != 0 || !(tg < ai && ai < chrome && chrome < ip) {
			t.Errorf("%s: Telegram %d, openai.com %d, Chrome %d, 10/8 %d: %q", name, tg, ai, chrome, ip, lines[:min(4, len(lines))])
		}
	}
	if c, _ := rank("PROCESS-NAME,x"); c >= 0 {
		t.Errorf("rank of an application rule is %d, not before DOMAIN's 0", c)
	}
}

// TestTemplateGrows: a group the template gained after the groups were
// saved arrives, before 直连; one the user deleted stays gone.
func TestTemplateGrows(t *testing.T) {
	old := legacySeen
	defer func() { legacySeen = old }()
	legacySeen = []string{Default, "ai", "dev", "direct", Reject} // as if 流媒体 were new

	saved := Plan{Groups: []GroupConf{{ID: Default}, {ID: "ai"}, {ID: "g1", Name: "我的"}, {ID: "direct"}, {ID: Reject}}}
	var ids []string
	for _, r := range Build(saved, nil) {
		ids = append(ids, r.ID)
	}
	if want := []string{Default, "ai", "g1", "media", "direct", Reject}; !slices.Equal(ids, want) {
		t.Fatalf("groups %q, want %q", ids, want)
	}
	if _, ok := serviceIn(groupOf(t, Build(saved, nil), "media"), "youtube"); !ok {
		t.Error("the new group's services aren't in it")
	}

	// A change saves what the template had, and from then on a missing
	// group is one the user deleted.
	p := saved.Clone()
	p.RenameGroup("g1", "别的")
	if !hasGroup(p.Groups, "media") || !slices.Contains(p.Seen, "media") {
		t.Fatalf("media not saved: %+v, seen %q", p.Groups, p.Seen)
	}
	if !p.DeleteGroup(nil, "media", Default) {
		t.Fatal("media not deleted")
	}
	if all := Build(p, nil); slices.ContainsFunc(all, func(r Route) bool { return r.ID == "media" }) {
		t.Error("a deleted group came back")
	}
	if groupOf(t, Build(p, nil), "g1").Label != "别的" {
		t.Error("the user's group lost its name")
	}
}

// TestAIServices: the ai group holds the AI services people use, by their
// own hosts, and takes nothing from the hosts they share with others.
func TestAIServices(t *testing.T) {
	all := Build(Plan{}, nil)
	for host, want := range map[string]string{
		"api.individual.githubcopilot.com":     "copilot",
		"copilot-proxy.githubusercontent.com":  "copilot",
		"origin-tracker.githubusercontent.com": "copilot",
		"copilot.microsoft.com":                "mscopilot",
		"server.codeium.com":                   "windsurf",
		"windsurf.com":                         "windsurf",
		"api.groq.com":                         "groq",
		"www.meta.ai":                          "metaai",
		"api.replicate.com":                    "replicate",
		"pbxt.replicate.delivery":              "replicate",
		"api.elevenlabs.io":                    "elevenlabs",
		"chat.com":                             "openai",
		"ab.chatgpt.livekit.cloud":             "openai",
		"claude.new":                           "claude",
		"cloudcode-pa.googleapis.com":          "gemini",
		"daily-cloudcode-pa.googleapis.com":    "gemini",
		"cloudaicompanion.googleapis.com":      "gemini",
		"aiplatform.googleapis.com":            "gemini",
		"ai.google.dev":                        "gemini",
		"www.gemini.gstatic.com":               "gemini",
		"geminiweb-pa.clients6.google.com":     "gemini",
		"www.perplexity.com":                   "perplexity",
		"grok.x.com":                           "grok",
		"us.cursorvm.com":                      "cursor",
		"user-space.hf.space":                  "hf",
	} {
		route, rule, _, _ := Explain(host, nil, all)
		s, ok := FindRule(all, rule)
		if route != "ai" || !ok || s.ID != want {
			t.Errorf("%s goes to %q by %q of %q, want ai / %s", host, route, rule, s.ID, want)
		}
	}
	// Shared hosts keep their route: GitHub's stay 开发, Google's sign-in
	// and static hosts and LiveKit's shared ones match no suffix.
	for host, want := range map[string]string{
		"api.github.com": "dev", "github.com": "dev", "raw.githubusercontent.com": "dev", "avatars.githubusercontent.com": "dev",
		"accounts.google.com": "", "www.gstatic.com": "", "lh3.googleusercontent.com": "",
		"oauth2.googleapis.com": "", "storage.googleapis.com": "", "www.google.com": "",
		"turn.livekit.cloud": "", "www.bing.com": "", "x.com": "", "www.microsoft.com": "",
	} {
		if route, rule, _, _ := Explain(host, nil, all); route != want {
			t.Errorf("%s goes to %q by %q, want %q", host, route, rule, want)
		}
	}
	// No built-in rule is written twice, and every ai rule is a suffix.
	seen := map[string]string{}
	for _, b := range builtins {
		for _, r := range b.rules {
			if was, ok := seen[r]; ok {
				t.Errorf("%s is in both %s and %s", r, was, b.id)
			}
			seen[r] = b.id
			if b.group == "ai" && !strings.HasPrefix(r, "DOMAIN-SUFFIX,") {
				t.Errorf("%s of %s is not a suffix", r, b.id)
			}
		}
	}
	// A service moved or switched off before this version stays so, and
	// the hosts it gained go with it.
	p := Plan{}
	p.MoveService(nil, "claude", "dev")
	p.ToggleService("gemini")
	all = Build(p, nil)
	if route, _, _, _ := Explain("claude.new", nil, all); route != "dev" {
		t.Errorf("claude.new goes to %q after Claude moved to dev", route)
	}
	if route, _, _, _ := Explain("cloudcode-pa.googleapis.com", nil, all); route != "" {
		t.Errorf("cloudcode-pa.googleapis.com goes to %q with Gemini off", route)
	}
}

// TestFromProvider (#272): the picked providers are the only ones used, or
// with ExceptProviders the ones left out; none picked is every one.
func TestFromProvider(t *testing.T) {
	q := Policy{}
	if !q.FromProvider("a") {
		t.Error("none picked leaves a out")
	}
	q.ExceptProviders = true
	if !q.FromProvider("a") || q.Scoped() {
		t.Error("except with none picked narrows")
	}
	q.Providers = []string{"home"}
	if q.FromProvider("home") || !q.FromProvider("a") || !q.FromProvider("added-later") || !q.Scoped() {
		t.Error("except home")
	}
	q.ExceptProviders = false
	if !q.FromProvider("home") || q.FromProvider("a") {
		t.Error("only home")
	}
	if c := q.clone(); c.ExceptProviders != q.ExceptProviders {
		t.Error("clone")
	}
}
