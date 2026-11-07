package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/settings"
)

// TestBordersTwisted: with twisted borders the board draws its lines, and
// the focused cell's wall twists in and, when the focus goes, out.
func TestBordersTwisted(t *testing.T) {
	u := fixtureUI()
	u.snap.Settings = settings.Defaults.Clone()
	u.snap.Settings.Borders = "twisted"
	tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
	tt.Advance(time.Second)
	if len(u.home.lines) == 0 {
		t.Fatal("no lines between the cells")
	}
	if len(u.home.walls) != 0 {
		t.Fatalf("walls with no cell focused: %v", u.home.walls)
	}
	u.home.x = "mode"
	tt.Frame()
	tt.Advance(2 * time.Second)
	if u.home.walls["mode"] == nil {
		t.Fatal("no wall round the focused cell")
	}
	u.home.x = ""
	tt.Frame()
	tt.Advance(2 * time.Second)
	if len(u.home.walls) != 0 {
		t.Errorf("walls left after the focus went: %v", u.home.walls)
	}
	// Straight, as by default: none of it.
	u.snap.Settings.Borders = "straight"
	u.home.x = "mode"
	tt.Frame()
	tt.Advance(2 * time.Second)
	if u.home.lines != nil || u.home.walls != nil {
		t.Errorf("straight borders keep twisted ones: %v %v", u.home.lines, u.home.walls)
	}
}

// TestWobblyFrame: the wall keeps within its swing of the edge, its
// corners' middles cut it into four sides in order, and the paths built
// from it close round the cell.
func TestWobblyFrame(t *testing.T) {
	r := ui.Rect{X: 10, Y: 20, W: 300, H: 200}
	pts, cuts := wobblyFrame(r, wallRound, wallSwing, wallWave, 0)
	if len(pts) == 0 {
		t.Fatal("no wall")
	}
	const eps = .01
	for _, q := range pts {
		if q[0] < r.X-wallSwing-eps || q[0] > r.X+r.W+wallSwing+eps || q[1] < r.Y-wallSwing-eps || q[1] > r.Y+r.H+wallSwing+eps {
			t.Fatalf("point %v strays from %+v", q, r)
		}
	}
	for i := 1; i < 4; i++ {
		if cuts[i] <= cuts[i-1] {
			t.Errorf("cuts out of order: %v", cuts)
		}
	}
	// The top right corner's middle is up and to the right of the cell's middle.
	if q := pts[cuts[0]]; q[0] < r.X+r.W-wallRound || q[1] > r.Y+wallRound {
		t.Errorf("top right cut at %v", q)
	}
	var wp wallPaths
	wp.build(r, r, wallSwing, 0)
	for i := range 4 {
		if b := wp.wall[i].Bounds(); b.W <= 0 && b.H <= 0 {
			t.Errorf("side %d has no line", i)
		}
		// A side's strip is thin across its edge.
		ob := wp.out[i].Bounds()
		if thin := min(ob.W, ob.H); thin > wallRound+lineReach+wallSwing+1 {
			t.Errorf("side %d's paper is %+v, not a strip", i, ob)
		}
	}
	if !wp.out[0].Contains(r.X+r.W/2, r.Y-lineReach+.5) || wp.out[0].Contains(r.X+r.W/2, r.Y+wallSwing+1) {
		t.Error("the paper above the top is not outside the line")
	}
	// Flat, nothing bulges past the edge.
	wp.build(r, r, 0, 0)
	for i := range 4 {
		if b := wp.in[i].Bounds(); b.W > 0 && b.H > 0 {
			t.Errorf("side %d bulges flat: %+v", i, b)
		}
	}
}

func TestSettingsBorders(t *testing.T) {
	s := settings.Defaults.Clone()
	if s.Borders != "straight" {
		t.Errorf("default borders %q, want straight", s.Borders)
	}
	for _, b := range []string{"", "straight", "twisted"} {
		s.Borders = b
		if err := s.Validate(); err != nil {
			t.Errorf("borders %q: %v", b, err)
		}
	}
	s.Borders = "wiggly"
	if s.Validate() == nil {
		t.Error("unknown borders pass")
	}
}
