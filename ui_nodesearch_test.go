package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// Searching the nodes until none match keeps the field focused: the empty
// list still holds the header, so the keys that follow land in the field.
func TestNodeSearchKeepsFocusWhenNothingMatches(t *testing.T) {
	u := fixtureUI()
	tt := ui.NewTester(u.view, 1060, 1000)
	u.home.x, u.home.sub = "routes", "media"
	tt.Frame()
	tt.Advance(2 * time.Second)
	// The field sits just before 全部测速.
	b, ok := tt.Find("全部测速")
	if !ok {
		t.Fatalf("no 全部测速: %q", tt.Texts())
	}
	tt.ClickAt(b.X-regionBarGap-regionSearchW/2, b.Y+b.H/2)
	tt.Frame()
	if on, _ := tt.TextInputActive(); !on {
		t.Fatal("clicking the search did not focus it")
	}
	// A key at a time, as typed: nothing matches from the first z.
	for _, k := range "zzzz" {
		tt.Type(string(k))
		tt.Frame()
	}
	if !tt.HasText("没有符合条件的节点") {
		t.Fatalf("no empty state: %q", tt.Texts())
	}
	if on, _ := tt.TextInputActive(); !on || u.home.query != "zzzz" {
		t.Fatalf("query %q, focused %v: the search lost focus once nothing matched", u.home.query, on)
	}
	tt.Key(0, ui.KeyBackspace)
	tt.Frame()
	tt.Type("y")
	tt.Frame()
	if u.home.query != "zzzy" {
		t.Fatalf("query %q after backspace and y, want zzzy", u.home.query)
	}
}
