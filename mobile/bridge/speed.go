package bridge

import (
	"context"
	"time"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/mihomo"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
	"github.com/oboo/moho/internal/speed"
)

// testWorkers is how many requests through nodes run at once, whoever asked
// for them.
const testWorkers = 8

// shownNode is a row the interface shows: a node, with the delay it has on
// a route's main target.
type shownNode struct {
	Node  string `json:"node"`
	Route string `json:"route"`
}

// startSpeed makes the queues every speed test goes through: requests
// through nodes, and the providers' own checks.
func (x *bridge) startSpeed() {
	x.tests = speed.New(testWorkers, x.testTarget)
	x.tests.Idle = x.readTests
	x.sweeps = speed.New(2, x.sweepProvider)
}

// testTarget requests a URL through a node. The core keeps the result with
// the node, by URL, which is where it is read from. Without a core there is
// no test.
func (x *bridge) testTarget(t speed.Target) bool {
	x.mu.Lock()
	c, home := x.client, x.nodeHome[t.Node]
	x.mu.Unlock()
	if c == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), siteTimeout+time.Second)
	defer cancel()
	// A failure is the node's result too: the core has it as a delay of 0.
	if home != "" {
		_, _ = c.NodeDelay(ctx, home, t.Node, t.URL, siteTimeout)
	} else {
		_, _ = c.ProxyDelay(ctx, t.Node, t.URL, siteTimeout) // a config's own proxy or group
	}
	return true
}

// readTests shows the results when the last test asked for is over.
func (x *bridge) readTests() {
	if x.refresh(context.Background()) == nil {
		x.push()
	}
}

// sweepProvider has the core test a provider's nodes its own way: every
// node on every target the groups registered. name is the core's.
func (x *bridge) sweepProvider(name string) bool {
	x.mu.Lock()
	c := x.client
	x.mu.Unlock()
	if c == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = c.HealthCheck(ctx, name)
	return true
}

// setViewing is told what the interface shows now: rows of nodes, and the
// route whose other sites are on the page. While the tunnel is up, what it
// shows is kept tested.
func (x *bridge) setViewing(nodes []shownNode, sites string) {
	x.mu.Lock()
	x.shownNodes, x.shownSites = nodes, sites
	x.mu.Unlock()
	go x.keepMeasured()
}

// keepMeasured asks, in the background, for the tests nobody else runs,
// each once per test interval.
func (x *bridge) keepMeasured() {
	x.mu.Lock()
	px := x.proxies
	x.mu.Unlock()
	age := time.Duration(x.set.Get().URLTestInterval) * time.Second
	x.tests.Want(age, x.wanted(), func(t speed.Target) time.Time { return speed.Tested(px[t.Node], t.URL) })
}

// wanted is what keepMeasured asks for. One is the node a route is fixed
// on, on that route's main target: the core tests every node of a provider
// while traffic passes through any group that uses it, and then this costs
// nothing; with no traffic the core rests, and a fixed node would never be
// looked at again. Under a config's own routing the groups are the config's.
// The others are what the interface shows.
func (x *bridge) wanted() []speed.Target {
	s := x.set.Get()
	im := x.importedNow()
	all := x.routes()
	x.mu.Lock()
	px, prov, nodes, sites := x.proxies, x.provNodes, x.shownNodes, x.shownSites
	x.mu.Unlock()
	var out []speed.Target
	if im == nil {
		out = speed.Fixed(px, all, s)
	}
	for _, n := range nodes {
		if _, ok := px[n.Node]; ok {
			out = append(out, speed.Target{Node: n.Node, URL: s.TargetsOf(n.Route)[0].URL})
		}
	}
	if r, err := x.route(sites); err == nil {
		if n := x.nodeOf(px, prov, r, im); providerOf(n) != "" {
			for _, t := range s.TargetsOf(sites)[1:] {
				out = append(out, speed.Target{Node: n, URL: t.URL})
			}
		}
	}
	return out
}

// reading is how the view reads a node's tests.
type reading struct {
	urls  map[string]string // route id → its main target; nil under a config's own routing
	stale time.Duration     // how long a failure stands
	now   time.Time
}

func readingOf(s settings.Settings, im *core.Imported) reading {
	rd := reading{stale: speed.StaleAfter(s.HealthInterval, s.URLTestInterval), now: time.Now()}
	if im != nil {
		return rd
	}
	rd.urls = map[string]string{}
	for _, r := range routes.Build(s.Routing, nil) {
		if r.NodeGroups() {
			rd.urls[r.ID] = s.TargetsOf(r.ID)[0].URL
		}
	}
	return rd
}

// last is the delay tests end on, and whether that is a result: a failure
// older than rd.stale is none.
func (rd reading) last(tests []mihomo.History) (ms int, ok bool) {
	h, ok := speed.Last(tests, rd.stale, rd.now)
	return h.Delay, ok
}

// tests reads p: its tests of the default route's main target, and its
// delay on each other route's (nodeData.Routes). Under a config's own
// routing no target is bowel's, and a node shows what the core last
// measured, on whatever.
func (rd reading) tests(p mihomo.Proxy) (main []mihomo.History, others map[string]int) {
	if rd.urls == nil {
		return p.History, nil
	}
	others = map[string]int{}
	for id, u := range rd.urls {
		if ms, ok := rd.last(p.HistoryOf(u)); ok && id != routes.Default {
			others[id] = ms
		}
	}
	return p.HistoryOf(rd.urls[routes.Default]), others
}
