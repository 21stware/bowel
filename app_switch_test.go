package main

import (
	"context"
	"testing"
	"time"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/mihomo"
	"github.com/oboo/moho/internal/provider"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

// switchFixture is a pair of providers: one with several nodes, one that is a
// whole pasted link, and the names the core gives their nodes.
func switchFixture(t *testing.T) (*App, string, string, string) {
	t.Helper()
	ps, err := provider.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st, err := settings.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	solo, err := ps.Add(ctx, nil, "trojan://pw@hk.example.com:443#HK01")
	if err != nil {
		t.Fatal(err)
	}
	pair, err := ps.Add(ctx, nil, "trojan://pw@a.example.com:443#A-IPLC\ntrojan://pw@b.example.com:443#B-02")
	if err != nil {
		t.Fatal(err)
	}
	if solo, err = ps.Rename(solo.ID, "Ohio"); err != nil {
		t.Fatal(err)
	}
	if pair, err = ps.Rename(pair.ID, "某 IPLC 机场"); err != nil {
		t.Fatal(err)
	}
	a := &App{providers: ps, settings: st}
	from, to := "A-IPLC"+core.NodeSep+pair.ID, "B-02"+core.NodeSep+pair.ID
	a.provNodes = map[string][]string{
		solo.ID: {"HK01" + core.NodeSep + solo.ID},
		pair.ID: {from, to},
	}
	return a, from, to, "HK01" + core.NodeSep + solo.ID
}

// A note names a node as "[provider]node" when there is more than one
// provider, and by its own name when there is not.
func TestNodeShown(t *testing.T) {
	a, from, to, soloNode := switchFixture(t)

	if got, want := a.nodeShown(a.provNodes, to), "[某 IPLC 机场]B-02"; got != want {
		t.Errorf("two providers: %q, want %q", got, want)
	}
	// A link provider's only node already goes by the provider's name: it is
	// not said twice.
	if got := a.nodeShown(a.provNodes, soloNode); got != "Ohio" {
		t.Errorf("a one-node link provider: %q", got)
	}
	// DIRECT has no provider, and neither does a name the core gave nothing.
	if got := a.nodeShown(nil, "DIRECT"); got != "DIRECT" {
		t.Errorf("DIRECT = %q", got)
	}

	// With one provider, its name is on every line: the node alone reads.
	one := map[string][]string{"only": {"B-02" + core.NodeSep + "only"}}
	if severalProviders(one) {
		t.Error("one provider, and it counts as several")
	}
	if got := a.nodeShown(one, "B-02"+core.NodeSep+"only"); got != "B-02" {
		t.Errorf("one provider: %q, want the node alone", got)
	}

	// A provider that is gone leaves the node's own name, not a bare id.
	if err := a.providers.Delete(providerOf(from)); err != nil {
		t.Fatal(err)
	}
	if got := a.nodeShown(a.provNodes, from); got != "A-IPLC" {
		t.Errorf("provider gone: %q, want A-IPLC", got)
	}
}

func TestSwitchExpiry(t *testing.T) {
	n := &switchNote{at: time.Now()}
	if switchExpired(n, n.at.Add(switchShow-time.Millisecond)) {
		t.Error("expired a moment too early")
	}
	if !switchExpired(n, n.at.Add(switchShow)) {
		t.Error("not expired at the mark")
	}
	if switchShow != time.Minute {
		t.Errorf("the note shows for %v, want a minute", switchShow)
	}
}

// A note stops telling what happens once the route moved on, was pinned by
// hand, or the node it left answers again.
func TestSwitchStale(t *testing.T) {
	a, from, to, _ := switchFixture(t)
	r := a.route(routes.Default)
	if r.ID == "" {
		t.Fatal("no default route")
	}
	url := a.settings.Get().TargetsOf(r.ID)[0].URL
	px := func(pinned, moved bool, fromDelay int) map[string]mihomo.Proxy {
		m := map[string]mihomo.Proxy{
			routes.Group(r): {Now: routes.Auto(r)},
			routes.Auto(r):  {Now: to},
			to:              {Name: to},
			from: {Name: from, Extra: map[string]mihomo.URLTests{
				url: {History: []mihomo.History{{Delay: fromDelay}}},
			}},
		}
		if pinned {
			m[routes.Group(r)] = mihomo.Proxy{Now: "pinned"}
		}
		if moved {
			m[routes.Auto(r)] = mihomo.Proxy{Now: "elsewhere"}
		}
		return m
	}
	n := &switchNote{route: r.ID, from: from, to: to, at: time.Now()}
	if a.switchStale(n, px(false, false, 0), a.settings.Get()) {
		t.Error("still telling, but called stale")
	}
	if !a.switchStale(n, px(true, false, 0), a.settings.Get()) {
		t.Error("the group was pinned by hand, and the note stayed")
	}
	if !a.switchStale(n, px(false, true, 0), a.settings.Get()) {
		t.Error("the group moved again, and the note stayed")
	}
	if !a.switchStale(n, px(false, false, 120), a.settings.Get()) {
		t.Error("the node it left answers again, and the note stayed")
	}
	if !a.switchStale(&switchNote{route: "no-such-route", from: from, to: to, at: time.Now()}, px(false, false, 0), a.settings.Get()) {
		t.Error("a route that is gone, and the note stayed")
	}
}

// The note is rendered with the shown names, and dropped — not just hidden —
// when its time is up or traffic is not being taken over.
func TestSwitchForRendersAndClears(t *testing.T) {
	a, from, to, _ := switchFixture(t)
	r := a.route(routes.Default)
	url := a.settings.Get().TargetsOf(r.ID)[0].URL
	px := map[string]mihomo.Proxy{
		routes.Group(r): {Now: routes.Auto(r)},
		routes.Auto(r):  {Now: to},
		to:              {Name: to},
		from: {Name: from, Extra: map[string]mihomo.URLTests{
			url: {History: []mihomo.History{{Delay: 0}}},
		}},
	}
	running := core.Status{State: core.Running}
	s := a.settings.Get()

	a.switched = &switchNote{route: r.ID, from: from, to: to, at: time.Now()}
	say := a.switchFor(a.switched, px, a.provNodes, running, s, true)
	if say == nil {
		t.Fatal("the switch is not said")
	}
	if say.from != "[某 IPLC 机场]A-IPLC" || say.to != "[某 IPLC 机场]B-02" {
		t.Errorf("names: %q → %q", say.from, say.to)
	}
	if say.title != routes.Title(r) {
		t.Errorf("title = %q, want %q", say.title, routes.Title(r))
	}

	a.switched = &switchNote{route: r.ID, from: from, to: to, at: time.Now().Add(-switchShow)}
	if say := a.switchFor(a.switched, px, a.provNodes, running, s, true); say != nil {
		t.Errorf("said after its time: %+v", say)
	}
	if a.switched != nil {
		t.Error("the note was kept after its time")
	}

	a.switched = &switchNote{route: r.ID, from: from, to: to, at: time.Now()}
	if say := a.switchFor(a.switched, px, a.provNodes, running, s, false); say != nil {
		t.Errorf("said with traffic not taken over: %+v", say)
	}
	if a.switched != nil {
		t.Error("the note was kept with traffic not taken over")
	}

	// A core that is not running is not switching anything.
	a.switched = &switchNote{route: r.ID, from: from, to: to, at: time.Now()}
	if say := a.switchFor(a.switched, px, a.provNodes, core.Status{State: core.Stopped}, s, true); say != nil {
		t.Errorf("said with the core stopped: %+v", say)
	}
	if a.switched != nil {
		t.Error("the note was kept with the core stopped")
	}

	// The note handed in is the one judged: a snapshot judges it against the
	// nodes it copied, and a newer note recorded meanwhile is left alone.
	older := &switchNote{route: r.ID, from: from, to: to, at: time.Now().Add(-switchShow)}
	a.switched = &switchNote{route: r.ID, from: from, to: to, at: time.Now()}
	if say := a.switchFor(older, px, a.provNodes, running, s, true); say != nil {
		t.Errorf("an expired note was said: %+v", say)
	}
	if a.switched == nil {
		t.Error("a newer note was dropped for an older one")
	}
}
