package main

import (
	"context"
	"errors"
	"maps"
	"net"
	"slices"
	"time"

	"github.com/oboo/moho/internal/gateway"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/mihomo"
	"github.com/oboo/moho/internal/ping"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
	"github.com/oboo/moho/internal/speed"
	"github.com/oboo/moho/internal/tun"
)

const (
	pingTimeout = 3 * time.Second
	siteTimeout = 8 * time.Second
	// mainTimeout is the core's own for a health check, so that a main
	// target is judged here as the automatic groups judge it.
	mainTimeout = 5 * time.Second
	pingWorkers = 16
	// testWait is how long 全部测速 keeps its caller. The tests it asked for
	// go on after that, and their results show as they come.
	testWait = time.Minute
)

// testWorkers is how many requests through nodes run at once, whoever asked
// for them.
var testWorkers = 8

// pingResult is a node's last ping; ms is 0 when there's no answer, which
// says little: plenty of servers ignore ICMP.
type pingResult struct {
	ms int
	at time.Time
}

// pingEnv is what a ping needs besides its node, kept for a few seconds so
// that a run of pings reads each provider's file once.
type pingEnv struct {
	at      time.Time
	iface   string
	tunOn   bool
	servers map[string]map[string]string // provider id → node → server
}

// viewing is what a window drew that shows a test's result: requests
// through nodes, and pings of nodes.
type viewing struct {
	tests []speed.Target
	pings []string
}

// setViewing is told what window win ("window" or "panel") draws now.
// While it shows, what it draws is kept tested.
func (a *App) setViewing(win string, v viewing) {
	a.mu.Lock()
	if a.viewing == nil {
		a.viewing = map[string]viewing{}
	}
	a.viewing[win] = v
	a.mu.Unlock()
	go a.keepMeasured()
}

// startSpeed makes the queues every speed test goes through: requests
// through nodes, pings, and the providers' own checks.
func (a *App) startSpeed() {
	a.tests = speed.New(testWorkers, a.testTarget)
	a.tests.Idle = a.readTests
	a.pinger = speed.New(pingWorkers, a.pingNode)
	a.sweeps = speed.New(2, a.sweepProvider)
}

// keepMeasured asks, in the background, for the tests nobody else runs,
// each once per test interval. One is the node a route is fixed on, on that
// route's main target: the core tests every node of a provider while
// traffic passes through any group that uses it, and then this asks for
// nothing; with no traffic the core rests, and a fixed node would never be
// looked at again. Under a config's own routing the groups are the
// config's. The others are what the window and the menu bar panel draw,
// while they show.
func (a *App) keepMeasured() {
	s := a.settings.Get()
	a.mu.Lock()
	px, pings := a.proxies, a.pings
	window, panel := a.viewing["window"], a.viewing["panel"]
	a.mu.Unlock()
	var want viewing
	if a.importedNow() == nil {
		want.tests = speed.Fixed(px, a.routes(), s)
	}
	if a.winShown.Load() {
		want.tests, want.pings = append(want.tests, window.tests...), append(want.pings, window.pings...)
	}
	if u := a.uiRef(); u != nil && u.panel.win.Load() != nil {
		want.tests, want.pings = append(want.tests, panel.tests...), append(want.pings, panel.pings...)
	}
	age := time.Duration(s.URLTestInterval) * time.Second
	a.tests.Want(age, want.tests, func(t speed.Target) time.Time { return speed.Tested(px[t.Node], t.URL) })
	a.pinger.Want(age, want.pings, func(n string) time.Time { return pings[n].at })
}

// testTarget requests a URL through a node. The core keeps the result with
// the node, by URL, which is where it is read from. Without a core there
// is no test.
func (a *App) testTarget(t speed.Target) bool {
	c, err := a.core.Client()
	if err != nil {
		return false
	}
	timeout := siteTimeout
	s := a.settings.Get()
	for _, r := range a.routes() {
		if r.NodeGroups() && s.TargetsOf(r.ID)[0].URL == t.URL {
			timeout = mainTimeout
		}
	}
	a.mu.Lock()
	home := a.nodeHome[t.Node]
	a.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), timeout+time.Second)
	defer cancel()
	// A failure is the node's result too: the core has it as a delay of 0.
	if home != "" {
		_, _ = c.NodeDelay(ctx, home, t.Node, t.URL, routes.Expected(t.URL), timeout)
	} else {
		_, _ = c.ProxyDelay(ctx, t.Node, t.URL, routes.Expected(t.URL), timeout) // a config's own proxy or group
	}
	a.mu.Lock()
	a.nodesAt = time.Time{} // the result is in the providers' lists
	a.mu.Unlock()
	return true
}

// readTests shows the results when the last test asked for is over.
func (a *App) readTests() {
	a.mu.Lock()
	a.nodesAt = time.Time{}
	a.mu.Unlock()
	a.refresh()
}

// sweepProvider has the core test a provider's nodes its own way: every
// node on every target the groups registered.
func (a *App) sweepProvider(id string) bool {
	c, err := a.core.Client()
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = c.HealthCheck(ctx, "p-"+id)
	return true
}

// mainURLs is each route's main target; direct routes have none.
func mainURLs(s settings.Settings) map[string]string {
	out := map[string]string{}
	for _, r := range routes.Build(s.Routing, nil) {
		if r.NodeGroups() {
			out[r.ID] = s.TargetsOf(r.ID)[0].URL
		}
	}
	return out
}

// nodeTest is node name's tests of url, for the view. A failure older than
// stale is no result.
func nodeTest(refused *speed.Refusals, name, url string, node mihomo.Proxy, stale time.Duration, now time.Time) NodeTest {
	tests := node.ResultsOf(url)
	t := NodeTest{History: make([]int, 0, len(tests))}
	for _, h := range tests[max(0, len(tests)-12):] {
		t.History = append(t.History, h.Delay)
	}
	if h, ok := speed.Last(tests, stale, now); ok {
		t.Delay, t.Tested, t.Refused = h.Delay, true, refused.Is(name, url, h)
	}
	return t
}

// siteViews lists targets with the tests of them by node, whose name is
// name.
func siteViews(refused *speed.Refusals, targets []routes.Target, name string, node mihomo.Proxy, stale time.Duration, now time.Time) []SiteDelay {
	out := make([]SiteDelay, len(targets))
	for i, t := range targets {
		out[i] = SiteDelay{Name: t.Name, URL: t.URL}
		if h, ok := speed.Last(node.ResultsOf(t.URL), stale, now); ok {
			// A failure is 0, which the view shows as a timeout unless the
			// site refused.
			out[i].Delay, out[i].Tested, out[i].Refused = h.Delay, true, refused.Is(name, t.URL, h)
		}
	}
	return out
}

// pingNode measures the way to a node's server, around the core.
func (a *App) pingNode(n string) bool {
	c, err := a.core.Client()
	if err != nil {
		return false
	}
	a.pingMu.Lock()
	if time.Since(a.pingEnv.at) > 5*time.Second {
		// A TUN would answer in place of the servers, and its DNS with fake
		// addresses: leave through the real interface, with real addresses.
		st := a.settings.Get()
		env := pingEnv{at: time.Now(), servers: map[string]map[string]string{}}
		env.tunOn = a.takeoverOn(st) && st.Tun && tun.Installed(a.tunBin) && a.tunFault.Load() == nil
		if env.tunOn {
			env.iface = gateway.PrimaryInterface()
		}
		a.pingEnv = env
	}
	env, id := a.pingEnv, providerOf(n)
	if env.servers[id] == nil {
		env.servers[id] = a.providers.Servers(id)
	}
	host, ok := env.servers[id][display(n)]
	a.pingMu.Unlock()
	if env.tunOn && env.iface == "" {
		return false
	}
	res := pingResult{}
	if ok {
		res.ms = measurePing(c, host, env.iface, env.tunOn)
	}
	res.at = time.Now()
	a.mu.Lock()
	next := maps.Clone(a.pings)
	if next == nil {
		next = map[string]pingResult{}
	}
	next[n] = res
	a.pings = next
	a.mu.Unlock()
	a.emit()
	return true
}

func measurePing(c *mihomo.Client, host, iface string, tunOn bool) int {
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	ip, err := resolveServer(ctx, c, host, tunOn)
	if err != nil {
		return 0
	}
	ms, _ := ping.ICMP(ctx, ip, iface, pingTimeout)
	return ms
}

func resolveServer(ctx context.Context, c *mihomo.Client, host string, tunOn bool) (string, error) {
	if net.ParseIP(host) != nil {
		return host, nil
	}
	if tunOn {
		return c.LookupIP(ctx, host)
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
	if err != nil {
		return "", err
	}
	return ips[0].String(), nil
}

// setRouteTargets changes what a route is tested against; nil puts its
// defaults back. A new main target is tried first, through the node the
// route uses, because the automatic group would otherwise find every node
// dead on it and stop choosing.
func (a *App) setRouteTargets(routeID string, list []routes.Target) error {
	r := a.route(routeID)
	if r.ID == "" || !r.NodeGroups() {
		return errors.New(i18n.T("没有这条路由", "No such route"))
	}
	done := a.begin("targets:" + routeID)
	defer done()
	if list != nil {
		if err := routes.ValidTargets(routeID, list); err != nil {
			return err
		}
		if list[0].URL != a.settings.Get().TargetsOf(routeID)[0].URL {
			if err := a.tryTarget(r, list[0].URL); err != nil {
				return err
			}
		}
	}
	err := a.updateSettings(func(s *settings.Settings) {
		if list == nil {
			delete(s.RouteTargets, routeID)
			return
		}
		if s.RouteTargets == nil {
			s.RouteTargets = map[string][]routes.Target{}
		}
		s.RouteTargets[routeID] = slices.Clone(list)
	})
	if err != nil {
		return err
	}
	go func() { _ = a.testRoute(context.Background(), routeID) }()
	return nil
}

// tryTarget requests url through the node route r uses. Without a node
// there is nothing to try it with.
func (a *App) tryTarget(r routes.Route, url string) error {
	a.mu.Lock()
	node := currentNode(a.proxies, r)
	a.mu.Unlock()
	if providerOf(node) == "" {
		return nil
	}
	c, err := a.core.Client()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), siteTimeout+time.Second)
	defer cancel()
	if _, err := c.NodeDelay(ctx, "p-"+providerOf(node), node, url, routes.Expected(url), siteTimeout); err != nil {
		return errors.New(i18n.T("这个地址经当前节点没有响应，不能设为主目标", "This address didn't answer through the current node, so it can't be the main target"))
	}
	return nil
}
