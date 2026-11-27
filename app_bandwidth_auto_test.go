package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/locate"
	"github.com/oboo/moho/internal/mihomo"
	"github.com/oboo/moho/internal/provider"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

// The fastest download is chosen; the node chosen before stays while it
// came within a fifth of it, so that two about as fast don't take turns;
// nothing is chosen when every download failed.
func TestChoosePick(t *testing.T) {
	now := time.Now()
	res := map[string]bandwidthResult{
		"a": {bps: 50e6}, "b": {bps: 90e6}, "c": {bps: 80e6}, "d": {failed: true},
	}
	if p := choosePick(res, "", now); p.Node != "b" || p.Bps != 90e6 || !p.At.Equal(now) {
		t.Errorf("no previous pick: %+v", p)
	}
	if p := choosePick(res, "c", now); p.Node != "c" {
		t.Errorf("c at 80 of 90 should stay: %+v", p)
	}
	if p := choosePick(res, "a", now); p.Node != "b" {
		t.Errorf("a at 50 of 90 should give way: %+v", p)
	}
	if p := choosePick(res, "d", now); p.Node != "b" {
		t.Errorf("a previous pick that failed stays: %+v", p)
	}
	if p := choosePick(map[string]bandwidthResult{"x": {failed: true}}, "x", now); p.Node != "" {
		t.Errorf("all failed: %+v", p)
	}
}

func TestDownloadDue(t *testing.T) {
	now := time.Now()
	by := routes.Route{ID: "media", Policy: routes.Policy{Mode: routes.ModeAuto, DownloadEvery: 12}}
	if !downloadDue(by, downloadPick{}, false, time.Time{}, now) {
		t.Error("never tested: due")
	}
	if downloadDue(by, downloadPick{At: now.Add(-11 * time.Hour)}, true, time.Time{}, now) {
		t.Error("11 h of 12: not due")
	}
	if !downloadDue(by, downloadPick{At: now.Add(-13 * time.Hour)}, true, time.Time{}, now) {
		t.Error("13 h of 12: due")
	}
	if !downloadDue(by, downloadPick{At: now.Add(time.Hour)}, true, time.Time{}, now) {
		t.Error("a test in the future (the clock moved back): due")
	}
	if downloadDue(by, downloadPick{}, false, now.Add(-time.Minute), now) {
		t.Error("tried a minute ago: wait")
	}
	off := by
	off.Policy.DownloadEvery = 0
	if downloadDue(off, downloadPick{}, false, time.Time{}, now) {
		t.Error("by delay: never due")
	}
	fixed := by
	fixed.Policy.Mode = routes.ModeFixed
	if downloadDue(fixed, downloadPick{}, false, time.Time{}, now) {
		t.Error("a fixed node keeps its setting but is not tested")
	}
	direct := by
	direct.Direct = true
	if downloadDue(direct, downloadPick{}, false, time.Time{}, now) {
		t.Error("direct: never due")
	}
}

// A pick that failed its last test, or was refused, is not used; one not
// tested yet, or not read yet, is.
func TestPickAlive(t *testing.T) {
	now, url := time.Now(), "https://example.com/"
	px := map[string]mihomo.Proxy{
		"up":   {Extra: map[string]mihomo.URLTests{url: {History: []mihomo.History{{Time: now, Delay: 120}}}}},
		"down": {Extra: map[string]mihomo.URLTests{url: {History: []mihomo.History{{Time: now, Delay: 120}, {Time: now, Delay: 0}}}}},
		"new":  {},
	}
	for node, want := range map[string]bool{"up": true, "down": false, "new": true, "unread": true} {
		if got := pickAlive(px, node, url, time.Hour, now); got != want {
			t.Errorf("%s: %v, want %v", node, got, want)
		}
	}
}

// Which node a group should be on comes from its pick while that is in the
// group, in range and answering; a pick that is not is forgotten.
func TestDownloadWant(t *testing.T) {
	dir := t.TempDir()
	st, err := settings.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := provider.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := &App{settings: st, providers: ps, dataDir: dir, loc: locate.New(filepath.Join(dir, "loc.json"))}
	name := func(s string) string { return s + core.NodeSep + "p" }
	if _, err := st.Update(func(s *settings.Settings) {
		s.Routing.ChangePolicy("media", func(q *routes.Policy) { q.Mode, q.DownloadEvery = routes.ModeAuto, 12 })
	}); err != nil {
		t.Fatal(err)
	}
	r := a.route("media")
	if !r.Policy.ByDownload() {
		t.Fatalf("media policy %+v", r.Policy)
	}
	s := st.Get()
	main := s.TargetsOf("media")[0].URL
	tested := func(ms int) mihomo.Proxy {
		return mihomo.Proxy{Extra: map[string]mihomo.URLTests{main: {History: []mihomo.History{{Time: time.Now(), Delay: ms}}}}}
	}
	px := map[string]mihomo.Proxy{
		name("fast"): tested(300), name("dead"): tested(0),
		routes.Group(r): {Now: routes.Auto(r), All: []string{routes.Auto(r), name("fast"), name("dead")}},
	}
	a.setDownloadPick("media", downloadPick{Node: name("fast"), Bps: 1, At: time.Now()})
	if got := a.downloadWant(r, s, px); got != name("fast") {
		t.Errorf("want %q, got %q", name("fast"), got)
	}

	// Kept across a restart.
	b := &App{settings: st, dataDir: dir}
	b.loadDownloadPicks()
	if b.picks["media"].Node != name("fast") {
		t.Errorf("not kept: %+v", b.picks)
	}

	// Out of range once the user narrows it: forgotten.
	r.Policy.Nodes = []string{name("dead")}
	if got := a.downloadWant(r, s, px); got != "" || a.picks["media"].Node != "" {
		t.Errorf("out of range: %q, pick %+v", got, a.picks["media"])
	}
	if a.picks["media"].At.IsZero() {
		t.Error("forgetting the node forgot when the test ran: the next would run at once")
	}
	r.Policy.Nodes = nil

	a.setDownloadPick("media", downloadPick{Node: name("dead"), At: time.Now()})
	if got := a.downloadWant(r, s, px); got != "" {
		t.Errorf("dead pick used: %q", got)
	}

	// Before the subscription's nodes are in the group, the pick waits.
	a.setDownloadPick("media", downloadPick{Node: name("fast"), At: time.Now()})
	empty := map[string]mihomo.Proxy{routes.Group(r): {Now: routes.Auto(r), All: []string{routes.Auto(r)}}}
	if got := a.downloadWant(r, s, empty); got != "" || a.picks["media"].Node != name("fast") {
		t.Errorf("no nodes yet: %q, pick %+v", got, a.picks["media"])
	}

	a.setDownloadPick("media", downloadPick{Node: name("gone"), At: time.Now()})
	if got := a.downloadWant(r, s, px); got != "" {
		t.Errorf("a pick no longer in the group used: %q", got)
	}

	// A group choosing by delay ignores a pick.
	a.setDownloadPick("media", downloadPick{Node: name("fast"), At: time.Now()})
	r.Policy.DownloadEvery = 0
	if got := a.downloadWant(r, s, px); got != "" {
		t.Errorf("by delay: %q", got)
	}
}

// A scheduled test takes the range of the group's automatic choice alone.
func TestPlanBandwidthInRange(t *testing.T) {
	dir := t.TempDir()
	st, err := settings.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := provider.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := &App{settings: st, providers: ps, loc: locate.New(filepath.Join(dir, "loc.json"))}
	name := func(i int) string { return fmt.Sprintf("n%d%sp", i, core.NodeSep) }
	if _, err := st.Update(func(s *settings.Settings) {
		s.Routing.ChangePolicy(routes.Default, func(q *routes.Policy) { q.Nodes = []string{name(1), name(3)} })
	}); err != nil {
		t.Fatal(err)
	}
	r := a.route(routes.Default)
	main := st.Get().TargetsOf(routes.Default)[0].URL
	px := map[string]mihomo.Proxy{}
	members := []string{routes.Auto(r)}
	for i := range 4 {
		px[name(i)] = mihomo.Proxy{Extra: map[string]mihomo.URLTests{main: {History: []mihomo.History{{Time: time.Now(), Delay: 100 + i}}}}}
		members = append(members, name(i))
	}
	px[routes.Group(r)] = mihomo.Proxy{Now: name(0), All: members}
	a.proxies = px
	if got := a.planRound(routes.Default).Nodes; !slices.Equal(got, []string{name(1), name(3)}) {
		t.Errorf("in range: %v", got)
	}
	all := []string{name(0), name(1), name(2), name(3)}
	if p := a.planBandwidth(routes.Default, all); len(p.Nodes) != 4 || p.Round {
		t.Errorf("by hand, choosing by delay, every node listed: %+v", p)
	}

	// Choosing by download speed, the test by hand is a round run now: of
	// the range alone, at a round's cap, and of the rows listed among it.
	if _, err := st.Update(func(s *settings.Settings) {
		s.Routing.ChangePolicy(routes.Default, func(q *routes.Policy) { q.Mode, q.DownloadEvery = routes.ModeAuto, 12 })
	}); err != nil {
		t.Fatal(err)
	}
	p := a.planBandwidth(routes.Default, all)
	if !p.Round || p.Cap != scheduledCap || !slices.Equal(p.Nodes, []string{name(1), name(3)}) {
		t.Errorf("a round by hand: %+v", p)
	}
	if got := a.planBandwidth(routes.Default, []string{name(3)}).Nodes; !slices.Equal(got, []string{name(3)}) {
		t.Errorf("a round of the rows searched for: %v", got)
	}
}

// With takeover off, a round that is due still starts: apps pointed at the
// core's port go by the groups all the same (issue 267).
func TestDueDownloadsWithTakeoverOff(t *testing.T) {
	st, err := settings.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ps, err := provider.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Update(func(s *settings.Settings) {
		s.Paused, s.SystemProxy, s.Tun = true, false, false
		s.Routing.ChangePolicy(routes.Default, func(q *routes.Policy) { q.Mode, q.DownloadEvery = routes.ModeAuto, 12 })
	}); err != nil {
		t.Fatal(err)
	}
	a := &App{settings: st, providers: ps}
	a.runDueDownloads()
	// The round ends at once (no node has answered a delay test, so there
	// is nothing to download through), noted as tried.
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		a.mu.Lock()
		busy, tried := a.downloading, a.downloadTried[routes.Default]
		a.mu.Unlock()
		if !busy {
			if tried.IsZero() {
				t.Error("no round with takeover off")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the round never ended")
		}
	}
}

// Choosing by download speed is a selector's position, like the others:
// turning it on or changing how often reloads nothing.
func TestDownloadEveryLeavesTheCore(t *testing.T) {
	var a, b routes.Plan
	a.ChangePolicy("media", func(q *routes.Policy) { q.Mode = routes.ModeAuto })
	b.ChangePolicy("media", func(q *routes.Policy) { q.Mode, q.DownloadEvery = routes.ModeAuto, 6 })
	if !routingCoreSame(a, b) {
		t.Error("DownloadEvery changed the core's config")
	}
}
