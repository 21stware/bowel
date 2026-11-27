package routes

import (
	"slices"
	"testing"
)

// TestPlaceGroup: a group goes where it is put among those that can move,
// by its place or beside another; the default route stays where it is,
// last in the list, and a place past the end is the last.
func TestPlaceGroup(t *testing.T) {
	var p Plan
	g := p.AddGroup(nil, "Netflix")
	if got, want := p.MovableOrder(), []string{Reject, "ai", "media", "dev", g, "direct"}; !slices.Equal(got, want) {
		t.Fatalf("start: %v, want %v", got, want)
	}
	for _, c := range []struct {
		id, to string
		after  bool
		want   []string
	}{
		{g, "ai", false, []string{Reject, g, "ai", "media", "dev", "direct"}},    // up, before
		{g, "dev", true, []string{Reject, "ai", "media", "dev", g, "direct"}},    // down, after
		{g, "media", false, []string{Reject, "ai", g, "media", "dev", "direct"}}, // up again
		{Reject, "direct", true, []string{"ai", g, "media", "dev", "direct", Reject}},
		{"direct", "ai", true, []string{"ai", "direct", g, "media", "dev", Reject}},
		{"ai", "ai", false, []string{"ai", "direct", g, "media", "dev", Reject}}, // itself: where it is
	} {
		p.PlaceGroup(c.id, p.PlaceAt(c.id, c.to, c.after))
		if got := p.MovableOrder(); !slices.Equal(got, c.want) {
			t.Errorf("%s %v %s: %v, want %v", c.id, map[bool]string{false: "before", true: "after"}[c.after], c.to, got, c.want)
		}
	}
	if !p.PlaceGroup(Reject, 0) || p.MovableOrder()[0] != Reject {
		t.Errorf("to the top: %v", p.MovableOrder())
	}
	if !p.PlaceGroup(Reject, 99) || p.MovableOrder()[5] != Reject {
		t.Errorf("past the end: %v", p.MovableOrder())
	}
	if p.PlaceGroup(Reject, 5) {
		t.Error("staying put reported a move")
	}
	if p.PlaceGroup(Default, 0) || p.PlaceAt(Default, "ai", false) != -1 || p.PlaceAt("ai", Default, false) != -1 || p.PlaceGroup("nowhere", 0) {
		t.Error("the default route moved, or was a place to move to")
	}
	if l := Listed(Build(p, nil)); l[len(l)-1].ID != Default {
		t.Errorf("the default route is not last: %v", l)
	}
	// A group made after the moves still goes where a new group goes:
	// after the others, before the fixed ones at the end.
	h := p.AddGroup(nil, "Work")
	if o := p.MovableOrder(); !slices.Equal(o[len(o)-2:], []string{h, Reject}) {
		t.Errorf("a new group: %v", o)
	}
}
