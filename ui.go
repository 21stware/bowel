package main

// The native interface: one window of MyGo's `ui`, drawn on the GPU. The
// view reads u.snap and u.tick on the main thread; the app pushes new ones
// with setSnapshot and setTick. Actions run on goroutines through u.do,
// which turns errors into toasts.
//
// Design: docs/product-ia.md (§4–5.1) and prototype/index.html (v8).

import (
	"embed"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/geo"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/provider"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

type UI struct {
	// quitOpen: the dialog saying what quitting will break is up.
	quitOpen bool
	// lockAsk is the route group whose exit lock, the first one, waits for
	// the dialog saying what it does; "" when none does.
	lockAsk string
	// panelCopied is when the panel last copied the terminal proxy command.
	panelCopied time.Time
	app         *App
	win         *mygo.Window
	// closes counts the windows ⌘W or Escape put away (hideWindow, closeWin), for the
	// tests, which have none.
	closes int
	snap   Snapshot
	tick   Tick

	page   string // home, activity, providers
	toasts []string
	// seeing is the list the frame being drawn adds to, drawn each window's
	// last one, and seen what the app was told of each (watch).
	seeing *viewing
	drawn  map[string]*viewing
	seen   map[string]viewing

	home   homeUI
	routes routesUI
	act    activityUI
	prov   providersUI
	setup  setupUI
	set    settingsUI
	clog   changelogUI
	panel  panelUI
	gw     gatewayUI
}

func newUI(a *App) *UI { return &UI{app: a, page: "home", home: newHomeUI()} }

func (u *UI) setSnapshot(s Snapshot) {
	if u.win == nil {
		u.snap = s
		return
	}
	u.win.Update(func() { u.snap = s; u.settingsRedraw() })
}

func (u *UI) setTick(t Tick) {
	if u.win == nil {
		u.tick = t
		return
	}
	u.win.Update(func() { u.tick = t; u.home.pushTick(t) })
}

// do runs an action off the main thread; a failure becomes a toast.
func (u *UI) do(fn func() error) {
	go func() {
		if err := fn(); err != nil {
			u.toast(err.Error())
		}
	}()
}

func (u *UI) toast(msg string) {
	if u.win == nil {
		return
	}
	u.win.Update(func() { u.toasts = append(u.toasts, msg) })
}

func (u *UI) pending(key string) bool {
	for _, p := range u.snap.Pending {
		if p == key {
			return true
		}
	}
	return false
}

// ---- Palette and type ----

// pal is the editorial palette: paper and ink (in the dark, a warm
// graphite, not a blue-black), one signal color, and the
// route colors, which appear only on the map and as small marks.
type pal struct {
	paper, paper2, card, sheet ui.Color
	ink, ink2, mute, faint     ui.Color
	hair, hair2, hover         ui.Color
	signal, good, warn         ui.Color
	route                      map[string]ui.Color
	// accent fills what is chosen and what to press, the theme color of
	// the settings; onAccent is text on it. accentLine is the accent where
	// it must read on paper, as a line or a word: ink when it is too pale.
	// accentOf is the setting they were made from.
	accent, onAccent, accentLine ui.Color
	accentOf                     string
	// dark is the dark palette's: the presets have a color for each.
	dark bool
	// The skin's (skins.go): font is the face of text, display of big
	// type; round scales corners, 0 square; glow, when not clear, haloes
	// big type and what is filled with the accent; deco is the texture laid
	// over a window.
	font, display string
	round         float32
	glow          ui.Color
	deco          func(pt *ui.Painter, r ui.Rect, p *pal)
	// What a skin may also change, each as every skin had it when left
	// zero. pick is how what is chosen shows: "" filled or raised,
	// "bracket" a dashed box with corner brackets and no fill, "block"
	// filled, with the current exit's name on a block of the accent. tags
	// is how a small tag is drawn: "" tinted, "solid", "outline". caps sets
	// short Latin labels in capitals, and label is their face, "" for
	// font. marker is how the map marks an exit: "" a ring, "box" a
	// bracket box, "cross" a crosshair with a boxed name. console is a
	// terminal's manners: a square switch, the sidebar's icons in gray,
	// a primary button that can't be pressed as an outline, no halo
	// around what is filled, and a failure as a solid tag.
	pick, tags, marker string
	caps, console      bool
	label              string
}

var lightPal = pal{
	paper: ui.Hex("#f6f4ef"), paper2: ui.Hex("#efece5"), card: ui.Hex("#fbfaf7"), sheet: ui.Hex("#fdfcf9"),
	ink: ui.Hex("#151413"), ink2: ui.Hex("#3b3936"), mute: ui.Hex("#8b877e"), faint: ui.Hex("#c2bdb2"),
	hair: ui.RGBA(21, 20, 19, .11), hair2: ui.RGBA(21, 20, 19, .06), hover: ui.RGBA(21, 20, 19, .035),
	signal: ui.Hex("#e2462b"), good: ui.Hex("#2c9a5a"), warn: ui.Hex("#cf8a12"),
	route: map[string]ui.Color{"default": ui.Hex("#2f5bea"), "ai": ui.Hex("#7a4de0"), "media": ui.Hex("#e0457b"), "dev": ui.Hex("#0e9db1"), "direct": ui.Hex("#9a968c")},
	font:  sans, display: sans, round: 1,
}

var darkPal = pal{
	dark:  true,
	paper: ui.Hex("#171615"), paper2: ui.Hex("#1c1b1a"), card: ui.Hex("#201f1d"), sheet: ui.Hex("#242321"),
	ink: ui.Hex("#f3f1ec"), ink2: ui.Hex("#cfccc5"), mute: ui.Hex("#8e8a82"), faint: ui.Hex("#4d4a45"),
	hair: ui.RGBA(244, 243, 238, .11), hair2: ui.RGBA(244, 243, 238, .06), hover: ui.RGBA(244, 243, 238, .04),
	signal: ui.Hex("#ff5a3c"), good: ui.Hex("#3fc07a"), warn: ui.Hex("#f0a630"),
	route: map[string]ui.Color{"default": ui.Hex("#5b80ff"), "ai": ui.Hex("#9a74ff"), "media": ui.Hex("#ff5f93"), "dev": ui.Hex("#22c3d6"), "direct": ui.Hex("#77736b")},
	font:  sans, display: sans, round: 1,
}

// themeAccent is the theme color the windows draw with: the settings'
// Accent, which each window's view sets (useAccent) before it draws.
var themeAccent = settings.Defaults.Accent

// useAccent makes a the theme color of what is drawn next; "" is the
// default's.
func useAccent(a string) {
	if a == "" {
		a = settings.Defaults.Accent
	}
	themeAccent = a
}

// windowPal is p for a window that fades when it loses the user's input,
// as AppKit fades an inactive window's selection: there the accent, what
// is chosen and what to press, goes a quiet gray with ink on it, and the
// accent's lines and selected text go muted. The menu bar's panel and the
// QR hint, never the active window, keep the accent.
func windowPal(c *ui.Context, p *pal) *pal {
	if c.WindowActive() {
		return p
	}
	q := *p
	q.accent, q.onAccent, q.accentLine = p.paper.Mix(p.ink, inactiveMix(p)), p.ink, p.mute
	return &q
}

// inactiveMix is how much ink the inactive gray takes over the paper: a
// touch more in the dark, where the same share reads fainter.
func inactiveMix(p *pal) float32 {
	if p.dark {
		return .2
	}
	return .14
}

// palette is the skin's palette (themeSkin) for the Mac's appearance.
func palette(c *ui.Context) *pal {
	p := skinByID(themeSkin).pal(c.Theme().Dark)
	if p.accentOf != themeAccent {
		p.setAccent(themeAccent)
	}
	return p
}

// accentPresets are the theme colors the settings offer as swatches, with
// their names; "ink" is none, the interface in black and white. hex is
// what the settings keep; light and dark are the colors drawn on each
// paper, "" for hex itself: a lime that glows on graphite is all but
// white on paper, and an indigo that reads on paper sinks into graphite.
var accentPresets = []struct{ hex, light, dark, zh, en string }{
	{"#DDFB4C", "#7A9E00", "", "青柠", "Lime"},
	{"#4B3BF5", "", "#7B6DFF", "靛蓝", "Indigo"},
	{"#FF6A3D", "#E8552A", "", "珊瑚", "Coral"},
	{"#14B8A6", "#0E9F8F", "", "青绿", "Teal"},
	{"#FF5FA2", "#E54888", "", "桃红", "Pink"},
	{"ink", "", "", "墨色", "Ink"},
}

// accentHex is the color theme color a draws as on the dark paper or the
// light: a preset's own for it, or a itself, as a color typed in is.
func accentHex(a string, dark bool) string {
	for _, pr := range accentPresets {
		if !strings.EqualFold(pr.hex, a) {
			continue
		}
		if dark && pr.dark != "" {
			return pr.dark
		}
		if !dark && pr.light != "" {
			return pr.light
		}
	}
	return a
}

// setAccent makes p's accent colors from a theme color (settings.Accent).
func (p *pal) setAccent(a string) {
	p.accentOf = a
	if a == "ink" || !settings.ValidAccent(a) {
		p.accent, p.onAccent, p.accentLine = p.ink, p.paper, p.ink
		return
	}
	p.accent = ui.Hex(accentHex(a, p.dark))
	// Text on it: whichever of near-black and white reads better.
	dark := ui.Hex("#171615")
	if contrast(dark, p.accent) >= contrast(ui.Hex("#ffffff"), p.accent) {
		p.onAccent = dark
	} else {
		p.onAccent = ui.Hex("#ffffff")
	}
	p.accentLine = p.accent
	if contrast(p.accent, p.paper) < 2.2 {
		p.accentLine = p.ink
	}
}

// accentTheme puts p's accent in t: the fills of switches, selections and
// primary buttons, and the focus ring.
func accentTheme(t *ui.Theme, p *pal) {
	t.Accent, t.AccentText = p.accent, p.onAccent
	t.AccentHover, t.AccentPressed = p.accent.Mix(p.onAccent, .1), p.accent.Mix(p.onAccent, .2)
	// Not the toolkit's own fade: the panel, never active, keeps the
	// accent, and the windows fade theirs to a gray made for ink on it
	// (windowPal), where a pale accent's dark text would fade with it.
	t.InactiveAccent = p.accent
	t.Focus = p.accentLine.Alpha(.55)
	t.Selection = p.accentLine.Alpha(.25)
}

// contrast is WCAG's contrast ratio of two opaque colors, 1 to 21.
func contrast(a, b ui.Color) float64 {
	lum := func(c ui.Color) float64 {
		ch := func(v uint8) float64 {
			s := float64(v) / 255
			if s <= .04045 {
				return s / 12.92
			}
			return math.Pow((s+.055)/1.055, 2.4)
		}
		return .2126*ch(c.R) + .7152*ch(c.G) + .0722*ch(c.B)
	}
	l1, l2 := lum(a), lum(b)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + .05) / (l2 + .05)
}

func (p *pal) routeColor(id string) ui.Color {
	if id == routes.Reject {
		return p.signal
	}
	if col, ok := p.route[id]; ok {
		return col
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(id, "g")); err == nil { // a group the user made
		return customRouteColors[(n-1)%len(customRouteColors)]
	}
	return p.mute
}

// customRouteColors mark the groups the user made, in turn.
var customRouteColors = []ui.Color{ui.Hex("#d9722b"), ui.Hex("#3f9d4e"), ui.Hex("#b0902a"), ui.Hex("#c44fc0"), ui.Hex("#4f7f9d"), ui.Hex("#a0694a")}

// sans is the face of all text, headlines included: Raise Sans, bundled on
// Windows and Linux (registerFonts), PingFang on a Mac, and the system's for
// what they lack or when it did not load.
const sans = "Raise Sans, PingFang SC, system-ui"

// motion is the one curve of the interface (the prototype's cubic-bezier(.32,.86,.22,1)).
var motion = ui.Animation{Duration: 520 * time.Millisecond, Ease: ui.CubicBezier(.32, .86, .22, 1)}

// ---- The window ----

func (u *UI) view(c *ui.Context) {
	defer u.watch("window")()
	useLook(u.shownSettings())
	p := windowPal(c, palette(c))
	applyTheme(c, p, true)
	// The caret in ink: the accent may be too pale for a line on paper.
	c.Root().CaretColor(p.ink)
	for _, m := range u.toasts {
		c.Toast(m)
	}
	u.toasts = nil
	u.setupTick()
	c.OnShortcut(ui.Cmd, ui.KeyW, u.hideWindow)
	c.OnShortcut(ui.Cmd, ui.KeyComma, u.openSettings)
	if !u.setup.on {
		c.OnShortcut(ui.Cmd, ui.Key1, func() { u.page = "home" })
		c.OnShortcut(ui.Cmd, ui.Key2, func() { u.page = "activity" })
		c.OnShortcut(ui.Cmd, ui.Key3, func() { u.page = "providers" })
		c.OnShortcut(ui.Cmd, ui.Key4, func() { u.page = "routes" })
		// ⌘V outside a text field, on another page, with a link or node list on
		// the clipboard: open Provider with it in the add bar. Provider's own
		// page handles ⌘V itself.
		if u.page != "providers" && !c.FocusTakesText() && c.Shortcut(ui.Cmd, ui.KeyV) {
			if s := strings.TrimSpace(c.ReadClipboard()); strings.Contains(s, "://") || strings.Contains(s, "proxies:") {
				u.page, u.prov.input, u.prov.focus = "providers", s, true
			}
		}
	}

	ui.Column(c).Fill().Background(p.paper).Children(func() {
		if !u.setup.on {
			ui.Column(c.Key("main")).Grow(1).Children(func() {
				u.titleBar(c, p)
				// Pages switch at once: a tab is a place, not a story.
				ui.Box(c).Grow(1).Children(func() {
					ui.Column(c.Key(u.page)).Absolute().Fill().Children(func() {
						switch u.page {
						case "activity":
							u.activityPage(c, p)
						case "providers":
							u.providersPage(c, p)
						case "routes":
							u.routesPage(c, p)
						default:
							u.homePage(c, p)
						}
					})
				})
			})
		}
		// After the main column: leaving, it fades off what it covered.
		if u.setup.on {
			ui.Box(c.Key("setup")).Absolute().Fill().Background(p.paper).Transition(setupLeave).Children(func() { u.setupPage(c, p) })
		}
		skinDeco(c, p)
		u.quitDialog(c, p)
		u.lockDialog(c, p)
	})
	// Escape with nothing left to step back from puts the window away, as
	// its × does. Declared after the pages, whose steps back come first,
	// and not in the first setup or in a text field, where it would lose
	// what was typed.
	if !u.setup.on && !c.FocusTakesText() {
		escAway(c, u.hideWindow)
	}
}

// hideWindow puts the main window away: bowel stays in the tray.
func (u *UI) hideWindow() {
	u.closes++
	if u.win != nil {
		u.win.Hide()
	}
}

// closeWin closes a settings or What's new window, if it is open.
func (u *UI) closeWin(w *atomic.Pointer[mygo.Window]) {
	u.closes++
	if win := w.Load(); win != nil {
		win.Close()
	}
}

// escAway declares Escape as a window's way out: away runs on a press,
// not on the repeats of one held down, which would carry a step back
// on through the window it was in.
func escAway(c *ui.Context, away func()) {
	c.OnWindowShortcut(0, ui.KeyEscape, func() {
		if !c.ShortcutRepeated() {
			away()
		}
	})
}

// pages are the window's tabs, named in each language.
var pages = []struct{ id, zh, en string }{
	{"home", "代理", "Proxy"},
	{"activity", "日志", "Log"},
	{"providers", "提供商", "Provider"},
	{"routes", "路由组", "Routes"},
}

// titleBarHeight is how tall titleBar is; the pages have the rest of the window.
func titleBarHeight(c *ui.Context) float32 { return max(c.TitleBar().Height, 50) }

// titleBar: the window's controls, the tabs, what needs attention, and the
// state on the right. No line under it. Its 50 is what app.go's
// TrafficLightPosition gives, so the controls center with the tabs.
func (u *UI) titleBar(c *ui.Context, p *pal) {
	bar := c.TitleBar()
	ui.Row(c).Height(titleBarHeight(c)).Padding(0, bar.Right+16, 0, bar.Left+14).Gap(16).AlignItems(ui.Center).DragWindow().Children(func() {
		track := ui.Row(c).Gap(2).Padding(3).Radius(p.r(10)).Shrink(0)
		if !p.brackets() {
			track.Background(p.hair2)
		}
		track.Children(func() {
			for _, pg := range pages {
				id := pg.id
				on := u.page == id
				b := ui.ButtonBase(c.Key(id)).Padding(4, 16).Radius(p.r(7))
				if on && !p.brackets() {
					lit(b.Background(p.accent), p)
				}
				picked(b, p, on).OnClick(func() { u.page = id }).Children(func() {
					ui.Row(c).Gap(6).Children(func() {
						col := p.ink2
						switch {
						case on && p.brackets():
							col = p.ink
						case on:
							col = p.onAccent
						}
						p.word(c, i18n.T(pg.zh, pg.en)).FontSize(13).FontWeight(600).TextColor(col)
						if id == "providers" && u.providerWarn() {
							ui.Box(c).Size(5, 5).Radius(p.r(3)).Background(p.warn)
						}
					})
				})
			}
		})
		u.notice(c, p)
		u.updateNotice(c, p)
		u.statePill(c, p)
		u.settingsButton(c, p)
	})
}

// stateName is a state in the interface's language.
func stateName(s State) string {
	switch s {
	case StateOK:
		return i18n.T("正常", "OK")
	case StateDegraded:
		return i18n.T("降级", "Degraded")
	case StateError:
		return i18n.T("异常", "Error")
	}
	return i18n.T("未接管", "Off")
}

func (u *UI) stateColor(p *pal) ui.Color {
	switch u.snap.State {
	case StateOK:
		return p.good
	case StateDegraded:
		return p.warn
	case StateError:
		return p.signal
	}
	return p.faint
}

func (u *UI) statePill(c *ui.Context, p *pal) {
	ui.Row(c).Gap(6).Padding(4, 11).Radius(p.r(999)).Background(p.hair2).Children(func() {
		ui.Box(c).Size(7, 7).Radius(p.r(4)).Background(u.stateColor(p)).Transition(ui.ElementTransition{Colors: true})
		p.word(c, stateName(u.snap.State)).FontSize(12).FontWeight(600).TextColor(p.ink2)
	})
}

func (u *UI) providerWarn() bool {
	for _, pv := range u.snap.Providers {
		if pv.Warn != "" || pv.Error != "" {
			return true
		}
	}
	return false
}

// notice is what needs attention, in the title bar after the tabs: only
// when degraded or broken. It takes the room left, and shortens.
func (u *UI) notice(c *ui.Context, p *pal) {
	row := ui.Row(c).Grow(1).Basis(0).Shrink(1).Gap(8).AlignItems(ui.Center)
	is := u.snap.Issue
	if is == nil || u.snap.State == StateOff {
		return
	}
	col := p.warn
	if is.Level == "error" {
		col = p.signal
	}
	row.Children(func() {
		ui.Row(c.Key("notice")).Gap(8).AlignItems(ui.Center).Shrink(1).
			Transition(ui.ElementTransition{Animation: &motion, Enter: &ui.Motion{Opacity: 0}, Exit: &ui.Motion{Opacity: 0}}).
			Children(func() {
				ui.Box(c).Size(7, 7).Radius(p.r(4)).Background(col).Shrink(0)
				t := ui.Text(c, is.Text).FontSize(12.5).FontWeight(500).TextColor(p.ink).SingleLine().Shrink(1)
				if is.Detail != "" {
					t.Tooltip(is.Detail)
				} else {
					t.Tooltip(is.Text)
				}
				if is.Action != "" {
					busy := (is.Action == "retake" || is.Action == "tun-update") && u.pending(is.Action)
					label := is.Label
					if busy && is.Action == "retake" {
						label = i18n.T("正在重新接管…", "Retaking…")
					} else if busy && u.snap.Settings.Tun && u.snap.Takeover.Tun.Detail != "" { // the missing rule, not a stale core
						label = i18n.T("正在授权…", "Authorizing…")
					} else if busy {
						label = i18n.T("正在更新…", "Updating…")
					}
					ui.ButtonBase(c).Shrink(0).Padding(5, 6).Margin(-5, -6).OnClick(func() { u.issueAction(is.Action) }).Children(func() {
						shimmer(ui.Text(c, label).FontSize(12.5).FontWeight(600).Underline().TextColor(p.ink).NoWrap(), busy, p)
					})
				}
				if id, ok := strings.CutPrefix(is.Action, "fix:"); ok {
					ui.ButtonBase(c.Key("all-auto")).Shrink(0).Padding(5, 6).Margin(-5, -6).OnClick(func() {
						u.changePolicy(id, func(q *routes.Policy) {
							q.Mode, q.Regions, q.Providers, q.ExceptProviders, q.Nodes = routes.ModeAuto, nil, nil, false, nil
						})
					}).Children(func() {
						ui.Text(c, i18n.T("改用全部节点自动", "Use all nodes")).FontSize(12.5).FontWeight(600).Underline().TextColor(p.ink).NoWrap()
					})
				}
			})
	})
}

func (u *UI) issueAction(action string) {
	switch action {
	case "retake":
		u.do(u.app.retake)
	case "tun-update":
		u.do(u.app.updateTun)
	case "take-dns":
		u.do(u.app.takeDNSNow)
	case "activity":
		u.page = "activity"
	case "providers":
		u.page = "providers"
	default:
		if id, ok := strings.CutPrefix(action, "fix:"); ok {
			u.fixRoute(id)
		}
		u.openSite(action)
	}
}

// openSite opens the website a "site:<url>" action names, and says whether
// action was one. The URL came from a subscription, so it is checked again
// here, where it is handed to the system.
func (u *UI) openSite(action string) bool {
	s, ok := strings.CutPrefix(action, "site:")
	if !ok {
		return false
	}
	if s = provider.SiteURL(s); s != "" {
		u.do(func() error { return openURL(s) })
	}
	return true
}

// projectURL is bowel's repository: its source, issues and releases.
const projectURL = "https://github.com/21stware/bowel"

// docsURL is the user guide at bowel.dev: the Chinese one for either
// Chinese, the English one otherwise.
func docsURL() string {
	if strings.HasPrefix(i18n.Lang(), "zh") {
		return "https://bowel.dev/docs/"
	}
	return "https://bowel.dev/en/docs/"
}

// openWeb is openURL, which tests swap out so as not to open a browser.
var openWeb = openURL

// browse opens one of bowel's own websites in the default browser.
func (u *UI) browse(url string) { u.do(func() error { return openWeb(url) }) }

// fixRoute opens where a broken group's strategy can be changed: its card
// when already on 路由组, otherwise its expansion on 代理.
func (u *UI) fixRoute(id string) {
	if u.page == "routes" {
		u.selectGroup(id)
		u.routes.pol = true
		return
	}
	u.page = "home"
	u.home.query, u.home.region, u.home.picking = "", "", ""
	if id == routes.Default {
		u.home.x, u.home.sub = "hero", ""
	} else {
		u.home.x, u.home.sub = "routes", id
	}
}

// ---- Shared parts ----

// kicker is the small spaced label above a section.
func kicker(c *ui.Context, p *pal, s string) ui.Element {
	return p.word(c, s).FontSize(10.5).FontWeight(600).LetterSpacing(1.5).TextColor(p.mute)
}

// pageHead is the head of a list page (日志, 提供商): kicker, big count
// and a muted line beside it, with the page's own controls on the right.
// One function, so switching between the pages doesn't shift the head.
// morph animates the count as it changes; leave it off for counts that
// tick every second.
func pageHead(c *ui.Context, p *pal, kick, title, sub string, morph bool, right func()) {
	ui.Row(c).Gap(18).Padding(18, 26, 12, 26).AlignItems(ui.End).Children(func() {
		ui.Column(c).Gap(4).Grow(1).Children(func() {
			kicker(c, p, kick)
			ui.Row(c).Gap(12).AlignItems(ui.Baseline).Children(func() {
				t := disp(ui.Text(c, title), p).FontSize(34).FontWeight(700).LetterSpacing(-.6).TextColor(p.ink).NoWrap()
				if morph {
					t.ContentTransition(morphText)
				}
				ui.Text(c, sub).FontSize(13).FontWeight(500).FontFeatures("tnum").TextColor(p.mute).NoWrap()
			})
		})
		if right != nil {
			right()
		}
	})
}

// shimmer makes a text show that what it names is being applied: a band
// of ink passes over it, as the prototype's .shimmer.
func shimmer(e ui.Element, on bool, p *pal) ui.Element {
	if !on {
		return e
	}
	t := e.Loop("shimmer", 1100*time.Millisecond, ui.Linear)
	return e.TextColor(p.faint).MaskLinearGradient(ui.LinearGradient{Angle: 90, Stops: []ui.GradientStop{
		{Color: ui.RGBA(0, 0, 0, .35), At: 0},
		{Color: ui.RGBA(0, 0, 0, .35), At: max(0, t-.25)},
		{Color: ui.RGBA(0, 0, 0, 1), At: t},
		{Color: ui.RGBA(0, 0, 0, .35), At: min(1, t+.25)},
		{Color: ui.RGBA(0, 0, 0, .35), At: 1},
	}}).TextColor(p.ink)
}

// pill is the prototype's round button: the accent, or a hairline ghost.
func pill(c *ui.Context, p *pal, label string, primary bool) ui.Element {
	// Capitals are wider, and so is a monospace: a little smaller and
	// closer, the three under the exit's name stay inside their cell.
	size, padX := float32(12.5), float32(14)
	if (p.caps || p.mono()) && latin(label) {
		size, padX = 11.5, 11
	}
	b := ui.ButtonBase(c).Padding(7, padX).Radius(p.r(999))
	if primary {
		lit(b.Background(p.accent), p)
	} else {
		b.Border(1, p.hair)
	}
	return b.Children(func() {
		col := p.ink
		if primary {
			col = p.onAccent
		}
		p.word(c, label).FontSize(size).FontWeight(600).TextColor(col)
	})
}

// btn is a small outlined button.
func btn(c *ui.Context, p *pal, label string) ui.Element {
	b := ui.ButtonBase(c).Padding(5, 11).Radius(p.r(7)).Border(1, p.hair)
	if b.Hovered() {
		b.BorderColor(p.ink)
	}
	// A label never breaks: "ドメインをコピー" on two lines reads as two buttons.
	return b.Children(func() { p.word(c, label).FontSize(12).FontWeight(500).TextColor(p.ink).NoWrap() })
}

// chevron is the mark of something that opens: a small drawn V, pointing
// down while it is shut and up while it is open. Both are the one shape
// mirrored, so they are the same size in the same place; the font's two
// small triangles are not (the up one is drawn smaller).
func chevron(c *ui.Context, col ui.Color, up bool) ui.Element {
	return ui.Box(c).Size(chevronW, chevronH).Shrink(0).AccessHidden(true).Draw(func(pt *ui.Painter, r ui.Rect) {
		tip, ends := r.Y+r.H-chevronInset, r.Y+chevronInset
		if up {
			tip, ends = ends, tip
		}
		var path ui.Path
		path.MoveTo(r.X+chevronInset, ends).LineTo(r.X+r.W/2, tip).LineTo(r.X+r.W-chevronInset, ends).Cap(ui.RoundCap)
		pt.StrokePath(&path, 1.3, col)
	})
}

// The chevron's box, and how far its stroke stays inside it so that the
// round ends are not cut off.
const (
	chevronW     = 9
	chevronH     = 6
	chevronInset = 1
)

// btnChevron is btn with a chevron after its label: a button that opens
// (down) or closes (up) what is under it.
func btnChevron(c *ui.Context, p *pal, label string, up bool) ui.Element {
	b := ui.ButtonBase(c).Row().Gap(5).AlignItems(ui.Center).Padding(5, 11).Radius(p.r(7)).Border(1, p.hair)
	if b.Hovered() {
		b.BorderColor(p.ink)
	}
	return b.Children(func() {
		p.word(c, label).FontSize(12).FontWeight(500).TextColor(p.ink).NoWrap()
		chevron(c, p.ink, up)
	})
}

// iconBtn is a bare glyph button (‹, ×) with a 32pt target and a hover
// wash, so it is not just the few pixels of the glyph. Callers pull it
// into place with negative margins where the glyph has to line up.
func iconBtn(c *ui.Context, p *pal, glyph string, size float32, label string) ui.Element {
	b := ui.ButtonBase(c).Size(32, 32).Shrink(0).Radius(p.r(8)).Justify(ui.Center).AlignItems(ui.Center).
		Label(label).Tooltip(label + " · Esc")
	col := p.mute
	if b.Pressed() {
		b.Background(p.hair)
		col = p.ink
	} else if b.Hovered() {
		b.Background(p.hair2)
		col = p.ink
	}
	return b.Children(func() { ui.Text(c, glyph).FontSize(size).TextColor(col) })
}

// swatch is a route's color mark.
func swatch(c *ui.Context, col ui.Color, size float32) ui.Element {
	return ui.Box(c).Size(size, size).Radius(2).Background(col)
}

// ---- Formatting ----

func fmtBytes(b int64) string {
	v := float64(b)
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for v >= 1000 && i < len(units)-1 {
		v /= 1024
		i++
	}
	switch {
	case i == 0:
		return fmt.Sprintf("%d B", b)
	case v < 10:
		return fmt.Sprintf("%.2f %s", v, units[i])
	case v < 100:
		return fmt.Sprintf("%.1f %s", v, units[i])
	}
	return fmt.Sprintf("%.0f %s", v, units[i])
}

func fmtRate(b int64) string { return fmtBytes(b) + "/s" }

// splitRate is "1.31" and "MB/s", for a large number beside a small unit.
func splitRate(b int64) (string, string) {
	s := fmtBytes(b)
	v, unit, _ := strings.Cut(s, " ")
	return v, unit + "/s"
}

// toggle is a switch: the toolkit's, or in a skin with a terminal's
// manners a square box whose square knob takes the theme color when on.
func toggle(c *ui.Context, p *pal, on *bool) ui.Element {
	if !p.console {
		return ui.Switch(c, on)
	}
	sw := ui.SwitchBase(c, on).Size(36, 20).BorderWidth(1).BorderColor(p.hair)
	var target float32
	if *on {
		target = 1
		sw.BorderColor(p.accentLine)
	}
	pos := sw.Animate("knob", target, 140*time.Millisecond)
	return sw.Draw(func(pt *ui.Painter, r ui.Rect) {
		const in, d = 4, 12
		pt.Fill(ui.Rect{X: r.X + in + pos*(r.W-d-2*in), Y: r.Y + in, W: d, H: d}, p.mute.Mix(p.accent, pos), 0)
	})
}

// failTag is a status word for a failure, 失败 or 超时, as a skin with a
// terminal's manners says it: on a solid block of the signal color. Where
// the theme color is a red too, the word alone in red reads as one more
// thing chosen. The other skins keep the word in the signal color.
func failTag(c *ui.Context, p *pal, text string, size float32) ui.Element {
	on := p.paper
	if w := ui.Hex("#ffffff"); contrast(w, p.signal) > contrast(on, p.signal) {
		on = w
	}
	e := ui.Row(c).Padding(1, 5).Shrink(0).Background(p.signal)
	e.Children(func() {
		p.word(c, text).FontSize(size).FontWeight(700).TextColor(on).NoWrap()
	})
	return e
}

// timedOut says whether a delay is one p says in a failTag: tested, and
// nothing came back.
func (p *pal) timedOut(ms int, tested bool) bool { return p.console && tested && ms <= 0 }

func fmtDelay(n NodeView) string { return fmtTest(n.Delay, n.Tested, n.Refused) }

func fmtSite(s SiteDelay) string { return fmtTest(s.Delay, s.Tested, s.Refused) }

func fmtMs(ms int, tested bool) string { return fmtTest(ms, tested, false) }

// fmtTest is a test's result in words: its delay, or how it failed.
func fmtTest(ms int, tested, refused bool) string {
	switch {
	case !tested:
		return "—"
	case ms <= 0:
		return failWord(refused)
	}
	return fmt.Sprintf("%d ms", ms)
}

// failWord is what a failed test is called: refused when the site answered
// and turned the node away, a timeout when nothing answered.
func failWord(refused bool) string {
	if refused {
		return i18n.T("被拒绝", "Refused")
	}
	return i18n.T("超时", "Timeout")
}

func delayColor(p *pal, ms int, tested bool) ui.Color {
	switch {
	case !tested:
		return p.mute
	case ms <= 0:
		return p.signal
	case ms < 120:
		return p.good
	case ms < 250:
		return p.warn
	}
	return p.signal
}

// siteColor is delayColor for a whole request to a site, which takes
// longer than the small test URL.
func siteColor(p *pal, ms int, tested bool) ui.Color {
	switch {
	case !tested:
		return p.mute
	case ms <= 0:
		return p.signal
	case ms < 400:
		return p.good
	case ms < 900:
		return p.warn
	}
	return p.signal
}

func ago(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return i18n.T("刚刚", "Just now")
	case d < time.Hour:
		return fmt.Sprintf(i18n.T("%d 分钟前", "%d min ago"), int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf(i18n.T("%d 小时前", "%d h ago"), int(d.Hours()))
	}
	return fmt.Sprintf(i18n.T("%d 天前", "%d d ago"), int(d.Hours()/24))
}

// ---- Lookups ----

func (u *UI) node(name string) (NodeView, bool) {
	for _, n := range u.snap.Nodes {
		if n.Name == name {
			return n, true
		}
	}
	return NodeView{}, false
}

func (u *UI) route(id string) RouteView {
	for _, r := range u.snap.Routes {
		if r.ID == id {
			return r
		}
	}
	return RouteView{}
}

func (u *UI) providerName(id string) string {
	for _, pv := range u.snap.Providers {
		if pv.ID == id {
			return pv.Name
		}
	}
	return ""
}

// trafficSeen says whether bowel has traffic to show: a connection now, a
// recent event, a connection the Log page still lists, or bytes in the
// history the graph draws. It does not ask whether bowel has taken over:
// an application pointed at the mixed port passes traffic either way.
func (u *UI) trafficSeen() bool {
	return u.tick.Active > 0 || len(u.tick.Events) > 0 || len(u.act.conns) > 0 || u.home.moved()
}

// active says whether a route carries traffic in the current mode.
func (u *UI) active(id string) bool {
	if !u.snap.Takeover.On {
		return false
	}
	switch u.snap.Mode {
	case "global":
		return id == "default"
	case "direct":
		return id == "direct"
	}
	return true
}

// nodeFlag is the flag emoji of where n is, "" while that is not known.
func nodeFlag(n NodeView) string {
	if n.Where.Place != nil {
		return n.Where.Place.Flag
	}
	return ""
}

// flagged lays out n's flag before its name, a round badge sized to the
// name's text (size) sitting on its baseline, as a small picture in a line
// of text does.
func flagged(c *ui.Context, p *pal, n NodeView, size float32, name func()) ui.Element {
	return ui.Row(c).Gap(round(size * .34)).AlignItems(ui.Baseline).Children(func() {
		flagBadge(c, p, nodeFlag(n), round(size*.8))
		name()
	})
}

// flagBadge is the round flag of a place (its flag emoji) d DIPs across,
// drawn from assets/flags, with a hairline so that white flags show on
// paper. No flag is a dashed ring: the place isn't known. A country with
// no picture shows its letters on a plain disc.
func flagBadge(c *ui.Context, p *pal, flag string, d float32) ui.Element {
	cc := countryCode(flag)
	pic := flagSVG(cc)
	return ui.Box(c).Size(d, d).Shrink(0).Draw(func(pt *ui.Painter, r ui.Rect) {
		rad := r.W / 2
		switch {
		case flag == "":
			var ring ui.Path
			ring.Circle(r.X+rad, r.Y+rad, rad-.5).Dash(0, 2, 2)
			pt.StrokePath(&ring, 1, p.faint)
			return
		case pic != nil:
			pt.Image(pic, r, ui.Contain)
		default:
			pt.Fill(r, p.hair2, rad)
			span := ui.Span{Text: cc, Size: d * .4, Weight: 700, Color: p.ink2}
			tw, th := pt.MeasureText(0, span)
			pt.RichText(r.X+(r.W-tw)/2, r.Y+(r.H-th)/2, 0, span)
		}
		pt.Stroke(r, p.ink.Alpha(.12), rad, .75)
	})
}

//go:embed assets/flags/*.svg
var flagFiles embed.FS

var flagSVGs sync.Map // country code → *ui.SVG, nil when there is none

// flagSVG is the round flag of country cc ("HK"), parsed once; nil when
// assets/flags has none.
func flagSVG(cc string) *ui.SVG {
	if v, ok := flagSVGs.Load(cc); ok {
		return v.(*ui.SVG)
	}
	var s *ui.SVG
	if b, err := flagFiles.ReadFile("assets/flags/" + strings.ToLower(cc) + ".svg"); err == nil {
		s, _ = ui.ParseSVG(b)
	}
	flagSVGs.Store(cc, s)
	return s
}

// countryCode is the letters of a flag emoji, "HK" of 🇭🇰.
func countryCode(flag string) string {
	var b []rune
	for _, r := range flag {
		if r >= 0x1F1E6 && r <= 0x1F1FF {
			b = append(b, 'A'+r-0x1F1E6)
		}
	}
	return string(b)
}

func round(v float32) float32 { return float32(math.Round(float64(v))) }

func placeName(n NodeView) string {
	if n.Where.Place != nil {
		return geo.Display(n.Where.Place.City)
	}
	if n.Where.Source == "locating" {
		return i18n.T("定位中", "Locating")
	}
	return i18n.T("位置未知", "Unknown location")
}

// exitNet is a measured exit's IP and the network it belongs to, e.g.
// "203.0.113.7 · AS13335 Cloudflare, Inc."; "" until there is an IP.
func exitNet(l Located) string {
	if l.IP == "" {
		return ""
	}
	s := l.IP
	if l.ASN != 0 {
		s += fmt.Sprintf(" · AS%d", l.ASN)
		if l.Org != "" {
			s += " " + l.Org
		}
	}
	return s
}

// locTag is the small label saying how a place is known.
func locTag(c *ui.Context, p *pal, src string) {
	text, col := i18n.T("未知", "Unknown"), p.warn
	switch src {
	case "probe":
		text, col = i18n.T("实测", "Measured"), p.good
	case "name":
		text, col = i18n.T("按名称", "By name"), p.mute
	case "locating":
		text, col = i18n.T("定位中", "Locating"), p.warn
	}
	tag(c, p, text, col)
}

// tag is a small label in col: tinted with it, or as the skin's tags
// are, a solid block of it or a hairline box.
func tag(c *ui.Context, p *pal, text string, col ui.Color) ui.Element {
	// A tag never breaks: squeezed, its letters would stack one a line.
	b := ui.Box(c).Padding(1, 6).Radius(p.r(4)).Shrink(0)
	fg := col
	switch p.tags {
	case "solid":
		b.Background(col)
		if fg = p.paper; contrast(p.ink, col) > contrast(fg, col) {
			fg = p.ink
		}
	case "outline":
		b.Border(1, col.Alpha(.6))
	default:
		b.Background(col.Alpha(.12))
	}
	b.Children(func() {
		p.word(c, text).FontSize(10.5).FontWeight(600).TextColor(fg).NoWrap()
	})
	return b
}

// cap is s as p's short labels say it: in capitals, where the skin sets
// them and every letter of s is Latin. 代理, ログ and 香港 03 have no
// capitals, and a label half in them reads worse than one left alone.
// It is for labels only: a name or a sentence is not passed through it.
func (p *pal) cap(s string) string {
	if !p.caps || !latin(s) {
		return s
	}
	return strings.ToUpper(s)
}

// latin says whether every letter of s is a Latin one.
func latin(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) && !unicode.Is(unicode.Latin, r) {
			return false
		}
	}
	return true
}

// word is s, a short label, as p draws its labels: in their face, and in
// capitals where the skin sets them. Capitals are how it is drawn, not
// what it is called: assistive technology gets it as written.
func (p *pal) word(c *ui.Context, s string) ui.Element {
	e := p.lab(ui.Text(c, p.cap(s)))
	if p.cap(s) != s {
		e.Label(s)
	}
	return e
}

// lab sets e, a short label, in p's label face, spaced a little where it
// is in capitals. Set a letter spacing of the label's own after it.
func (p *pal) lab(e ui.Element) ui.Element {
	if p.label != "" {
		e.Font(p.label)
	}
	if p.caps {
		e.LetterSpacing(.6)
	}
	return e
}

// labelFace is the face of p's short labels, for what is painted, not laid
// out.
func (p *pal) labelFace() string {
	if p.label != "" {
		return p.label
	}
	return p.font
}

// brackets says whether p marks what is chosen with a box of brackets,
// in place of a fill or a ring.
func (p *pal) brackets() bool { return p.pick == "bracket" }

var _ = math.Pi
