package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/mihomo"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
	"github.com/oboo/moho/internal/speed"
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

// A route's other sites are the node's own tests of them, as the core keeps
// them: another node has its own.
func TestSiteViews(t *testing.T) {
	now := time.Now()
	targets := []routes.Target{{Name: "A", URL: "https://a"}, {Name: "B", URL: "https://b"}, {Name: "C", URL: "https://c"}}
	n1 := mihomo.Proxy{Extra: map[string]mihomo.URLTests{
		"https://a": {History: []mihomo.History{{Time: now, Delay: 50}}},
		"https://c": {History: []mihomo.History{{Time: now.Add(-time.Hour), Delay: 0}}},
	}}
	got := siteViews(nil, targets, "n1", n1, 30*time.Minute, now)
	if !got[0].Tested || got[0].Delay != 50 || got[0].Name != "A" || got[1].Tested || got[1].Name != "B" {
		t.Fatalf("got %+v", got)
	}
	if got[2].Tested {
		t.Fatalf("a failure from an hour ago still shows: %+v", got[2])
	}
	if got = siteViews(nil, targets, "n2", mihomo.Proxy{}, 30*time.Minute, now); got[0].Tested {
		t.Fatalf("a node that was never tested shows a result: %+v", got)
	}
}

// testedOn is a node whose tests of url are h.
func testedOn(url string, h []mihomo.History) mihomo.Proxy {
	return mihomo.Proxy{Extra: map[string]mihomo.URLTests{url: {History: h}}}
}

// A site that tells a refusal by its status turned the node away: the core
// has that test as a failure of the URL, with its delay among all the
// node's tests. It shows as a refusal, and a test nothing answered doesn't.
func TestARefusalIsToldFromNoAnswer(t *testing.T) {
	now := time.Now()
	at := now.Add(-time.Minute)
	const openai, claude = "https://api.openai.com/v1/models", "https://api.anthropic.com/v1/models"
	hk := mihomo.Proxy{
		History: []mihomo.History{{Time: at, Delay: 120}, {Time: at.Add(time.Second), Delay: 0}},
		Extra: map[string]mihomo.URLTests{
			openai: {History: []mihomo.History{{Time: at, Delay: 0}}},                  // answered, with 403
			claude: {History: []mihomo.History{{Time: at.Add(time.Second), Delay: 0}}}, // no answer
		},
	}
	var refused speed.Refusals
	refused.Note(map[string]mihomo.Proxy{"hk": hk})
	if v := nodeTest(&refused, "hk", openai, hk, time.Hour, now); !v.Tested || v.Delay != 0 || !v.Refused {
		t.Fatalf("a refusal: %+v", v)
	}
	if v := nodeTest(&refused, "hk", claude, hk, time.Hour, now); !v.Tested || v.Delay != 0 || v.Refused {
		t.Fatalf("no answer: %+v", v)
	}
	sites := siteViews(&refused, []routes.Target{{Name: "OpenAI", URL: openai}, {Name: "Claude", URL: claude}}, "hk", hk, time.Hour, now)
	if !sites[0].Refused || sites[1].Refused {
		t.Fatalf("sites: %+v", sites)
	}
}

func TestNodeTestKeepsTheLastTwelve(t *testing.T) {
	var h []mihomo.History
	for i := 1; i <= 15; i++ {
		h = append(h, mihomo.History{Delay: i})
	}
	if v := nodeTest(nil, "n", "https://a", testedOn("https://a", h), time.Hour, time.Now()); v.Delay != 15 || !v.Tested || len(v.History) != 12 || v.History[0] != 4 {
		t.Fatalf("got %+v", v)
	}
	if v := nodeTest(nil, "n", "https://a", mihomo.Proxy{}, time.Hour, time.Now()); v.Tested || v.History == nil {
		t.Fatalf("untested: %+v", v)
	}
}

// A Tailscale node that no site answers through reads as not tested, not
// timed out: it leads into a tailnet. A success it had still shows.
func TestNodeTestOfATailnetCountsSuccessesOnly(t *testing.T) {
	now := time.Now()
	failed := testedOn("https://a", []mihomo.History{{Time: now, Delay: 0}, {Time: now, Delay: 0}})
	failed.Type = "Tailscale"
	if v := nodeTest(nil, "home", "https://a", failed, time.Hour, now); v.Tested || len(v.History) != 0 {
		t.Fatalf("only failures: %+v", v)
	}
	exit := testedOn("https://a", []mihomo.History{{Time: now, Delay: 90}, {Time: now, Delay: 0}})
	exit.Type = "Tailscale"
	if v := nodeTest(nil, "home", "https://a", exit, time.Hour, now); !v.Tested || v.Delay != 90 || len(v.History) != 1 {
		t.Fatalf("through an exit node once: %+v", v)
	}
}

// A failure from before a long quiet stretch is not the node's state now:
// it reads as not tested, which no route counts against it.
func TestNodeTestForgetsAnOldFailure(t *testing.T) {
	now := time.Now()
	h := []mihomo.History{{Time: now.Add(-2 * time.Hour), Delay: 80}, {Time: now.Add(-time.Hour), Delay: 0}}
	if v := nodeTest(nil, "n", "https://a", testedOn("https://a", h), 30*time.Minute, now); v.Tested || v.Delay != 0 || len(v.History) != 2 {
		t.Fatalf("an hour-old failure: %+v", v)
	}
	if v := nodeTest(nil, "n", "https://a", testedOn("https://a", h), 2*time.Hour, now); !v.Tested || v.Delay != 0 {
		t.Fatalf("a failure still in its time: %+v", v)
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
