package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func importedFixture() *UI {
	u := fixtureUI()
	node := u.snap.Nodes[0].Name
	u.snap.Imported = &ImportedView{Provider: u.snap.Providers[0].ID, Name: "Full config", Rules: 12, PausedRules: 3, PausedSets: 1, Groups: []GroupView{
		{Name: "Streaming", Kind: "select", Route: "g1", Rules: 9, Now: "Japan", Exit: node, Selectable: true, Members: []MemberView{{Name: "Japan", Group: true, Tested: true, Delay: 88}, {Name: "DIRECT"}}},
		{Name: "Japan", Kind: "url-test", Route: "default", Rules: 3, Now: "JP-01", Exit: node, Members: []MemberView{{Name: "JP-01", Node: node, Tested: true, Delay: 88}, {Name: "JP-02"}}},
		{Name: "Hidden", Kind: "fallback", Now: "JP-02", Exit: "JP-02", Members: []MemberView{{Name: "JP-02"}}},
	}}
	return u
}

func TestImportedPageShowsTheGroups(t *testing.T) {
	u := importedFixture()
	u.page = "routes"
	tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
	tt.Advance(2 * time.Second)
	shot(t, tt, "imported-routes")
	for _, s := range []string{"Full config", "Streaming", "Japan", "手动选择", "自动最快", "改回 bowel 路由", "DIRECT", "路由组"} {
		if !tt.HasText(s) {
			t.Errorf("page lacks %q: %q", s, tt.Texts())
		}
	}
	if tt.HasText("＋ 新建路由组") {
		t.Error("new route groups are offered while the config's routing is in use")
	}
	if tt.HasText("不会生效") {
		t.Errorf("a config that has nothing unused says something is: %q", tt.Texts())
	}
	// What of the config's own settings is not used is named.
	u.snap.Imported.Unused = []string{"dns", "tun", "listeners"}
	tt = ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
	tt.Advance(time.Second)
	shot(t, tt, "imported-routes-unused")
	if !tt.HasText("这份配置里的 dns、tun、listeners 不会生效：DNS、TUN 和本机端口用的是 bowel 自己的设置。") || !tt.HasText("新建路由组、改规则和测试一个地址现在不可用") {
		t.Errorf("unused settings: %q", tt.Texts())
	}
	u.snap.Imported.Unused = nil
	// A group only rules' targets lead to is read-only in its own words.
	u.routes.group = "Japan"
	tt = ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
	tt.Advance(time.Second)
	shot(t, tt, "imported-routes-urltest")
	if !tt.HasText("自动最快 · 3 条规则指向这里 · 由内核自己选择") {
		t.Errorf("url-test group: %q", tt.Texts())
	}
}

func TestProviderOffersItsRouting(t *testing.T) {
	u := fixtureUI()
	u.snap.Providers[0].Routing = ProviderRouting{Groups: 2, Rules: 5}
	u.page = "providers"
	tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
	tt.Advance(2 * time.Second)
	shot(t, tt, "imported-offer")
	if !tt.HasText("这份配置带 2 个路由组、5 条规则") || !tt.HasText("使用它们") {
		t.Errorf("no offer: %q", tt.Texts())
	}
	u.snap.Providers[0].Routing = ProviderRouting{Groups: 2, Rules: 5, InUse: true}
	tt = ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
	tt.Advance(time.Second)
	if !tt.HasText("正在使用它的 2 个路由组") || !tt.HasText("改回 bowel 路由") {
		t.Errorf("not in use: %q", tt.Texts())
	}
	u.snap.Providers[0].Routing = ProviderRouting{Problem: "「r」指向本地文件"}
	tt = ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
	tt.Advance(time.Second)
	if !tt.HasText("带路由组，但不能使用：「r」指向本地文件") {
		t.Errorf("no reason: %q", tt.Texts())
	}
	// A proxy-provider that is a local file can be given one from the row,
	// besides the add bar's own button; a local file can be read again.
	files := func() (n int) {
		for _, s := range tt.Texts() {
			if s == "选择文件…" {
				n++
			}
		}
		return n
	}
	before := files()
	u.snap.Providers[0].Routing = ProviderRouting{Problem: "「nodes」指向本地文件", NeedsFile: "nodes"}
	u.snap.Providers[2].Kind = "file"
	tt = ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
	tt.Advance(time.Second)
	shot(t, tt, "imported-needs-file")
	if files() != before+1 || !tt.HasText("重新读取") {
		t.Errorf("no file to choose or read again: %q", tt.Texts())
	}
}

// A config that brings a routing is asked about once, when it is added; the
// answer "only the nodes" leaves the line and its button, and changes nothing.
func TestProviderAsksAboutItsRoutingWhenAdded(t *testing.T) {
	u := fixtureUI()
	u.snap.Providers[0].Routing = ProviderRouting{Groups: 2, Rules: 5}
	u.page = "providers"
	u.provAdded(u.snap.Providers[0])
	tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
	tt.Advance(2 * time.Second)
	shot(t, tt, "imported-ask")
	for _, s := range []string{"这份配置自带 2 个路由组、5 条规则。要按它的路由分流，还是只用它的节点？", "按它的路由分流", "只用节点"} {
		if !tt.HasText(s) {
			t.Errorf("not asked %q: %q", s, tt.Texts())
		}
	}
	if tt.HasText("使用它们") {
		t.Error("the question and the line both show")
	}
	if err := tt.Click("只用节点"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if u.prov.offer != "" || tt.HasText("只用节点") || !tt.HasText("使用它们") || !tt.HasText("这份配置带 2 个路由组、5 条规则") {
		t.Errorf("after 只用节点: offer %q, %q", u.prov.offer, tt.Texts())
	}
	// Nodes alone, a config that can't be used, one already in use: no question.
	for _, r := range []ProviderRouting{{}, {Problem: "x"}, {Groups: 2, Rules: 5, InUse: true}} {
		u.prov.offer = ""
		pv := u.snap.Providers[0]
		pv.Routing = r
		if u.provAdded(pv); u.prov.offer != "" {
			t.Errorf("%+v is asked about", r)
		}
	}
}

// The page of a config in use offers to copy its DNS into the settings, and
// says what that replaces before it does.
func TestImportedPageOffersItsDNS(t *testing.T) {
	u := importedFixture()
	u.page = "routes"
	u.snap.Imported.Unused = []string{"dns", "hosts"}
	tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
	tt.Advance(2 * time.Second)
	if tt.HasText("把其中的 DNS 填入设置…") {
		t.Errorf("offered with nothing to copy: %q", tt.Texts())
	}
	u.snap.Imported.DNSFill = []string{"nameserver", "nameserver-policy", "hosts"}
	u.snap.Imported.DNSSkipped = []string{"nameserver-policy: rule-set:mine"}
	tt = ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
	tt.Advance(time.Second)
	shot(t, tt, "imported-dns-offer")
	if !tt.HasText("把其中的 DNS 填入设置…") || tt.HasText("确认填入") {
		t.Errorf("no offer: %q", tt.Texts())
	}
	if err := tt.Click("把其中的 DNS 填入设置…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	shot(t, tt, "imported-dns-confirm")
	for _, s := range []string{"将用这份配置的 nameserver、nameserver-policy、hosts 替换「设置 › DNS」里对应的几项，其余不动；DNS 模式、监听和劫持仍按 bowel 的。之后订阅更新不会再改它们。", "有 1 项填不进去，会跳过：nameserver-policy: rule-set:mine", "确认填入", "取消"} {
		if !tt.HasText(s) {
			t.Errorf("confirm lacks %q: %q", s, tt.Texts())
		}
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if tt.HasText("确认填入") || !tt.HasText("把其中的 DNS 填入设置…") {
		t.Errorf("after 取消: %q", tt.Texts())
	}
}

// A node that dials through a name the routing in use lacks can't connect:
// the row says how many and through what, and what to do when its config's
// routing would bring the name; the node is marked in the row's list.
func TestProviderSaysWhatItsNodesDialThrough(t *testing.T) {
	u := fixtureUI()
	id := u.snap.Providers[0].ID
	marked := 0
	for i := range u.snap.Nodes {
		if u.snap.Nodes[i].Provider == id && marked < 2 {
			u.snap.Nodes[i].MissingDialer = "入口"
			marked++
		}
	}
	u.page = "providers"
	u.prov.open = id
	u.snap.Providers[0].Routing = ProviderRouting{Groups: 2, Rules: 5}
	tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
	tt.Advance(2 * time.Second)
	shot(t, tt, "imported-dialer-offer")
	if !tt.HasText("2 个节点要经这份配置里的「入口」连接，只有使用这份配置的路由时才能连上") || !tt.HasText("经 入口") {
		t.Errorf("no line for an unused routing: %q", tt.Texts())
	}
	// A list of nodes has no routing to offer, and one in use that lacks
	// the name is not helped by using it.
	for _, r := range []ProviderRouting{{}, {Groups: 2, Rules: 5, InUse: true}} {
		u.snap.Providers[0].Routing = r
		tt = ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
		tt.Advance(time.Second)
		if !tt.HasText("2 个节点要经「入口」连接，现在的路由里没有这个名字，所以连不上") {
			t.Errorf("%+v: no line: %q", r, tt.Texts())
		}
	}
	shot(t, tt, "imported-dialer")
	for i := range u.snap.Nodes {
		u.snap.Nodes[i].MissingDialer = ""
	}
	tt = ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
	tt.Advance(time.Second)
	if tt.HasText("经 入口") {
		t.Errorf("a line with nothing missing: %q", tt.Texts())
	}
}
