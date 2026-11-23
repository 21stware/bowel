package main

import (
	"image"
	"image/color"
	"runtime"
	"slices"
	"strings"
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

// webKitPath is where the core says the WebKit networking process is, on
// macOS 27.2: under the system's cryptex, outside every app's bundle.
const webKitPath = "/System/Volumes/Preboot/Cryptexes/OS/System/Library/Frameworks/WebKit.framework/Versions/A/XPCServices/com.apple.WebKit.Networking.xpc/Contents/MacOS/com.apple.WebKit.Networking"

// TestRoutesAppNoteSafari: a rule for Safari says, before it is added, that
// it leaves Safari's pages out, and offers the WebKit process, which says
// what else it takes.
func TestRoutesAppNoteSafari(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("bundles and the WebKit process are macOS's")
	}
	withAppRules(t, true)
	u := routesFixture()
	u.selectGroup("ai")
	u.routes.svc = routes.MineID("ai")
	u.routes.kind = routes.KindApp
	u.routes.picker = appPicker{at: time.Now(), apps: []appChoice{{Name: "Safari.app", Bundle: "/Applications/Safari.app"}, {Name: "Zed.app", Bundle: "/Applications/Zed.app"}}}
	tt := ui.NewTester(u.view, 1060, 2200)
	tt.Advance(time.Second)
	if hasTextPart(tt, "WebKit") {
		t.Fatalf("a note before anything is chosen: %q", tt.Texts())
	}
	if err := tt.Click("Zed.app"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if hasTextPart(tt, "WebKit") {
		t.Fatalf("a note for an app that connects for itself: %q", tt.Texts())
	}
	if err := tt.Click("Safari.app"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !hasTextPart(tt, "这条规则管不到网页") {
		t.Fatalf("no word that Safari's rule misses its pages: %q", tt.Texts())
	}
	shot(t, tt, "routes-app-note-safari")
	if err := tt.Click("改用 WebKit 网络进程"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if u.routes.rule != routes.WebKitProcess {
		t.Fatalf("the rule is %q, want the WebKit process", u.routes.rule)
	}
	if !hasTextPart(tt, "这条规则会把它们一起管到") || tt.HasText("改用 WebKit 网络进程") {
		t.Errorf("the WebKit process's rule doesn't say what it takes: %q", tt.Texts())
	}
	shot(t, tt, "routes-app-note-webkit")
}

// TestActivityWebKitMenu: a page in Safari is the WebKit process's
// connection, so 让它走… offers a rule for that process, never one for
// Safari.app that would not match, and says what else the rule takes.
func TestActivityWebKitMenu(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the WebKit process is macOS's")
	}
	withAppRules(t, true)
	u := actUI()
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(time.Second)
	if err := tt.Click("www.bilibili.com:443"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if err := tt.Click("让它走…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	m := tt.Menu()
	if !slices.Contains(m, "让 "+routes.WebKitProcess+" 的所有连接走…") || !slices.Contains(m, "Safari 和内嵌网页的应用都算在内") {
		t.Fatalf("the menu doesn't offer the WebKit process with its note: %q", m)
	}
	if slices.ContainsFunc(m, func(s string) bool { return strings.Contains(s, "Safari.app") }) {
		t.Errorf("the menu offers a rule for Safari.app: %q", m)
	}
	shot(t, tt, "activity-route-menu-webkit")
}
