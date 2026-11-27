package main

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"time"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/settings"
	"github.com/oboo/moho/internal/speed"
)

// bandwidthNodes is how many of a route's nodes a download test takes: the
// ones with the lowest delay on its main target. Each costs up to
// speed.DownloadCap of the user's traffic.
const bandwidthNodes = 5

// bandwidthResult is a node's last download test.
type bandwidthResult struct {
	bps    int64 // bits a second; 0 with failed
	failed bool
	at     time.Time
}

// bandwidthPlan is what a download test of a route would test, and what it
// may cost at most, for the dialog that asks first.
type bandwidthPlan struct {
	Nodes    []string
	MaxBytes int64
	Cap      int64 // the most downloaded through each node
	URL      string
	// Round: the route chooses by download speed, and this is a round of
	// its schedule run now, which puts it on the fastest download.
	Round bool
}

// planBandwidth is what a download test of route routeID by hand would do
// now, of listed, the rows its node list shows (the range in 自动最快, the
// region and the search otherwise): the few of them that answered fastest,
// and with them the node in use, whatever its delay or region. Of a group
// that chooses by download speed it is a Round, of the rows in its range
// alone.
func (a *App) planBandwidth(routeID string, listed []string) bandwidthPlan {
	r := a.route(routeID)
	shown := make(map[string]bool, len(listed))
	for _, n := range listed {
		shown[n] = true
	}
	if r.Policy.ByDownload() && !r.Direct {
		inRange := func(n string) bool { return a.pickInRange(r, n) }
		plan := a.planBandwidthOf(routeID, func(n string) bool { return shown[n] && inRange(n) }, inRange, scheduledCap)
		plan.Round = true
		return plan
	}
	return a.planBandwidthOf(routeID, func(n string) bool { return shown[n] }, func(string) bool { return true }, speed.DownloadCap)
}

// planBandwidthOf is the plan of a test of route routeID: of the nodes
// among says yes to, the bandwidthNodes with the lowest delay on its main
// target, and then the node in use if current says yes to it; up to
// capBytes each.
func (a *App) planBandwidthOf(routeID string, among, current func(string) bool, capBytes int64) bandwidthPlan {
	r := a.route(routeID)
	a.mu.Lock()
	px := a.proxies
	a.mu.Unlock()
	s := a.settings.Get()
	plan := bandwidthPlan{URL: s.DownloadURL, Cap: capBytes}
	if plan.URL == "" {
		plan.URL = settings.Defaults.DownloadURL // cleared in Settings, not yet saved back
	}
	if r.ID == "" || !r.NodeGroups() || a.importedNow() != nil {
		return plan
	}
	main := s.TargetsOf(routeID)[0].URL
	stale, now := speed.StaleAfter(s.HealthInterval, s.URLTestInterval), time.Now()
	type cand struct {
		node  string
		delay int
	}
	var list []cand
	for _, t := range speed.OfRoute(px, r, main) {
		if providerOf(t.Node) == "" || !among(t.Node) {
			continue
		}
		if h, ok := speed.Last(px[t.Node].HistoryOf(main), stale, now); ok && h.Delay > 0 {
			list = append(list, cand{t.Node, h.Delay})
		}
	}
	// The fastest to answer first: a node that is slow to answer is seldom
	// the fastest to download, and each test costs traffic.
	sort.SliceStable(list, func(i, j int) bool { return list[i].delay < list[j].delay })
	for _, c := range list[:min(len(list), bandwidthNodes)] {
		plan.Nodes = append(plan.Nodes, c.node)
	}
	// The node in use is worth comparing with, whatever its delay; with
	// nothing to compare it with, it is not worth the traffic alone.
	if n := currentNode(px, r); len(plan.Nodes) > 0 && providerOf(n) != "" && current(n) && !slices.Contains(plan.Nodes, n) {
		plan.Nodes = append(plan.Nodes, n)
	}
	plan.MaxBytes = int64(len(plan.Nodes)) * capBytes
	return plan
}

// testBandwidth runs plan, as the dialog showed it, on route routeID: one
// download through each of its nodes, one after another, through the probe
// listener. A test chooses nothing, and the results show beside the delays;
// a Round puts the group on the fastest download, as a scheduled one does,
// and the next is due from now.
func (a *App) testBandwidth(ctx context.Context, routeID string, plan bandwidthPlan) error {
	if plan.Round {
		return a.downloadRound(ctx, routeID, plan)
	}
	_, err := a.runBandwidth(ctx, routeID, plan, plan.Cap)
	return err
}

// runBandwidth downloads plan's file through each of its nodes, up to
// capBytes each, and keeps the results with the nodes. It returns them too.
func (a *App) runBandwidth(ctx context.Context, routeID string, plan bandwidthPlan, capBytes int64) (map[string]bandwidthResult, error) {
	if len(plan.Nodes) == 0 {
		return nil, errors.New(i18n.T("没有测得通延迟的节点，先测一次延迟", "No node answered the delay test; test delays first"))
	}
	c, err := a.core.Client()
	if err != nil {
		return nil, err
	}
	done := a.begin(bandwidthKey(routeID))
	defer done()
	out := map[string]bandwidthResult{}
	failed := 0
	for _, n := range plan.Nodes {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		res := bandwidthResult{at: time.Now()}
		tctx, cancel := context.WithTimeout(ctx, speed.DownloadTime+15*time.Second)
		err := a.loc.Through(tctx, c, a.probePort, n, func(proxy *url.URL) error {
			client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}}
			t, err := speed.Download(tctx, client, plan.URL, speed.DownloadTime, capBytes)
			res.bps = t.BitsPerSec
			return err
		})
		cancel()
		if err != nil {
			res.bps, res.failed = 0, true
			failed++
		}
		out[n] = res
		a.mu.Lock()
		next := maps.Clone(a.bandwidth)
		if next == nil {
			next = map[string]bandwidthResult{}
		}
		next[n] = res
		a.bandwidth = next
		a.mu.Unlock()
		a.emit()
	}
	if failed == len(plan.Nodes) {
		return out, errors.New(i18n.T("全部下载失败：测速文件可能打不开，或者被节点拦了。可以在设置里换一个测速文件", "Every download failed: the test file may be unreachable, or blocked by the nodes. You can pick another file in Settings"))
	}
	return out, nil
}

// downloadView is a node's last download test, for the view.
func downloadView(r bandwidthResult, ok bool) *DownloadTest {
	if !ok {
		return nil
	}
	return &DownloadTest{Mbps: float64(r.bps) / 1e6, Failed: r.failed, At: r.at}
}

// bandwidthKey is the pending key of a route's download test.
func bandwidthKey(routeID string) string { return "bandwidth:" + routeID }
