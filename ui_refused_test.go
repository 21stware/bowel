package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// A node its route's site turned away says 被拒绝 where one nothing
// answered says 超时: in the route's node list, and among its other sites
// for the node it uses.
func TestRefusedNodeSaysSo(t *testing.T) {
	u := lockFixture()
	if len(u.snap.Nodes) < 3 {
		t.Fatalf("the fixture has %d nodes", len(u.snap.Nodes))
	}
	on := func(i int, test NodeTest) {
		test.History = []int{}
		u.snap.Nodes[i].Routes = map[string]NodeTest{"ai": test}
	}
	on(0, NodeTest{Tested: true, Refused: true}) // answered, and turned away
	on(1, NodeTest{Tested: true})                // no answer
	on(2, NodeTest{Tested: true, Delay: 86})
	for i, r := range u.snap.Routes {
		if r.ID == "ai" {
			u.snap.Routes[i].Node = u.snap.Nodes[2].Name
			u.snap.Routes[i].Sites = []SiteDelay{
				{Name: "Claude", URL: "https://api.anthropic.com/v1/models", Tested: true, Refused: true},
				{Name: "Gemini", URL: "https://gemini.google.com", Tested: true, Delay: 210},
			}
		}
	}
	tt := ui.NewTester(u.view, 1060, 1300) // tall enough for the failed nodes, which sort last
	tt.Advance(2 * time.Second)
	refused, timeouts := 0, 0
	for _, s := range tt.Texts() {
		switch s {
		case "被拒绝":
			refused++
		case "超时":
			timeouts++
		}
	}
	// The node and the site are refused; the node nothing answered times
	// out, as does one of the fixture's own.
	if refused != 2 || timeouts < 1 {
		t.Errorf("%d say 被拒绝 and %d 超时, want 2 and at least 1: %q", refused, timeouts, tt.Texts())
	}
	shot(t, tt, "refused-nodes")
}
