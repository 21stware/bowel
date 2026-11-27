package main

import (
	"context"
	"encoding/json"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/oboo/moho/internal/mihomo"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
	"github.com/oboo/moho/internal/speed"
)

// A group that chooses by download speed (routes.Policy.ByDownload) has its
// download test run on a schedule, of the nodes in its range that answer
// fastest, and its selector put on the fastest download: a selector's
// position, so nothing reloads. The pick holds until the next test, or
// until it stops answering the group's main target: then the group is back
// on its automatic group, the fastest to answer, till the next test. Its
// 测下载 is a round run now (bandwidthPlan.Round).

const (
	// scheduledCap is the most a scheduled test downloads through a node:
	// less than a test asked for by hand, since nobody watches what it
	// costs. The rate settles well before.
	scheduledCap = 20 << 20
	// downloadRetry is how long a scheduled test that could not run (no
	// node had answered a delay test, no core) waits to try again.
	downloadRetry = 3 * time.Minute
	// keepWithin: a new test keeps the node in use while its download came
	// within a fifth of the fastest, so that two about as fast don't take
	// turns.
	keepWithin = 0.8
)

// downloadPick is what the last scheduled test of a group chose.
type downloadPick struct {
	Node string    `json:"node,omitempty"` // "" when nothing came through
	Bps  int64     `json:"bps,omitempty"`
	At   time.Time `json:"at"` // when the test ran: the next is due Every hours on
}

// downloadPicksFile is where the picks are kept, so that a restart neither
// loses them nor spends traffic on a test that isn't due.
func (a *App) downloadPicksFile() string { return filepath.Join(a.dataDir, "download-picks.json") }

func (a *App) loadDownloadPicks() {
	if a.dataDir == "" {
		return
	}
	b, err := os.ReadFile(a.downloadPicksFile())
	if err != nil {
		return
	}
	var m map[string]downloadPick
	if json.Unmarshal(b, &m) == nil {
		a.mu.Lock()
		a.picks = m
		a.mu.Unlock()
	}
}

func (a *App) saveDownloadPicks(m map[string]downloadPick) {
	if a.dataDir == "" {
		return
	}
	b, _ := json.Marshal(m)
	tmp := a.downloadPicksFile() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err == nil {
		err = os.Rename(tmp, a.downloadPicksFile())
		if err != nil {
			log.Println("download picks:", err)
		}
	}
}

func (a *App) setDownloadPick(routeID string, p downloadPick) {
	a.mu.Lock()
	next := maps.Clone(a.picks)
	if next == nil {
		next = map[string]downloadPick{}
	}
	next[routeID] = p
	a.picks = next
	a.mu.Unlock()
	a.saveDownloadPicks(next)
}

// dropDownloadPick forgets the node a group's last test chose, keeping when
// it ran: the group is on its fastest to answer until the next test.
func (a *App) dropDownloadPick(routeID string) {
	a.mu.Lock()
	p, ok := a.picks[routeID]
	a.mu.Unlock()
	if ok && p.Node != "" {
		a.setDownloadPick(routeID, downloadPick{At: p.At})
	}
}

// downloadDue says whether route r's scheduled test should run at now.
func downloadDue(r routes.Route, p downloadPick, ok bool, tried, now time.Time) bool {
	if !r.Policy.ByDownload() || r.Direct || !r.NodeGroups() {
		return false
	}
	if now.Sub(tried) < downloadRetry {
		return false
	}
	return !ok || now.Sub(p.At) >= time.Duration(r.Policy.DownloadEvery)*time.Hour || p.At.After(now)
}

// runDueDownloads starts the scheduled tests that are due, one group at a
// time. It is cheap when none is: refresh calls it every few seconds, once
// it has read the core. Takeover off is no reason to wait: apps that are
// pointed at the core's port go by these groups all the same (issue 267).
func (a *App) runDueDownloads() {
	if a.importedNow() != nil {
		return // the config's own groups: none of them chooses by download
	}
	now := time.Now()
	a.mu.Lock()
	if a.downloading {
		a.mu.Unlock()
		return
	}
	picks, tried := a.picks, a.downloadTried
	var due []routes.Route
	for _, r := range a.routes() {
		p, ok := picks[r.ID]
		if downloadDue(r, p, ok, tried[r.ID], now) && !a.pending[bandwidthKey(r.ID)] {
			due = append(due, r)
		}
	}
	if len(due) == 0 {
		a.mu.Unlock()
		return
	}
	a.downloading = true
	a.mu.Unlock()
	go func() {
		defer func() {
			a.mu.Lock()
			a.downloading = false
			a.mu.Unlock()
		}()
		for _, r := range due {
			a.scheduledDownload(r.ID)
		}
	}()
}

// scheduledDownload runs route routeID's scheduled test and puts it on the
// fastest download.
func (a *App) scheduledDownload(routeID string) {
	a.downloadTriedNow(routeID)
	plan := a.planRound(routeID)
	if len(plan.Nodes) == 0 {
		return // no delays yet: tried again in a while
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(len(plan.Nodes))*(speed.DownloadTime+20*time.Second))
	defer cancel()
	if err := a.downloadRound(ctx, routeID, plan); err != nil {
		log.Println("download round of", routeID+":", err)
	}
}

// planRound is the plan of a scheduled round of route routeID: of the nodes
// in the range of its automatic choice alone, the node in use too.
func (a *App) planRound(routeID string) bandwidthPlan {
	r := a.route(routeID)
	inRange := func(n string) bool { return a.pickInRange(r, n) }
	return a.planBandwidthOf(routeID, inRange, inRange, scheduledCap)
}

// downloadTriedNow notes that a round of route routeID starts now, so that
// none is started beside it, nor again before downloadRetry if it can't run.
func (a *App) downloadTriedNow(routeID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	next := maps.Clone(a.downloadTried)
	if next == nil {
		next = map[string]time.Time{}
	}
	next[routeID] = time.Now()
	a.downloadTried = next
}

// downloadRound runs plan, a round of route routeID's schedule, and puts
// the group on the fastest download. The next round is due from now. A
// round that tested nothing (no core, or cut short as the app quits) is not
// one: it is tried again in a while.
func (a *App) downloadRound(ctx context.Context, routeID string, plan bandwidthPlan) error {
	a.downloadTriedNow(routeID)
	a.mu.Lock()
	prev := a.picks[routeID]
	a.mu.Unlock()
	res, err := a.runBandwidth(ctx, routeID, plan, plan.Cap)
	if len(res) == 0 || (ctx.Err() != nil && len(res) < len(plan.Nodes)) {
		return err
	}
	if !a.route(routeID).Policy.ByDownload() {
		return err // it went back to delay meanwhile: the results show, and nothing is chosen
	}
	pick := choosePick(res, prev.Node, time.Now())
	if err != nil {
		pick.Node, pick.Bps = "", 0
	}
	a.setDownloadPick(routeID, pick)
	logPolicyErr(a.followPolicies())
	a.refresh()
	return err
}

// choosePick is the node a test's results choose: the fastest download,
// unless the node chosen before, prev, came within keepWithin of it.
func choosePick(res map[string]bandwidthResult, prev string, now time.Time) downloadPick {
	out := downloadPick{At: now}
	for n, r := range res {
		if r.failed || r.bps <= 0 {
			continue
		}
		if r.bps > out.Bps || r.bps == out.Bps && n < out.Node {
			out.Node, out.Bps = n, r.bps
		}
	}
	if r, ok := res[prev]; ok && prev != "" && !r.failed && float64(r.bps) >= keepWithin*float64(out.Bps) {
		out.Node, out.Bps = prev, r.bps
	}
	return out
}

// pickAlive says whether node, picked by a download test, is still usable
// on url: a node whose last test of it failed, or was refused, is not. One
// not tested yet (the app has just started) is.
func pickAlive(px map[string]mihomo.Proxy, node, url string, stale time.Duration, now time.Time) bool {
	n, ok := px[node]
	if !ok {
		return true // not read yet; one gone from its group is told by the group
	}
	h, tested := speed.Last(n.HistoryOf(url), stale, now)
	return !tested || h.Delay > 0
}

// downloadWant is the node route r's selector should be on for its
// download test's pick, "" when it should be where its strategy otherwise
// says (wanted). A pick that has died is forgotten.
func (a *App) downloadWant(r routes.Route, s settings.Settings, px map[string]mihomo.Proxy) string {
	if !r.Policy.ByDownload() || r.Direct {
		return ""
	}
	a.mu.Lock()
	p := a.picks[r.ID]
	a.mu.Unlock()
	if p.Node == "" {
		return ""
	}
	members := px[routes.Group(r)].All
	if !slices.ContainsFunc(members, func(m string) bool { return providerOf(m) != "" }) {
		return "" // no subscription's nodes in it yet (just started): not a sign the pick is gone
	}
	if !slices.Contains(members, p.Node) || !a.pickInRange(r, p.Node) ||
		!pickAlive(px, p.Node, s.TargetsOf(r.ID)[0].URL, speed.StaleAfter(s.HealthInterval, s.URLTestInterval), time.Now()) {
		a.dropDownloadPick(r.ID)
		return ""
	}
	return p.Node
}

// pickInRange says whether node is in the range of route r's automatic
// choice, which may have changed since the test.
func (a *App) pickInRange(r routes.Route, node string) bool {
	q := r.Policy
	if !q.Scoped() {
		return true
	}
	country := ""
	if pl := a.loc.Guess(node, display(node)).Place; pl != nil {
		country = pl.Country
	}
	return inScope(q, node, providerOf(node), country)
}

// followDownloads puts the selectors of the groups that choose by download
// speed where their picks say, from px, the core's groups and nodes as just
// read: on the pick while it answers, else on the automatic group.
func (a *App) followDownloads(px map[string]mihomo.Proxy) {
	if a.importedNow() != nil {
		return
	}
	s := a.settings.Get()
	var moves [][2]string
	for _, r := range a.routes() {
		if !r.Policy.ByDownload() || r.Direct || !r.NodeGroups() {
			continue
		}
		g, ok := px[routes.Group(r)]
		if !ok {
			continue
		}
		want := a.downloadWant(r, s, px)
		if want == "" {
			want = routes.Auto(r)
		}
		if g.Now != want && slices.Contains(g.All, want) {
			moves = append(moves, [2]string{routes.Group(r), want})
		}
	}
	if len(moves) == 0 {
		return
	}
	logPolicyErr(a.coreCall(5*time.Second, func(ctx context.Context, c *mihomo.Client) error {
		var first error
		for _, m := range moves {
			if err := c.Select(ctx, m[0], m[1]); err != nil && first == nil {
				first = err
			}
		}
		return first
	}))
}
