package main

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/routes"
)

// TestRoutesPickApp: with 应用 chosen, the editor lists the applications
// running and those with connections, these first; a click puts the name in
// the rule, ready to add, and what is typed narrows the list.
func TestRoutesPickApp(t *testing.T) {
	withAppRules(t, true)
	u := routesFixture()
	u.selectGroup("ai")
	u.routes.svc = routes.MineID("ai")
	u.routes.kind = routes.KindApp
	icon := image.NewNRGBA(image.Rect(0, 0, 36, 36))
	for i := range icon.Pix {
		icon.Pix[i] = 0xc0
	}
	icon.Set(0, 0, color.Black)
	u.routes.picker = appPicker{
		apps: []appChoice{
			{Name: "Claude.app", Bundle: "/Applications/Claude.app", Conns: 3},
			{Name: "Telegram.exe"},
			{Name: "Zed.app", Bundle: "/Applications/Zed.app"},
		},
		at:    time.Now(),
		icons: map[string]*ui.Bitmap{"/Applications/Claude.app": ui.NewBitmap(icon)},
	}
	claude := displayApp("Claude.app")
	tt := ui.NewTester(u.view, 1060, 1400)
	tt.Advance(time.Second)
	for _, s := range []string{"有连接的应用", "正在运行的应用", claude, "Telegram.exe", displayApp("Zed.app")} {
		if !tt.HasText(s) {
			t.Errorf("no %q: %q", s, tt.Texts())
		}
	}
	if !hasTextPart(tt, "在下面点一个正在运行的应用") {
		t.Errorf("the hint doesn't point at the list: %q", tt.Texts())
	}
	shot(t, tt, "routes-pick-app")
	// By the chip's own label: the group has a service called Claude too.
	if err := tt.Click("Claude.app，3 个连接"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if u.routes.rule != "Claude.app" || !hasTextPart(tt, "添加后：Claude.app · 应用") {
		t.Fatalf("after the click: rule %q, %q", u.routes.rule, tt.Texts())
	}
	// The choice made, the others are still there to change it.
	if !tt.HasText("Telegram.exe") {
		t.Errorf("the list narrowed to the choice: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Frame()
	got := u.shownSettings().Rules
	if want := (routes.UserRule{Kind: routes.KindApp, Value: "Claude.app", Route: "ai"}); len(got) != 1 || got[0] != want {
		t.Fatalf("rules after Enter: %+v", got)
	}

	// Typing narrows the list; a name no app has is still welcome.
	tt.Advance(100 * time.Millisecond)
	typeInto(t, tt, "新规则", "tele")
	if !tt.HasText("Telegram.exe") || tt.HasText(displayApp("Zed.app")) || tt.HasText("有连接的应用") {
		t.Errorf("typed tele: %q", tt.Texts())
	}
	typeInto(t, tt, "新规则", "zz")
	if !hasTextPart(tt, "没有名字含「zz」的应用在运行") {
		t.Errorf("typed zz: %q", tt.Texts())
	}
}

// TestPickable: what is typed narrows the applications, without regard to
// case; the name of one shows them all.
func TestPickable(t *testing.T) {
	apps := []appChoice{{Name: "Claude.app"}, {Name: "Telegram.exe"}, {Name: "telegram-desktop"}}
	for typed, want := range map[string]int{"": 3, "TELE": 2, "claude.app": 3, "qq": 0, " clau ": 1} {
		if got := pickable(apps, typed); len(got) != want {
			t.Errorf("%q: %d apps, want %d", typed, len(got), want)
		}
	}
}

// TestRoutesPickAppNone: where there is nothing to pick (no process list
// and no connections), the hint doesn't point at a list below.
func TestRoutesPickAppNone(t *testing.T) {
	withAppRules(t, true)
	u := routesFixture()
	u.selectGroup("ai")
	u.routes.svc = routes.MineID("ai")
	u.routes.kind = routes.KindApp
	u.routes.picker = appPicker{at: time.Now()}
	tt := ui.NewTester(u.view, 1060, 1400)
	tt.Advance(time.Second)
	if hasTextPart(tt, "在下面点一个正在运行的应用") || !hasTextPart(tt, "写应用或进程的名字") {
		t.Errorf("hint with nothing to pick: %q", tt.Texts())
	}
}
