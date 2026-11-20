package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/settings"
)

// lockFixture is the board with route group ai's detail open.
func lockFixture() *UI {
	u := fixtureUI()
	u.snap.Settings = settings.Defaults.Clone()
	u.home.x, u.home.sub = "routes", "ai"
	return u
}

// The first lock waits for the dialog saying what it does; the lock is on
// once that is confirmed.
func TestFirstLockAsks(t *testing.T) {
	u := lockFixture()
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(2 * time.Second)
	if err := tt.Click("出口锁定"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Advance(time.Second)
	if u.lockAsk != "ai" {
		t.Fatalf("lockAsk %q, want ai", u.lockAsk)
	}
	if u.shownSettings().Routing.Locked("ai") {
		t.Fatal("locked before the dialog was answered")
	}
	if !hasTextPart(tt, "关闭接管时") {
		t.Errorf("the dialog doesn't say what the lock does: %q", tt.Texts())
	}
	shot(t, tt, "lock-ask")
	defer func() { shot(t, tt, "lock-detail") }()
	if err := tt.Click("锁定"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Advance(time.Second)
	if u.lockAsk != "" || !u.shownSettings().Routing.Locked("ai") {
		t.Errorf("after 锁定: ask %q, locked %v", u.lockAsk, u.shownSettings().Routing.Locked("ai"))
	}
}

// Cancelling the dialog leaves the group as it was.
func TestFirstLockCancel(t *testing.T) {
	u := lockFixture()
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(2 * time.Second)
	if err := tt.Click("出口锁定"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if u.lockAsk != "" || u.shownSettings().Routing.Locked("ai") {
		t.Errorf("after 取消: ask %q, locked %v", u.lockAsk, u.shownSettings().Routing.Locked("ai"))
	}
}

// Once a group is locked, another is locked with a click.
func TestLaterLockIsAClick(t *testing.T) {
	u := lockFixture()
	for i := range u.snap.Routes {
		if u.snap.Routes[i].ID == "ai" {
			u.snap.Routes[i].Locked = true
		}
	}
	u.home.sub = "media"
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(2 * time.Second)
	if err := tt.Click("出口锁定"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if u.lockAsk != "" || !u.shownSettings().Routing.Locked("media") {
		t.Errorf("ask %q, media locked %v", u.lockAsk, u.shownSettings().Routing.Locked("media"))
	}
}

// With takeover off, the board says which locked groups are refused, and
// their row says why their connections fail.
func TestLockedBoardWhenOff(t *testing.T) {
	u := fixtureUI()
	u.snap.Takeover.On, u.snap.State = false, StateOff
	u.snap.Takeover.Lock = &Entry{Wanted: true, Status: "on"}
	name := ""
	for i := range u.snap.Routes {
		if u.snap.Routes[i].ID == "ai" {
			u.snap.Routes[i].Locked, u.snap.Routes[i].Refused = true, true
			name = u.snap.Routes[i].Name
		}
	}
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(2 * time.Second)
	if !hasTextPart(tt, "出口锁定 · 正在拦下 "+name) {
		t.Errorf("the takeover cell doesn't say the lock holds: %q", tt.Texts())
	}
	if _, ok := tt.Find(refusedSay()); !ok {
		t.Errorf("no mark saying the group is refused: %q", tt.Texts())
	}
	shot(t, tt, "lock-off-board")
	u.home.x = "routes"
	tt.Advance(2 * time.Second)
	if !hasTextPart(tt, refusedSay()) {
		t.Errorf("the route list doesn't say why the group fails: %q", tt.Texts())
	}

}

// A lock that can't hold, the system proxy being elsewhere, says so.
func TestLockThatCantHold(t *testing.T) {
	u := fixtureUI()
	u.snap.Takeover.On, u.snap.State = false, StateOff
	u.snap.Takeover.Lock = &Entry{Wanted: true, Status: "error", Detail: "系统代理没有指向 bowel，锁定的路由组这时拦不住"}
	for i := range u.snap.Routes {
		if u.snap.Routes[i].ID == "ai" {
			u.snap.Routes[i].Locked, u.snap.Routes[i].Refused = true, true
		}
	}
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(2 * time.Second)
	if !hasTextPart(tt, "拦不住") {
		t.Errorf("a lock that can't hold isn't said: %q", tt.Texts())
	}
}
