package bridge

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/oboo/moho/internal/routes"
)

// The exit lock on the phone, as on the desktop: a group that leaves by a
// node or not at all, which global mode does not send through the default's
// node nor direct mode send direct.
func TestGroupLocks(t *testing.T) {
	x := testBridge(t)
	ctx := context.Background()
	if g := group(t, x, "AI 服务"); g.Locked || !g.Lockable {
		t.Fatalf("before: %+v", g)
	}
	for _, id := range []string{"direct", routes.Reject, routes.Default} {
		i := slices.IndexFunc(x.routing().Groups, func(g groupData) bool { return g.ID == id })
		if x.routing().Groups[i].Lockable {
			t.Errorf("%s offers the lock", id)
		}
		if err := x.routeOp(ctx, callArgs{Op: "setLocked", ID: id, On: true}); err == nil {
			t.Errorf("%s: no error", id)
		}
	}
	if err := x.routeOp(ctx, callArgs{Op: "setDirect", ID: "ai", On: true}); err != nil {
		t.Fatal(err)
	}
	if err := x.routeOp(ctx, callArgs{Op: "setLocked", ID: "ai", On: true}); err != nil {
		t.Fatal(err)
	}
	if g := group(t, x, "AI 服务"); !g.Locked || g.Direct {
		t.Fatalf("locked: %+v, want locked and on a node", g)
	}
	if err := x.routeOp(ctx, callArgs{Op: "setDirect", ID: "ai", On: true}); err == nil {
		t.Fatal("a locked group went direct")
	}
	if i := slices.IndexFunc(x.view().Routes, func(r routeData) bool { return r.ID == "ai" }); i < 0 || !x.view().Routes[i].Locked {
		t.Fatalf("the view doesn't show the lock: %+v", x.view().Routes)
	}

	r, _ := x.route("ai")
	for _, mode := range []string{"global", "direct"} {
		if err := x.setMode(ctx, mode); err != nil {
			t.Fatal(err)
		}
		cfg := config(t, x)
		if cfg["mode"] != "rule" {
			t.Fatalf("%s: the core is in %v, want rule", mode, cfg["mode"])
		}
		raw, _ := json.Marshal(cfg["rules"])
		if !strings.Contains(string(raw), "chatgpt.com,"+routes.Group(r)) {
			t.Fatalf("%s: chatgpt.com no longer goes by its own group: %s", mode, raw)
		}
	}

	if err := x.routeOp(ctx, callArgs{Op: "setLocked", ID: "ai", On: false}); err != nil {
		t.Fatal(err)
	}
	if g := group(t, x, "AI 服务"); g.Locked {
		t.Fatalf("still locked: %+v", g)
	}
	if cfg := config(t, x); cfg["mode"] != "direct" {
		t.Fatalf("unlocked, the core is in %v, want direct", cfg["mode"])
	}
}
