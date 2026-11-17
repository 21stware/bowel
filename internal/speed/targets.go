package speed

import (
	"slices"
	"strings"
	"time"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/mihomo"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

// Tested is when p was last tested on url by anyone, as the core has it.
func Tested(p mihomo.Proxy, url string) time.Time {
	h := p.HistoryOf(url)
	if len(h) == 0 {
		return time.Time{}
	}
	return h[len(h)-1].Time
}

// Fixed are the nodes the routes are fixed on, each with its route's main
// target: what no group of the core chooses, and so none keeps tested.
func Fixed(px map[string]mihomo.Proxy, all []routes.Route, s settings.Settings) []Target {
	var out []Target
	for _, r := range all {
		if !r.NodeGroups() {
			continue
		}
		if n := px[routes.Group(r)].Now; strings.Contains(n, core.NodeSep) && !routes.Internal(r, n) {
			out = append(out, Target{Node: n, URL: s.TargetsOf(r.ID)[0].URL})
		}
	}
	return out
}

// OfRoute are the nodes route r can use, each with url: its group's members
// but for the route's own helper groups.
func OfRoute(px map[string]mihomo.Proxy, r routes.Route, url string) []Target {
	var out []Target
	for _, m := range px[routes.Group(r)].All {
		if t := (Target{Node: m, URL: url}); !routes.Internal(r, m) && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}
