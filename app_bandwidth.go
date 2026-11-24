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
	URL      string
}

// planBandwidth is what testBandwidth(routeID) would do now.
func (a *App) planBandwidth(routeID string) bandwidthPlan {
	r := a.route(routeID)
	a.mu.Lock()
	px := a.proxies
	a.mu.Unlock()
	s := a.settings.Get()
	plan := bandwidthPlan{URL: s.DownloadURL}
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
		if providerOf(t.Node) == "" {
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
	// The node in use is worth comparing with, whatever its delay.
	if n := currentNode(px, r); providerOf(n) != "" && !slices.Contains(plan.Nodes, n) {
		plan.Nodes = append(plan.Nodes, n)
	}
	plan.MaxBytes = int64(len(plan.Nodes)) * speed.DownloadCap
	return plan
}

// testBandwidth downloads the test file through each node of
// planBandwidth(routeID), one after another, through the probe listener.
// Nothing is chosen by it: the results show beside the delays.
func (a *App) testBandwidth(ctx context.Context, routeID string) error {
	plan := a.planBandwidth(routeID)
	if len(plan.Nodes) == 0 {
		return errors.New(i18n.T("没有测得通延迟的节点，先测一次延迟", "No node answered the delay test; test delays first"))
	}
	c, err := a.core.Client()
	if err != nil {
		return err
	}
	done := a.begin(bandwidthKey(routeID))
	defer done()
	failed := 0
	for _, n := range plan.Nodes {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		res := bandwidthResult{at: time.Now()}
		tctx, cancel := context.WithTimeout(ctx, speed.DownloadTime+15*time.Second)
		err := a.loc.Through(tctx, c, a.probePort, n, func(proxy *url.URL) error {
			client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}}
			t, err := speed.Download(tctx, client, plan.URL, speed.DownloadTime, speed.DownloadCap)
			res.bps = t.BitsPerSec
			return err
		})
		cancel()
		if err != nil {
			res.bps, res.failed = 0, true
			failed++
		}
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
		return errors.New(i18n.T("全部下载失败：测速文件可能打不开，或者被节点拦了。可以在设置里换一个测速文件", "Every download failed: the test file may be unreachable, or blocked by the nodes. You can pick another file in Settings"))
	}
	return nil
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
