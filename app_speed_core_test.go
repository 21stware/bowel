package main

import (
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
	"github.com/oboo/moho/internal/speed"
)

// testHits counts the test requests that reach a local server, by path, and
// how many of one path were being answered at once.
type testHits struct {
	mu    sync.Mutex
	n     map[string]int
	now   map[string]int
	most  map[string]int
	last  time.Time
	pause time.Duration // how long an answer takes
}

func (h *testHits) serve(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	h.mu.Lock()
	h.n[p]++
	h.now[p]++
	h.most[p] = max(h.most[p], h.now[p])
	h.last = time.Now()
	pause := h.pause
	h.mu.Unlock()
	time.Sleep(pause)
	h.mu.Lock()
	h.now[p]--
	h.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// settle waits for the core's own tests of a start or a reload to be over,
// and returns the counts from then.
func (h *testHits) settle(t *testing.T) map[string]int {
	t.Helper()
	for end := time.Now().Add(40 * time.Second); ; time.Sleep(200 * time.Millisecond) {
		h.mu.Lock()
		quiet := !h.last.IsZero() && time.Since(h.last) > 3*time.Second
		if quiet {
			defer h.mu.Unlock()
			clear(h.most)
			return maps.Clone(h.n)
		}
		h.mu.Unlock()
		if time.Now().After(end) {
			t.Fatal("the test requests never stopped")
		}
	}
}

// since is what arrived after before, by path.
func (h *testHits) since(before map[string]int) (d, most map[string]int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	d = map[string]int{}
	for p, n := range h.n {
		if n -= before[p]; n > 0 {
			d[p] = n
		}
	}
	return d, maps.Clone(h.most)
}

// speedApp is routingApp with every route tested against a local server:
// the default route on /t/default, the others on /t/<id>, and no other
// sites. A test is one request: with unified delay, bowel's default, the
// core sends a second one on the same connection and times that.
func speedApp(t *testing.T, prepare func(*settings.Settings)) (*App, map[string]NodeView, *testHits) {
	h := &testHits{n: map[string]int{}, now: map[string]int{}, most: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(h.serve))
	t.Cleanup(srv.Close)
	a, n := routingApp(t, func(s *settings.Settings) {
		s.TestURL, s.UnifiedDelay = srv.URL+"/t/default", false
		s.RouteTargets = map[string][]routes.Target{}
		for _, id := range []string{"ai", "media", "dev"} {
			s.RouteTargets[id] = []routes.Target{{Name: id, URL: srv.URL + "/t/" + id}}
		}
		if prepare != nil {
			prepare(s)
		}
	})
	return a, n, h
}

// 全部测速 asks each of the route's nodes once, on that route's main
// target, and no more of them at once than the limit: here one.
func TestCoreTestRouteGoesNodeByNode(t *testing.T) {
	saved := testWorkers
	testWorkers = 1
	t.Cleanup(func() { testWorkers = saved })
	a, _, h := speedApp(t, nil)
	before := h.settle(t)
	h.mu.Lock()
	h.pause = 100 * time.Millisecond
	h.mu.Unlock()
	if err := a.testRoute(context.Background(), "ai"); err != nil {
		t.Fatal(err)
	}
	got, most := h.since(before)
	if len(got) != 1 || got["/t/ai"] != 3 {
		t.Errorf("requests: %v, want 3 on /t/ai and nothing else", got)
	}
	if most["/t/ai"] != 1 {
		t.Errorf("%d at once, want one after another", most["/t/ai"])
	}
	for _, n := range a.snapshot().Nodes {
		if m := n.On("ai"); !m.Tested || m.Delay == 0 {
			t.Errorf("%s shows no result on the route: %+v", n.Display, m)
		}
	}
}

// viewingApp is speedApp with its pings noted instead of sent, and a site
// no group of the core tests.
func viewingApp(t *testing.T) (a *App, node speed.Target, h *testHits, pinged func() []string) {
	a, n, h := speedApp(t, nil)
	var mu sync.Mutex
	var pings []string
	a.pinger = speed.New(1, func(n string) bool {
		mu.Lock()
		defer mu.Unlock()
		pings = append(pings, n)
		return true
	})
	site := strings.TrimSuffix(a.settings.Get().TestURL, "/t/default") + "/t/site"
	return a, speed.Target{Node: n["JP"].Name, URL: site}, h, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(pings)
	}
}

// refreshFor is the app's own refreshes, for d.
func refreshFor(a *App, d time.Duration) {
	for end := time.Now().Add(d); time.Now().Before(end); time.Sleep(500 * time.Millisecond) {
		a.refresh()
	}
}

// With the window showing, what it draws is tested when it is due, and
// nothing else: one row is one request and one ping.
func TestCoreViewingAsksOnlyForWhatIsDrawn(t *testing.T) {
	a, row, h, pinged := viewingApp(t)
	before := h.settle(t)
	a.winShown.Store(true)
	a.setViewing("window", viewing{tests: []speed.Target{row}, pings: []string{row.Node}})
	refreshFor(a, 3*time.Second)
	a.setViewing("window", viewing{tests: []speed.Target{row}, pings: []string{row.Node}}) // drawn again: just tested
	refreshFor(a, 2*time.Second)
	if got, _ := h.since(before); len(got) != 1 || got["/t/site"] != 1 {
		t.Errorf("requests: %v, want 1 on /t/site and nothing else", got)
	}
	if got := pinged(); !slices.Equal(got, []string{row.Node}) {
		t.Errorf("pings: %v, want one of %s", got, row.Node)
	}
	a.mu.Lock()
	tests := a.proxies[row.Node].HistoryOf(row.URL)
	a.mu.Unlock()
	if len(tests) != 1 || tests[0].Delay == 0 {
		t.Errorf("the node's tests of the site, as read from the core: %+v", tests)
	}
}

// A window that is hidden keeps nothing tested, whatever it drew last;
// shown again, it does.
func TestCoreViewingStopsWhenTheWindowHides(t *testing.T) {
	a, row, h, pinged := viewingApp(t)
	before := h.settle(t)
	a.setViewing("window", viewing{tests: []speed.Target{row}, pings: []string{row.Node}})
	refreshFor(a, 3*time.Second)
	if got, _ := h.since(before); len(got) != 0 || len(pinged()) != 0 {
		t.Fatalf("hidden, it asked for %v and pinged %v", got, pinged())
	}
	a.winShown.Store(true)
	a.keepMeasured() // as the next refresh of the providers' lists does
	refreshFor(a, 3*time.Second)
	if got, _ := h.since(before); len(got) != 1 || got["/t/site"] != 1 || len(pinged()) != 1 {
		t.Errorf("shown again: requests %v, pings %v; want one of each", got, pinged())
	}
}

// A core that reloads has lost what it measured: what the window still
// shows is asked for again then, not a test interval later.
func TestCoreViewingAsksAgainAfterTheCoreReloads(t *testing.T) {
	a, row, h, _ := viewingApp(t)
	before := h.settle(t)
	a.winShown.Store(true)
	a.setViewing("window", viewing{tests: []speed.Target{row}, pings: []string{row.Node}})
	refreshFor(a, 3*time.Second)
	if got, _ := h.since(before); got["/t/site"] != 1 {
		t.Fatalf("requests: %v, want 1 on /t/site", got)
	}
	if _, err := a.settings.Update(func(s *settings.Settings) { s.URLTestTolerance++ }); err != nil {
		t.Fatal(err)
	}
	if err := a.applyConfig(); err != nil { // as a change of the settings does
		t.Fatal(err)
	}
	refreshFor(a, 3*time.Second)
	if got, _ := h.since(before); got["/t/site"] != 2 {
		t.Errorf("after the reload: %d on /t/site, want it asked for again", got["/t/site"])
	}
	a.mu.Lock()
	tests := a.proxies[row.Node].HistoryOf(row.URL)
	a.mu.Unlock()
	if len(tests) == 0 || tests[len(tests)-1].Delay == 0 {
		t.Errorf("the node's tests of the site after the reload: %+v", tests)
	}
}

// With nothing on screen and no traffic, bowel tests one thing: the node a
// route was fixed on, once per test interval. The routes on automatic are
// the core's.
func TestCoreIdleKeepsOnlyFixedNodesTested(t *testing.T) {
	a, n, h := speedApp(t, func(s *settings.Settings) { s.URLTestInterval = 10 })
	if err := a.selectNode("ai", n["JP"].Name); err != nil {
		t.Fatal(err)
	}
	a.refresh()
	before := h.settle(t)
	for end := time.Now().Add(24 * time.Second); time.Now().Before(end); time.Sleep(time.Second) {
		a.mu.Lock()
		a.nodesAt = time.Time{} // as if the providers' lists were due each time
		a.mu.Unlock()
		a.refresh()
	}
	time.Sleep(500 * time.Millisecond)
	got, _ := h.since(before)
	if d := got["/t/ai"]; len(got) != 1 || d < 2 || d > 3 {
		t.Errorf("requests in 24 s at an interval of 10 s: %v, want 2 or 3 on /t/ai and nothing else", got)
	}
}

// A change that leaves the core's config as it was, as the system proxy's
// switch does, leaves the core alone: no reload, and so none of the tests
// the core runs of every node when it loads its providers (#200). A change
// that is in the config still reloads it, and applyConfig always does.
func TestCoreUnchangedConfigIsNotReloaded(t *testing.T) {
	a, _, h := speedApp(t, nil)
	before := h.settle(t)
	pid := a.core.Status().PID
	for range 3 {
		if err := a.applyConfigIfChanged(); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(3 * time.Second) // what a reload would have tested would show by now
	if got, _ := h.since(before); len(got) != 0 {
		t.Fatalf("an unchanged config: requests %v, want none", got)
	}
	if got := a.core.Status().PID; got != pid {
		t.Fatalf("the core's pid went %d → %d", pid, got)
	}

	// The same config, asked for as applyConfig asks: the core loads it.
	if err := a.applyConfig(); err != nil {
		t.Fatal(err)
	}
	if !h.arrives(before, "/t/default") {
		t.Fatal("applyConfig: the core did not test its nodes, as a reload has it do")
	}
	after := h.settle(t)

	// A change of the config, asked for as the switches ask.
	if _, err := a.settings.Update(func(s *settings.Settings) { s.URLTestTolerance++ }); err != nil {
		t.Fatal(err)
	}
	if err := a.applyConfigIfChanged(); err != nil {
		t.Fatal(err)
	}
	if !h.arrives(after, "/t/default") {
		t.Error("a changed config: the core did not test its nodes, as a reload has it do")
	}
}

// arrives waits a while for a request on path after before.
func (h *testHits) arrives(before map[string]int, path string) bool {
	for end := time.Now().Add(15 * time.Second); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		if got, _ := h.since(before); got[path] > 0 {
			return true
		}
	}
	return false
}

// A provider's 测速 asks each of its nodes once, on the default route's
// main target, and pings them.
func TestCoreTestProvider(t *testing.T) {
	a, n, h := speedApp(t, nil)
	var mu sync.Mutex
	var pings []string
	a.pinger = speed.New(1, func(n string) bool {
		mu.Lock()
		defer mu.Unlock()
		pings = append(pings, n)
		return true
	})
	before := h.settle(t)
	id := providerOf(n["JP"].Name)
	want := 0
	for _, v := range a.snapshot().Nodes {
		if v.Provider == id {
			want++
		}
	}
	if err := a.testProvider(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	got, _ := h.since(before)
	if want == 0 || len(got) != 1 || got["/t/default"] != want {
		t.Errorf("requests: %v, want %d on /t/default and nothing else", got, want)
	}
	mu.Lock()
	if len(pings) != want {
		t.Errorf("pinged %v, want %d nodes", pings, want)
	}
	mu.Unlock()
	if a.busy("test-provider:" + id) {
		t.Errorf("still marked as testing")
	}
	if err := a.testProvider(context.Background(), "nope"); err == nil {
		t.Errorf("a provider without nodes was tested")
	}
}
