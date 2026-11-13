package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// The routes' names on the board stay clear of the nodes beside them: the
// names' column is as wide as the longest name, up to a cap past which a
// name ends in "…". A fixed column let longer names run into the nodes.
func TestRouteNamesClearOfNodes(t *testing.T) {
	u := fixtureUI()
	long := map[string]string{"ai": "JP-Osaka-Relay-AnyTLS", "media": "Global-Media", "dev": "Duolingo"}
	for i, r := range u.snap.Routes {
		if n, ok := long[r.ID]; ok {
			u.snap.Routes[i].Name = n
		}
	}
	const cellW = 340
	tt := ui.NewTester(func(c *ui.Context) {
		p := windowPal(c, palette(c))
		ui.Column(c).Width(cellW).Children(func() { u.routesNormal(c, p, cellW) })
	}, cellW, 300)
	tt.Advance(time.Second)
	shot(t, tt, "routes-cell-long-names")
	capW := float32((cellW - 32) * routeNameShare) // the cell's padding is 16

	// What each name needs on one line, laid out on its own.
	natural := func(name string) float32 {
		m := ui.NewTester(func(c *ui.Context) {
			ui.Row(c).Children(func() { ui.Text(c, name).FontSize(12.5).FontWeight(600).NoWrap() })
		}, 600, 40)
		r, _ := m.Find(name)
		return r.W
	}
	var col float32
	for _, r := range u.snap.Routes {
		if r.ID == "default" || r.Reject {
			continue
		}
		box, ok := tt.Find(r.Name)
		if !ok {
			t.Fatalf("no %q in the cell: %q", r.Name, tt.Texts())
		}
		if col == 0 {
			col = box.W
		} else if box.W != col {
			t.Errorf("%q is %v wide, the others %v: the nodes no longer line up", r.Name, box.W, col)
		}
		node := "直接连接"
		if n, ok := u.node(r.Node); ok && !r.Direct {
			node = n.Display
		}
		nb, ok := tt.Find(node)
		if !ok {
			t.Fatalf("no node %q in the cell", node)
		}
		if box.X+box.W > nb.X {
			t.Errorf("%q ends at %v, past %q at %v", r.Name, box.X+box.W, node, nb.X)
		}
		if need := natural(r.Name); need > box.W && box.W < capW-1 {
			t.Errorf("%q needs %v but has %v, under the cap", r.Name, need, box.W)
		}
	}
	if need := natural("Global-Media"); col < need {
		t.Errorf("the column is %v, short of Global-Media's %v", col, need)
	}
	if col > capW {
		t.Errorf("the column is %v, past its cap of %v", col, capW)
	}

	// A name that ends in "…" shows whole on hover; one that fits does not.
	count := func(s string) (n int) {
		for _, x := range tt.Texts() {
			if x == s {
				n++
			}
		}
		return n
	}
	for name, tip := range map[string]bool{"JP-Osaka-Relay-AnyTLS": true, "Global-Media": false} {
		box, _ := tt.Find(name)
		tt.Move(box.X+box.W/2, box.Y+box.H/2)
		tt.Advance(2 * time.Second)
		if got := count(name) == 2; got != tip {
			t.Errorf("hovering %q: tooltip %v, want %v (%q)", name, got, tip, tt.Texts())
		}
		tt.Move(0, 290)
		tt.Advance(2 * time.Second)
	}
}

func TestRouteNameColumn(t *testing.T) {
	for _, c := range []struct {
		widths      []float32
		avail, want float32
	}{
		{[]float32{20, 30}, 300, routeNameMin}, // short names keep the old column
		{[]float32{20, 90.2}, 300, 91},         // the longest, whole pixels
		{[]float32{20, 400}, 300, 105},         // capped at the share
		{[]float32{400}, 100, routeNameMin},    // a narrow cell keeps the old column
		{nil, 300, routeNameMin},               // no routes
	} {
		if got := routeNameColumn(c.widths, c.avail); got != c.want {
			t.Errorf("routeNameColumn(%v, %v) = %v, want %v", c.widths, c.avail, got, c.want)
		}
	}
}
