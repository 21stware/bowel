package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/mihomo"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

// /proxies has no provider nodes: without the providers' lists every node
// looks untested.
func TestAddNodesKeepsGroupsAndAddsNodes(t *testing.T) {
	px := map[string]mihomo.Proxy{"默认": {Type: "Selector", Now: "a"}}
	addNodes(px, map[string]mihomo.Proxy{
		"a":  {Type: "Hysteria2", History: []mihomo.History{{Delay: 243}}},
		"默认": {Type: "Trojan"},
	})
	if px["a"].Type != "Hysteria2" || len(px["a"].History) != 1 {
		t.Fatalf("node not added: %+v", px["a"])
	}
	if px["默认"].Type != "Selector" {
		t.Fatalf("a node replaced a group: %+v", px["默认"])
	}
}

// Timing a site must not change a node's delay.
func TestHistoryOfIgnoresSiteTests(t *testing.T) {
	const test = "https://test.example/204"
	p := mihomo.Proxy{
		History: []mihomo.History{{Delay: 200}, {Delay: 900}},
		Extra: map[string]mihomo.URLTests{
			test:                 {History: []mihomo.History{{Delay: 200}}},
			"https://github.com": {History: []mihomo.History{{Delay: 900}}},
		},
	}
	if h := p.HistoryOf(test); len(h) != 1 || h[0].Delay != 200 {
		t.Fatalf("got %+v", h)
	}
	if h := p.HistoryOf("https://other.example"); len(h) != 0 {
		t.Fatalf("a URL never tested has no tests, got %+v", h)
	}
}

// A node's delay is the one on the main target of the route looked at.
func TestNodeOnRoute(t *testing.T) {
	n := NodeView{Delay: 100, Tested: true, Routes: map[string]NodeTest{"media": {Delay: 700, Tested: true, History: []int{700}}, "ai": {}}}
	if m := n.On("media"); m.Delay != 700 || !m.Tested || len(m.History) != 1 {
		t.Fatalf("media: %+v", m)
	}
	if a := n.On("ai"); a.Tested || a.Delay != 0 {
		t.Fatalf("a target not tested yet shows no delay: %+v", a)
	}
	if o := n.On("other"); o.Delay != 100 {
		t.Fatalf("no such route: %+v", o)
	}
}

// Only the main targets are in the core's config: changing the others
// must not reload it, which would close connections.
func TestAppOnlyFollowsMainTargets(t *testing.T) {
	old := settings.Defaults.Clone()
	sites := old.Clone()
	sites.RouteTargets = map[string][]routes.Target{"media": append(routes.DefaultTargets("media"), routes.Target{Name: "bbc.com", URL: "https://www.bbc.com"})}
	if !appOnly(old, sites) {
		t.Error("a new reference site reloaded the core")
	}
	main := old.Clone()
	main.RouteTargets = map[string][]routes.Target{"media": {{Name: "bbc.com", URL: "https://www.bbc.com"}}}
	if appOnly(old, main) {
		t.Error("a new main target didn't reload the core")
	}
}

func TestSiteViews(t *testing.T) {
	targets := []routes.Target{{Name: "A", URL: "https://a"}, {Name: "B", URL: "https://b"}}
	res := siteResult{node: "n1", sites: []SiteDelay{{Name: "A", URL: "https://a", Delay: 50, Tested: true}}}
	got := siteViews(targets, res, "n1")
	if !got[0].Tested || got[0].Delay != 50 || got[1].Tested || got[1].Name != "B" {
		t.Fatalf("got %+v", got)
	}
	// Results of the node used before say nothing of this one.
	if got = siteViews(targets, res, "n2"); got[0].Tested {
		t.Fatalf("stale result shown: %+v", got)
	}
}

func TestNodeTestKeepsTheLastTwelve(t *testing.T) {
	var h []mihomo.History
	for i := 1; i <= 15; i++ {
		h = append(h, mihomo.History{Delay: i})
	}
	if v := nodeTest(h); v.Delay != 15 || !v.Tested || len(v.History) != 12 || v.History[0] != 4 {
		t.Fatalf("got %+v", v)
	}
	if v := nodeTest(nil); v.Tested || v.History == nil {
		t.Fatalf("untested: %+v", v)
	}
}

// The dialog lists a route's targets, the first as the main one.
func TestTargetsDialog(t *testing.T) {
	u := fixtureUI()
	r := RouteView{ID: "media", Name: "流媒体", Main: routes.Target{Name: "YouTube", URL: "https://www.youtube.com/generate_204"},
		Targets: routes.DefaultTargets("media"), Sites: []SiteDelay{{Name: "Netflix", URL: "https://www.netflix.com", Delay: 321, Tested: true}}}
	u.snap.Routes = []RouteView{r}
	u.home.targetsOpen = true
	tt := ui.NewTesterWith(func(c *ui.Context) { u.targetsDialog(c, palette(c), r) }, 800, 600, ui.TesterOptions{})
	tt.Advance(time.Second)
	for _, want := range []string{"YouTube", "Netflix", "主目标", "设为主目标", "恢复默认"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
}
