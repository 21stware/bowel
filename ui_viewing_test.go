package main

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/speed"
)

// viewingUI is the fixture with many nodes, every route with a main target
// of its own, and one other site on the AI route.
func viewingUI(nodes int) *UI {
	u := fixtureUI()
	first := u.snap.Nodes[0]
	u.snap.Nodes = nil
	for i := range nodes {
		n := first
		n.Name, n.Display = fmt.Sprintf("n%03d​‹a", i), fmt.Sprintf("node %03d", i)
		n.Routes = map[string]NodeTest{"ai": {Delay: 700 + i, Tested: true}}
		u.snap.Nodes = append(u.snap.Nodes, n)
	}
	for i, r := range u.snap.Routes {
		if r.Direct {
			continue
		}
		r.Main = routes.Target{Name: r.ID, URL: "https://main/" + r.ID}
		r.Node = u.snap.Nodes[min(i, nodes-1)].Name
		if r.ID == "ai" {
			r.Sites = []SiteDelay{{Name: "Site", URL: "https://site"}}
		}
		u.snap.Routes[i] = r
	}
	u.snap.Exit = &Exit{Node: u.snap.Nodes[0].Name}
	return u
}

// on is how many of the tests a window drew are on url.
func on(v viewing, url string) (n int) {
	for _, t := range v.tests {
		if t.URL == url {
			n++
		}
	}
	return n
}

// The picker says which rows it draws: the ones in view, not the 300 the
// route can use. Each is a delay on the route's main target and a ping, and
// the route's other sites are timed through the node it uses.
func TestPickerMarksOnlyTheRowsItDraws(t *testing.T) {
	u := viewingUI(300)
	u.home.x, u.home.sub = "routes", "ai"
	tt := ui.NewTester(u.view, 1060, 1200)
	tt.Advance(2 * time.Second)
	v := u.seen["window"]
	rows := len(v.pings) // only the picker shows pings here
	if rows < 5 || rows > 40 {
		t.Errorf("%d rows of 300 marked, want the ones in view: %v", rows, v.pings)
	}
	for _, n := range v.pings {
		if !slices.Contains(v.tests, speed.Target{Node: n, URL: "https://main/ai"}) {
			t.Errorf("the row of %s is not marked on the route's main target", n)
		}
	}
	if site := (speed.Target{Node: u.route("ai").Node, URL: "https://site"}); !slices.Contains(v.tests, site) {
		t.Errorf("the route's site through its node is not marked: %+v", v.tests)
	}
	// Scrolled, it is other rows.
	before := slices.Clone(v.pings)
	tt.Scroll(700, 1000, 0, 4000)
	tt.Advance(time.Second)
	v = u.seen["window"]
	if n := len(v.pings); n < 5 || n > 40 || slices.Contains(v.pings, before[0]) {
		t.Errorf("after scrolling: %d rows marked, the first of before among them: %v", n, v.pings)
	}
	// Another page draws none of them.
	u.page = "activity"
	tt.Frame()
	tt.Advance(time.Second)
	if v = u.seen["window"]; len(v.tests) != 0 || len(v.pings) != 0 {
		t.Errorf("the log page marks %+v", v)
	}
}

// The board marks what its cells show: each route's node on that route's
// own main target.
func TestBoardMarksEachRoutesNode(t *testing.T) {
	u := viewingUI(8)
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(2 * time.Second)
	v := u.seen["window"]
	for _, r := range u.snap.Routes {
		if want := (speed.Target{Node: r.Node, URL: r.Main.URL}); !r.Direct && !slices.Contains(v.tests, want) {
			t.Errorf("%s: %+v is not marked in %+v", r.ID, want, v.tests)
		}
	}
	if len(v.pings) != 0 {
		t.Errorf("the board shows no ping, and marks %v", v.pings)
	}
}

// The list of routes shows each route's node as that route tests it.
func TestRouteListShowsEachRoutesOwnDelay(t *testing.T) {
	u := viewingUI(8)
	u.home.x = "routes"
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(2 * time.Second)
	n, _ := u.node(u.route("ai").Node)
	if want := fmtDelay(n.On("ai")); !tt.HasText(want) {
		t.Errorf("no %q in %q", want, tt.Texts())
	}
}

// The menu bar panel marks the few nodes it lists.
func TestPanelMarksItsRows(t *testing.T) {
	u := viewingUI(30)
	tt := ui.NewTester(u.panelView, panelWidth, 700)
	tt.Advance(time.Second)
	v := u.seen["panel"]
	if n := on(v, "https://main/default"); n != panelNodes || len(v.tests) != n {
		t.Errorf("the panel marks %+v, want its %d rows on the default route's target", v.tests, panelNodes)
	}
}

// A provider's nodes are all laid out, one under the other: the ones the
// window has no room for are not on screen, and are not marked.
func TestProviderPageMarksOnlyTheRowsInView(t *testing.T) {
	u := viewingUI(300)
	u.page, u.prov.open = "providers", "a"
	tt := ui.NewTester(u.view, 1060, 760)
	next := func() viewing { // the frame after says what the last one showed
		tt.Frame()
		return u.seen["window"]
	}
	v := next()
	if n := len(v.pings); n < 5 || n > 40 || on(v, "https://main/default") != n {
		t.Fatalf("%d pings and %d delays of 300 rows marked, want the ones in view", n, on(v, "https://main/default"))
	}
	before := slices.Clone(v.pings)
	tt.Scroll(500, 400, 0, 4000)
	if v = next(); len(v.pings) < 5 || len(v.pings) > 40 || slices.Contains(v.pings, before[0]) {
		t.Errorf("after scrolling: %d rows marked, the first of before among them: %v", len(v.pings), v.pings)
	}
}

// The provider page marks the rows of the provider that is open.
func TestProviderPageMarksItsNodes(t *testing.T) {
	u := viewingUI(8)
	u.page, u.prov.open = "providers", "a"
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Frame() // the rows have their places now
	v := u.seen["window"]
	if n := on(v, "https://main/default"); n != 8 || len(v.pings) != n {
		t.Errorf("the provider page marks %d delays and %d pings: %+v", n, len(v.pings), v)
	}
}
