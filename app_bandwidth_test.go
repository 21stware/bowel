package main

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/mihomo"
	"github.com/oboo/moho/internal/provider"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
	"github.com/oboo/moho/internal/speed"
)

// A download test takes the nodes that answered fastest on the route's main
// target, a few of them, and the node in use whatever its delay; never a
// node that failed or one the route cannot use.
func TestPlanBandwidth(t *testing.T) {
	st, err := settings.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ps, err := provider.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := &App{settings: st, providers: ps}
	r := a.route(routes.Default)
	main := st.Get().TargetsOf(routes.Default)[0].URL
	now := time.Now()
	tested := func(ms int) mihomo.Proxy {
		return mihomo.Proxy{Extra: map[string]mihomo.URLTests{main: {History: []mihomo.History{{Time: now, Delay: ms}}}}}
	}
	px := map[string]mihomo.Proxy{}
	var members []string
	name := func(i int) string { return fmt.Sprintf("n%d%sp", i, core.NodeSep) }
	for i := range 8 {
		px[name(i)] = tested(100 + 10*(7-i)) // n7 the fastest, n0 the slowest
		members = append(members, name(i))
	}
	px["dead"+core.NodeSep+"p"] = tested(0)
	members = append(members, "dead"+core.NodeSep+"p", routes.Auto(r))
	px[routes.Group(r)] = mihomo.Proxy{Now: name(0), All: members}
	a.proxies = px

	plan := a.planBandwidth(routes.Default)
	want := []string{name(7), name(6), name(5), name(4), name(3), name(0)}
	if !slices.Equal(plan.Nodes, want) {
		t.Errorf("nodes %v, want %v", plan.Nodes, want)
	}
	if plan.MaxBytes != int64(len(want))*speed.DownloadCap {
		t.Errorf("max bytes %d", plan.MaxBytes)
	}
	if plan.URL != settings.Defaults.DownloadURL {
		t.Errorf("url %q", plan.URL)
	}

	// A route that has no nodes plans nothing.
	if p := a.planBandwidth("no-such-route"); len(p.Nodes) != 0 {
		t.Errorf("unknown route: %v", p.Nodes)
	}
}

func TestDownloadsAlike(t *testing.T) {
	now := time.Now()
	nodes := func(mbps ...float64) []NodeView {
		var out []NodeView
		for _, m := range mbps {
			out = append(out, NodeView{Download: &DownloadTest{Mbps: m, At: now}})
		}
		return out
	}
	if !downloadsAlike(nodes(60, 65, 70)) {
		t.Error("60–70 Mbps read as different")
	}
	if downloadsAlike(nodes(20, 65, 70)) {
		t.Error("20 against 70 read as alike")
	}
	if downloadsAlike(nodes(60, 65)) {
		t.Error("two results are too few to tell")
	}
	old := nodes(60, 65, 70)
	old[0].Download.At = now.Add(-2 * time.Hour)
	if downloadsAlike(old) {
		t.Error("an old result counted")
	}
	failed := append(nodes(60, 65, 70), NodeView{Download: &DownloadTest{Failed: true, At: now}})
	if !downloadsAlike(failed) {
		t.Error("a failure counted as a rate")
	}
}

func TestFmtMbps(t *testing.T) {
	for _, c := range []struct {
		t    *DownloadTest
		want string
	}{
		{nil, "—"},
		{&DownloadTest{Mbps: 4.25}, "4.2 Mbps"},
		{&DownloadTest{Mbps: 64.6}, "65 Mbps"},
		{&DownloadTest{Mbps: 1250}, "1.2 Gbps"},
	} {
		if got := fmtMbps(c.t); got != c.want {
			t.Errorf("%+v: %q, want %q", c.t, got, c.want)
		}
	}
}
