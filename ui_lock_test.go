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
	if !hasTextPart(tt, "出口锁定 · 经系统代理拦下 "+name) {
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

// The first-lock dialog says what the lock can't do without TUN.
func TestFirstLockSaysProxyOnly(t *testing.T) {
	u := lockFixture()
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(2 * time.Second)
	if err := tt.Click("出口锁定"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Advance(time.Second)
	for _, part := range []string{"只管得住跟随系统代理的程序", "命令行工具、SDK 和应用仍会直连", "请开着 TUN"} {
		if !hasTextPart(tt, part) {
			t.Errorf("the dialog doesn't say %q: %q", part, tt.Texts())
		}
	}
}

// With takeover off there is no TUN: the board's line doesn't claim more
// than the system proxy can hold.
func TestLockedBoardSaysProxyOnly(t *testing.T) {
	u := fixtureUI()
	u.snap.Takeover.On, u.snap.State = false, StateOff
	u.snap.Takeover.Lock = lockEntry(true, true, false, true, "")
	for i := range u.snap.Routes {
		if u.snap.Routes[i].ID == "ai" {
			u.snap.Routes[i].Locked, u.snap.Routes[i].Refused = true, true
		}
	}
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(2 * time.Second)
	if !hasTextPart(tt, "经系统代理拦下") || hasTextPart(tt, "正在拦下") {
		t.Errorf("the line claims every program is held: %q", tt.Texts())
	}
}

// Takeover by TUN alone whose TUN is lost: the board says the system proxy
// stands in, or that nothing holds the lock, and the group's row and
// padlock say so when it is not held.
func TestLockWhenTunIsLost(t *testing.T) {
	u := fixtureUI()
	u.snap.Takeover.Lock = lockEntry(true, false, true, true, "")
	for i := range u.snap.Routes {
		if u.snap.Routes[i].ID == "ai" {
			u.snap.Routes[i].Locked = true
		}
	}
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(2 * time.Second)
	if !hasTextPart(tt, "系统代理暂时指向 bowel") || hasTextPart(tt, "拦下") {
		t.Errorf("the board doesn't say the system proxy stands in for TUN: %q", tt.Texts())
	}
	shot(t, tt, "lock-tun-lost-fallback")

	u = fixtureUI()
	u.snap.Takeover.Lock = lockEntry(true, false, true, false, "")
	for i := range u.snap.Routes {
		if u.snap.Routes[i].ID == "ai" {
			u.snap.Routes[i].Locked, u.snap.Routes[i].Unheld = true, true
		}
	}
	tt = ui.NewTester(u.view, 1060, 760)
	tt.Advance(2 * time.Second)
	if !hasTextPart(tt, "现在没有守住") {
		t.Errorf("a lock nothing holds isn't said: %q", tt.Texts())
	}
	if _, ok := tt.Find(unheldSay()); !ok {
		t.Errorf("the padlock doesn't say the lock isn't holding: %q", tt.Texts())
	}
	shot(t, tt, "lock-tun-lost-unheld")
	u.home.x = "routes"
	tt.Advance(2 * time.Second)
	if !hasTextPart(tt, unheldSay()) {
		t.Errorf("the group's row doesn't say the lock isn't holding: %q", tt.Texts())
	}
}

// A locked group with no node that works: its row and detail say the lock
// refuses its connections, and the ways out.
func TestLockedGroupWithoutNodeSaysRefusing(t *testing.T) {
	u := lockFixture()
	u.snap.Settings.Routing.SetLocked("ai", true)
	for i := range u.snap.Routes {
		if u.snap.Routes[i].ID == "ai" {
			u.snap.Routes[i].Locked, u.snap.Routes[i].Broken = true, "all"
		}
	}
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(2 * time.Second)
	if !hasTextPart(tt, "没有可用节点，正在拦下") {
		t.Errorf("the lock's chip doesn't say it is refusing: %q", tt.Texts())
	}
	if !hasTextPart(tt, lockedOutSay()) {
		t.Errorf("the detail doesn't say the lock refuses, and the ways out: %q", tt.Texts())
	}
	shot(t, tt, "lock-no-node")
}
