package speed

import (
	"slices"
	"testing"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/mihomo"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

// Only a route whose selector is on a node is kept tested by bowel: the
// automatic and the ordered groups are the core's to test.
func TestFixed(t *testing.T) {
	s := settings.Defaults.Clone()
	all := routes.Build(s.Routing, nil)
	node := "B" + core.NodeSep + "p1"
	px := map[string]mihomo.Proxy{}
	for _, r := range all {
		switch {
		case !r.NodeGroups():
		case r.ID == "ai":
			px[routes.Group(r)] = mihomo.Proxy{Now: node}
		case r.ID == "media":
			px[routes.Group(r)] = mihomo.Proxy{Now: routes.Backup(r)}
		case r.ID == "dev":
			px[routes.Group(r)] = mihomo.Proxy{Now: "DIRECT"}
		default:
			px[routes.Group(r)] = mihomo.Proxy{Now: routes.Auto(r)}
		}
	}
	got := Fixed(px, all, s)
	want := []Target{{Node: node, URL: s.TargetsOf("ai")[0].URL}}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if got := Fixed(map[string]mihomo.Proxy{}, all, s); len(got) != 0 {
		t.Fatalf("no groups yet: %+v", got)
	}
}

// 全部测速 is a request through each node the route can use, not through
// its own helper groups.
func TestOfRouteLeavesOutItsOwnGroups(t *testing.T) {
	r := routes.Route{ID: "ai", Name: "AI 服务"}
	a, b := "A"+core.NodeSep+"p1", "B"+core.NodeSep+"p1"
	px := map[string]mihomo.Proxy{routes.Group(r): {All: []string{routes.Auto(r), routes.Backup(r), a, b, a}}}
	got := OfRoute(px, r, "https://u")
	want := []Target{{Node: a, URL: "https://u"}, {Node: b, URL: "https://u"}}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
