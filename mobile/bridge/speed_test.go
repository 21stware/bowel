package bridge

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/mihomo"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/speed"
)

// fakeCore answers as the core does for one provider's two nodes behind the
// default route's groups, and keeps what it was asked to test.
type fakeCore struct {
	a, b, provider string // the nodes and their provider, as the core names them
	group, auto    string // the default route's selector and its automatic group

	mu     sync.Mutex
	now    string                   // what the selector is on
	nodes  map[string]*mihomo.Proxy // with the tests so far
	tests  []speed.Target           // a request through one node
	groups []string                 // a group tested all at once
	sweeps []string                 // a provider's own check
}

func (f *fakeCore) asked() []speed.Target {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := slices.Clone(f.tests)
	slices.SortFunc(out, func(x, y speed.Target) int { return strings.Compare(x.Node+x.URL, y.Node+y.URL) })
	return out
}

// tested gives node a test of url that long ago.
func (f *fakeCore) tested(node, url string, ago time.Duration, ms int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.nodes[node]
	if p.Extra == nil {
		p.Extra = map[string]mihomo.URLTests{}
	}
	h := mihomo.History{Time: time.Now().Add(-ago), Delay: ms}
	p.History = append(p.History, h)
	p.Extra[url] = mihomo.URLTests{History: append(p.Extra[url].History, h)}
}

// speedBridge is a bridge with the VPN on and a fake core behind it.
func speedBridge(t *testing.T) (*bridge, *fakeCore) {
	t.Helper()
	x := testBridge(t)
	id := x.store.List()[0].ID
	r, err := x.route(routes.Default)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeCore{a: "东京 01" + core.NodeSep + id, b: "香港 02" + core.NodeSep + id, provider: "p-" + id, group: routes.Group(r), auto: routes.Auto(r)}
	f.now = f.auto
	f.nodes = map[string]*mihomo.Proxy{f.a: {Name: f.a, Type: "Shadowsocks"}, f.b: {Name: f.b, Type: "Shadowsocks"}}

	answer := func(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) }
	mux := http.NewServeMux()
	mux.HandleFunc("GET /proxies", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		answer(w, map[string]any{"proxies": map[string]mihomo.Proxy{
			f.group: {Name: f.group, Type: "Selector", Now: f.now, All: []string{f.auto, f.a, f.b}},
			f.auto:  {Name: f.auto, Type: "URLTest", Now: f.a, All: []string{f.a, f.b}},
		}})
	})
	mux.HandleFunc("GET /providers/proxies", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		answer(w, map[string]any{"providers": map[string]any{f.provider: map[string]any{"name": f.provider, "proxies": []mihomo.Proxy{*f.nodes[f.a], *f.nodes[f.b]}}}})
	})
	mux.HandleFunc("GET /providers/proxies/{provider}/{node}/healthcheck", func(w http.ResponseWriter, q *http.Request) {
		node, url := q.PathValue("node"), q.URL.Query().Get("url")
		f.mu.Lock()
		f.tests = append(f.tests, speed.Target{Node: node, URL: url})
		f.mu.Unlock()
		f.tested(node, url, 0, 50)
		answer(w, map[string]int{"delay": 50})
	})
	mux.HandleFunc("GET /providers/proxies/{provider}/healthcheck", func(w http.ResponseWriter, q *http.Request) {
		f.mu.Lock()
		f.sweeps = append(f.sweeps, q.PathValue("provider"))
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("PUT /configs", func(w http.ResponseWriter, _ *http.Request) { // a reload: the nodes are new ones, with no tests
		f.mu.Lock()
		for _, p := range f.nodes {
			p.History, p.Extra = nil, nil
		}
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /group/{group}/delay", func(w http.ResponseWriter, q *http.Request) {
		f.mu.Lock()
		f.groups = append(f.groups, q.PathValue("group"))
		f.mu.Unlock()
		answer(w, map[string]int{})
	})

	dir, err := os.MkdirTemp("", "b") // a socket's path is short
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, socketName)
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	x.client, x.vpn = mihomo.NewUnix(sock), "on"
	if err := x.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	return x, f
}

// settle waits until the fake core was asked for n tests, and a little
// longer for one too many.
func (f *fakeCore) settle(t *testing.T, n int) []speed.Target {
	t.Helper()
	for end := time.Now().Add(3 * time.Second); len(f.asked()) < n && time.Now().Before(end); {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	return f.asked()
}

func mainOf(x *bridge, route string) string { return x.set.Get().TargetsOf(route)[0].URL }

// 全部测速 is one request through each node the route can use, and the
// route's other sites through the node it is on: not the group's all at once.
func TestTestRouteGoesNodeByNode(t *testing.T) {
	x, f := speedBridge(t)
	if err := x.testRoute(context.Background(), routes.Default); err != nil {
		t.Fatal(err)
	}
	targets := x.set.Get().TargetsOf(routes.Default)
	want := []speed.Target{{Node: f.a, URL: targets[0].URL}, {Node: f.b, URL: targets[0].URL}}
	for _, s := range targets[1:] {
		want = append(want, speed.Target{Node: f.a, URL: s.URL})
	}
	slices.SortFunc(want, func(x, y speed.Target) int { return strings.Compare(x.Node+x.URL, y.Node+y.URL) })
	if got := f.asked(); !slices.Equal(got, want) {
		t.Errorf("asked for %+v, want %+v", got, want)
	}
	if len(f.groups) != 0 {
		t.Errorf("the group was tested all at once: %v", f.groups)
	}
	// The results are the view's by the time the call is over.
	for _, n := range x.view().Nodes {
		if !n.Tested || n.Delay != 50 {
			t.Errorf("%s shows %+v after its test", n.Name, n)
		}
	}
}

// A provider's 测速 stays the provider's own check, through the service.
func TestTestProviderRunsTheProvidersOwnCheck(t *testing.T) {
	x, f := speedBridge(t)
	if err := x.testProvider(context.Background(), x.store.List()[0].ID); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.sweeps, []string{f.provider}) || len(f.asked()) != 0 {
		t.Errorf("sweeps %v, tests %v", f.sweeps, f.asked())
	}
}

// With the VPN on and nobody looking, only a node a route is fixed on is
// asked for; a node the core tested within the interval is left alone.
func TestKeepMeasuredAsksOnlyForFixedNodes(t *testing.T) {
	x, f := speedBridge(t)
	x.keepMeasured()
	if got := f.settle(t, 0); len(got) != 0 {
		t.Fatalf("every route chooses by itself, and %+v was asked for", got)
	}
	f.mu.Lock()
	f.now = f.b
	f.mu.Unlock()
	if err := x.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	x.keepMeasured()
	want := []speed.Target{{Node: f.b, URL: mainOf(x, routes.Default)}}
	if got := f.settle(t, 1); !slices.Equal(got, want) {
		t.Fatalf("asked for %+v, want %+v", got, want)
	}
	if err := x.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	x.keepMeasured()
	if got := f.settle(t, 2); len(got) != 1 {
		t.Fatalf("tested again within the interval: %+v", got)
	}
}

// The interface says which rows it shows; those are asked for and no others,
// each on the main target of the route it is shown for.
func TestViewingAsksOnlyForWhatItNames(t *testing.T) {
	x, f := speedBridge(t)
	bMu.Lock()
	b = x
	bMu.Unlock()
	t.Cleanup(func() {
		bMu.Lock()
		b = nil
		bMu.Unlock()
	})
	args, _ := json.Marshal(map[string]any{"nodes": []map[string]string{{"node": f.b, "route": "ai"}}})
	if _, err := Call("viewing", string(args)); err != nil {
		t.Fatal(err)
	}
	want := []speed.Target{{Node: f.b, URL: mainOf(x, "ai")}}
	if got := f.settle(t, 1); !slices.Equal(got, want) {
		t.Fatalf("asked for %+v, want %+v", got, want)
	}
	// A route's page shows its other sites, through the node the route is on.
	if _, err := Call("viewing", `{"sites":"default"}`); err != nil {
		t.Fatal(err)
	}
	sites := x.set.Get().TargetsOf(routes.Default)[1:]
	if len(sites) == 0 {
		t.Fatal("the default route has no other site to test")
	}
	got := f.settle(t, 1+len(sites))
	for _, s := range sites {
		if !slices.Contains(got, speed.Target{Node: f.a, URL: s.URL}) {
			t.Errorf("%s through %s was not asked for: %+v", s.URL, f.a, got)
		}
	}
	if len(got) != 1+len(sites) {
		t.Errorf("asked for %+v", got)
	}
	// Nothing shown is nothing wanted.
	if _, err := Call("viewing", `{}`); err != nil {
		t.Fatal(err)
	}
	if w := x.wanted(); len(w) != 0 {
		t.Errorf("still wanted with nothing shown: %+v", w)
	}
}

// A core that reloads has lost what it measured: a row still shown is asked
// for again then, not a test interval later.
func TestViewingAsksAgainAfterTheCoreReloads(t *testing.T) {
	x, f := speedBridge(t)
	x.setViewing([]shownNode{{Node: f.b, Route: "ai"}}, "")
	if got := f.settle(t, 1); len(got) != 1 {
		t.Fatalf("asked for %+v, want the row once", got)
	}
	ctx := context.Background()
	if err := x.reload(ctx, x.client); err != nil {
		t.Fatal(err)
	}
	if err := x.refresh(ctx); err != nil {
		t.Fatal(err)
	}
	x.keepMeasured()
	if got := f.settle(t, 2); len(got) != 2 {
		t.Errorf("after the reload it was asked for %+v, want the row again", got)
	}
}

// A row on a route that has no target (the direct one) is nothing to test.
func TestViewingARouteWithoutATargetAsksNothing(t *testing.T) {
	x, f := speedBridge(t)
	x.mu.Lock()
	x.shownNodes, x.shownSites = []shownNode{{Node: f.b, Route: "direct"}, {Node: f.b, Route: routes.Reject}}, "direct"
	x.mu.Unlock()
	if w := x.wanted(); len(w) != 0 {
		t.Errorf("wanted: %+v", w)
	}
}

// Nor is there anything to test on such a route by hand: it says so.
func TestTestRouteRefusesARouteWithoutNodes(t *testing.T) {
	x, f := speedBridge(t)
	for _, id := range []string{"direct", routes.Reject} {
		if err := x.testRoute(context.Background(), id); err == nil {
			t.Errorf("%s: tested without an error", id)
		}
	}
	if got := f.settle(t, 1); len(got) != 0 {
		t.Errorf("asked for %+v", got)
	}
}

// With the VPN off there is no core to ask: what is shown stays due, and
// is asked for when the core is back.
func TestViewingWaitsForTheCore(t *testing.T) {
	x, f := speedBridge(t)
	c := x.client
	x.disconnect()
	x.mu.Lock()
	x.proxies = map[string]mihomo.Proxy{f.b: *f.nodes[f.b]} // as the last view had them
	x.mu.Unlock()
	x.setViewing([]shownNode{{Node: f.b, Route: "ai"}}, "")
	if got := f.settle(t, 1); len(got) != 0 {
		t.Fatalf("asked a core that is not there for %+v", got)
	}
	x.mu.Lock()
	x.client = c
	x.mu.Unlock()
	x.keepMeasured()
	if got := f.settle(t, 1); len(got) != 1 {
		t.Errorf("with the core back it was asked for %+v, want the row", got)
	}
}

// A route's other sites are its node's own tests of them, as the core keeps
// them, once 测试这些网站 has run.
func TestSitesAreTheNodesOwnTests(t *testing.T) {
	x, f := speedBridge(t)
	if g := group(t, x, routes.Title(must(x.route(routes.Default)))); len(g.Sites) == 0 || g.Sites[0].Tested {
		t.Fatalf("before any test: %+v", g.Sites)
	}
	if err := x.testSites(context.Background(), routes.Default); err != nil {
		t.Fatal(err)
	}
	g := group(t, x, routes.Title(must(x.route(routes.Default))))
	for _, s := range g.Sites {
		if !s.Tested || s.Delay != 50 {
			t.Errorf("%s shows %+v", s.URL, s)
		}
	}
	for _, q := range f.asked() {
		if q.Node != f.a || q.URL == mainOf(x, routes.Default) {
			t.Errorf("asked for %+v", q)
		}
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// A node reads as each route tests it: on that route's main target.
func TestNodeViewReadsEachRoutesOwnTarget(t *testing.T) {
	now := time.Now()
	rd := reading{urls: map[string]string{routes.Default: "https://d", "ai": "https://ai", "media": "https://m"}, stale: 30 * time.Minute, now: now}
	p := mihomo.Proxy{
		History: []mihomo.History{{Time: now, Delay: 88}, {Time: now, Delay: 0}},
		Extra: map[string]mihomo.URLTests{
			"https://d":  {History: []mihomo.History{{Time: now, Delay: 88}}},
			"https://ai": {History: []mihomo.History{{Time: now, Delay: 700}, {Time: now, Delay: 0}}},
		},
	}
	n := nodeView("n", "a", p, rd)
	if !n.Tested || n.Delay != 88 || len(n.H) != 1 {
		t.Errorf("the node itself is the default route's: %+v", n)
	}
	if ms, tested := n.on("ai"); !tested || ms != 0 {
		t.Errorf("ai, which it just failed: %d %v", ms, tested)
	}
	if ms, tested := n.on("media"); tested {
		t.Errorf("a target never tested: %d", ms)
	}
	// Under a config's own routing no target is bowel's: the node shows what
	// the core last measured, on every route.
	own := nodeView("n", "a", p, reading{stale: 30 * time.Minute, now: now})
	if ms, tested := own.on("ai"); !tested || ms != 0 || len(own.H) != 2 || own.Routes != nil {
		t.Errorf("a config's own routing: %+v", own)
	}
}

// A failure from before a long quiet stretch reads as not tested, and no
// route counts it against the node.
func TestOldFailureIsNotTested(t *testing.T) {
	now := time.Now()
	rd := reading{urls: map[string]string{routes.Default: "https://d"}, stale: 30 * time.Minute, now: now}
	failed := func(ago time.Duration) mihomo.Proxy {
		h := []mihomo.History{{Time: now.Add(-ago), Delay: 0}}
		return mihomo.Proxy{History: h, Extra: map[string]mihomo.URLTests{"https://d": {History: h}}}
	}
	q := routes.Policy{Mode: routes.ModeFixed, Fixed: "n"}
	fresh := nodeView("n", "a", failed(time.Minute), rd)
	if !fresh.Tested || brokenWhy(q, []nodeData{fresh}, routes.Default, false) != "fixed" {
		t.Errorf("a failure a minute ago: %+v", fresh)
	}
	old := nodeView("n", "a", failed(time.Hour), rd)
	if old.Tested || brokenWhy(q, []nodeData{old}, routes.Default, false) != "" {
		t.Errorf("a failure an hour ago: %+v", old)
	}
}

// A route is broken by its own tests: a node that fails another route's
// target still carries this one.
func TestBrokenWhyReadsTheRoutesOwnTest(t *testing.T) {
	n := nodeData{Name: "n", Tested: true, Delay: 80, Routes: map[string]int{"ai": 0}}
	q := routes.Policy{Mode: routes.ModeFixed, Fixed: "n"}
	if why := brokenWhy(q, []nodeData{n}, routes.Default, false); why != "" {
		t.Errorf("default: %q", why)
	}
	if why := brokenWhy(q, []nodeData{n}, "ai", false); why != "fixed" {
		t.Errorf("ai: %q", why)
	}
}

// The note that a route moved off a failed node goes by the node's test of
// that route's main target, not by its last test of anything.
func TestSwitchNoteReadsTheRoutesTarget(t *testing.T) {
	x := testBridge(t)
	r := must(x.route(routes.Default))
	s := x.set.Get()
	url := s.TargetsOf(routes.Default)[0].URL
	now := time.Now()
	moved := func(from mihomo.Proxy) *switchNote {
		x.switched = nil
		x.lastNode = map[string]string{r.ID: "a"}
		x.proxies = map[string]mihomo.Proxy{
			routes.Group(r): {Now: routes.Auto(r)},
			routes.Auto(r):  {Now: "b"},
			"a":             from,
		}
		x.noticeSwitchesLocked([]routes.Route{r}, s)
		return x.switched
	}
	ok := mihomo.History{Time: now, Delay: 90}
	bad := mihomo.History{Time: now, Delay: 0}
	// It failed a site, and is fine on the route's own target.
	if n := moved(mihomo.Proxy{History: []mihomo.History{ok, bad}, Extra: map[string]mihomo.URLTests{url: {History: []mihomo.History{ok}}, "https://site": {History: []mihomo.History{bad}}}}); n != nil {
		t.Errorf("a failed site made the note %+v", n)
	}
	if n := moved(mihomo.Proxy{History: []mihomo.History{bad}, Extra: map[string]mihomo.URLTests{url: {History: []mihomo.History{bad}}}}); n == nil || n.route != r.ID {
		t.Errorf("no note for a node that failed the route's target: %+v", n)
	}
}
