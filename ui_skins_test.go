package main

import (
	"image"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/settings"
)

// TestSkins: the skins are the settings' choices, in order; each palette
// is whole and reads, its own theme color included; a skin with one
// palette keeps its appearance.
func TestSkins(t *testing.T) {
	ids := make([]string, len(skins))
	for i, k := range skins {
		ids[i] = k.id
	}
	if !slices.Equal(ids, settings.Skins) {
		t.Fatalf("skins %v, settings %v", ids, settings.Skins)
	}
	if skins[0].id != settings.Defaults.Skin || skinByID("").id != settings.Defaults.Skin || skinByID("nope").id != settings.Defaults.Skin {
		t.Error("the default skin is not the first, or an unknown one is not it")
	}
	for _, k := range skins {
		if !settings.ValidAccent(k.accent) {
			t.Errorf("%s: theme color %q", k.id, k.accent)
		}
		for _, base := range []*pal{k.light, k.dark} {
			if base == nil {
				continue
			}
			p := *base
			p.setAccent(k.accent)
			name := k.id + map[bool]string{false: "", true: " dark"}[p.dark]
			if p.font == "" || p.display == "" {
				t.Errorf("%s: no face", name)
			}
			if p.round < 0 || p.round > 2 {
				t.Errorf("%s: round %v", name, p.round)
			}
			if c := contrast(p.ink, p.paper); c < 7 {
				t.Errorf("%s: ink on paper is %.2f:1", name, c)
			}
			if c := contrast(p.mute, p.paper); c < 2.4 {
				t.Errorf("%s: muted text on paper is %.2f:1", name, c)
			}
			if c := contrast(p.onAccent, p.accent); c < 3 {
				t.Errorf("%s: text on its theme color is %.2f:1", name, c)
			}
			for _, id := range []string{"default", "ai", "media", "dev", "direct"} {
				if _, ok := p.route[id]; !ok {
					t.Errorf("%s: no color for route %s", name, id)
				}
			}
		}
	}
	s := settings.Defaults.Clone()
	s.Appearance = "light"
	if appearanceOf(s) != "light" {
		t.Errorf("paper, light: %s", appearanceOf(s))
	}
	skinPick(&s, "phosphor")
	if s.Skin != "phosphor" || s.Accent != "#46F08A" || appearanceOf(s) != "dark" {
		t.Errorf("phosphor: %s %s %s", s.Skin, s.Accent, appearanceOf(s))
	}
	if err := s.Validate(); err != nil {
		t.Error(err)
	}
	s.Skin = "chrome"
	if s.Validate() == nil {
		t.Error("an unknown skin passes")
	}
}

// TestSkinPick: a skin's card in the settings picks it and its theme
// color, and the windows draw in it at once.
func TestSkinPick(t *testing.T) {
	u := fixtureUI()
	u.snap.Settings = settings.Defaults.Clone()
	tt := ui.NewTesterWith(u.settingsView, 760, 900, ui.TesterOptions{})
	tt.Advance(time.Second)
	if err := tt.Click("霓虹"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Frame()
	s := u.shownSettings()
	if s.Skin != "neon" || s.Accent != "#FF3FA4" {
		t.Fatalf("after the card: skin %q, accent %q", s.Skin, s.Accent)
	}
	if !tt.HasText("这款皮肤只有深色") {
		t.Error("the appearance does not say the skin keeps the dark")
	}
	home := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
	home.Frame()
	r, g, b, _ := home.Image().At(530, 400).RGBA()
	if q := neonPal.paper; r>>8 > uint32(q.R)+24 || g>>8 > uint32(q.G)+24 || b>>8 > uint32(q.B)+24 {
		t.Errorf("the main window draws %d %d %d, not on neon's paper", r>>8, g>>8, b>>8)
	}
}

// TestMrRobotSkin: 黑客军团 keeps the dark and brings its red, with white
// on it; it is a terminal, in one face with square corners; its texture is
// its own; and the main window draws on its near-black paper, which the
// texture leaves dark.
func TestMrRobotSkin(t *testing.T) {
	k := skinByID("mrrobot")
	if k.id != "mrrobot" || k.fixed() != "dark" {
		t.Fatalf("skin %q keeps %q", k.id, k.fixed())
	}
	s := settings.Defaults.Clone()
	s.Appearance = "light"
	skinPick(&s, "mrrobot")
	if s.Accent != "#E0242B" || appearanceOf(s) != "dark" {
		t.Errorf("picked: accent %s, appearance %s", s.Accent, appearanceOf(s))
	}
	if err := s.Validate(); err != nil {
		t.Error(err)
	}
	p := *k.dark
	p.setAccent(k.accent)
	if p.onAccent != ui.Hex("#ffffff") || p.accentLine != p.accent {
		t.Errorf("on its red: text %v, line %v", p.onAccent, p.accentLine)
	}
	if p.font != faceMono || p.display != faceMono || p.round != 0 {
		t.Errorf("face %q, display %q, round %v", p.font, p.display, p.round)
	}
	if p.deco == nil {
		t.Fatal("no texture")
	}
	for _, col := range []ui.Color{p.ink2, p.signal, p.good, p.warn, p.route["default"], p.route["ai"], p.route["media"], p.route["dev"]} {
		if c := contrast(col, p.paper); c < 4.5 {
			t.Errorf("%v on its paper is %.2f:1", col, c)
		}
	}
	u := fixtureUI()
	u.snap.Settings = s
	tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{Dark: true})
	tt.Frame()
	// Between the scan lines, in the middle, where the texture is lightest.
	r, g, b, _ := tt.Image().At(530, 401).RGBA()
	if q := p.paper; r>>8 > uint32(q.R)+24 || g>>8 > uint32(q.G)+24 || b>>8 > uint32(q.B)+24 {
		t.Errorf("the main window draws %d %d %d, not on its paper", r>>8, g>>8, b>>8)
	}
	// The cursor, bottom right: red half of the time, and all of it for a
	// desktop that asks for less motion.
	lit := func(tt *ui.Tester) bool {
		r, g, _, _ := tt.Image().At(1048, 745).RGBA()
		return r>>8 > 150 && g>>8 < 90
	}
	on := 0
	for i := 0; i < 4; i++ {
		tt.Advance(cursorBlink)
		if lit(tt) {
			on++
		}
	}
	if on != 2 {
		t.Errorf("the cursor shows in %d of 4 half blinks", on)
	}
	still := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{Dark: true, Preferences: ui.Preferences{ReduceMotion: true}})
	for i := 0; i < 4; i++ {
		still.Advance(cursorBlink)
		if !lit(still) {
			t.Fatal("the cursor blinks for a desktop that asks for less motion")
		}
	}
}

// TestSkinAbilitiesDefault: what a skin may change beyond its colors,
// faces, corners and texture is left alone by every skin that does not
// ask for it, so they draw as they did.
func TestSkinAbilitiesDefault(t *testing.T) {
	for _, k := range skins {
		if k.id == "mrrobot" || k.id == "machine" || k.id == "samaritan" {
			continue
		}
		for _, p := range []*pal{k.light, k.dark} {
			if p == nil {
				continue
			}
			if p.pick != "" || p.tags != "" || p.marker != "" || p.label != "" || p.caps || p.console {
				t.Errorf("%s: pick %q, tags %q, marker %q, label %q, caps %v, console %v", k.id, p.pick, p.tags, p.marker, p.label, p.caps, p.console)
			}
		}
	}
}

// near says whether the pixel of tt's frame at x, y is within d of col in
// each channel.
func near(tt *ui.Tester, x, y int, col ui.Color, d int) bool {
	return nearIn(tt.Image(), x, y, col, d)
}

// nearIn is near in a picture already taken: taking one for each pixel
// of an area is slow.
func nearIn(img image.Image, x, y int, col ui.Color, d int) bool {
	r, g, b, _ := img.At(x, y).RGBA()
	diff := func(a uint32, b uint8) bool { return int(a>>8)-int(b) <= d && int(b)-int(a>>8) <= d }
	return diff(r, col.R) && diff(g, col.G) && diff(b, col.B)
}

// lighter says whether the pixel at x, y is lighter than col by at least d.
func lighter(tt *ui.Tester, x, y int, col ui.Color, d int) bool {
	r, g, b, _ := tt.Image().At(x, y).RGBA()
	return int(r>>8)+int(g>>8)+int(b>>8) >= int(col.R)+int(col.G)+int(col.B)+3*d
}

// TestMachineSkin: The Machine keeps the dark and brings its yellow, with
// dark text on it; white is what is well, yellow what is chosen and red
// what failed; labels are in Jost and data in the bundled monospace, with
// square corners; it asks for brackets, solid tags, capitals, boxes on the
// map and a terminal's manners; and its texture leaves the paper black,
// with a bracket in each corner of the window and ticks along its edges.
func TestMachineSkin(t *testing.T) {
	k := skinByID("machine")
	if k.id != "machine" || k.fixed() != "dark" {
		t.Fatalf("skin %q keeps %q", k.id, k.fixed())
	}
	s := settings.Defaults.Clone()
	s.Appearance = "light"
	skinPick(&s, "machine")
	if s.Accent != "#F2E02A" || appearanceOf(s) != "dark" {
		t.Errorf("picked: accent %s, appearance %s", s.Accent, appearanceOf(s))
	}
	if err := s.Validate(); err != nil {
		t.Error(err)
	}
	p := *k.dark
	p.setAccent(k.accent)
	if p.onAccent != ui.Hex("#171615") || p.accentLine != p.accent {
		t.Errorf("on its yellow: text %v, line %v", p.onAccent, p.accentLine)
	}
	if p.good != p.ink || p.warn != p.accent {
		t.Errorf("well is %v and a warning %v, not its white and its yellow", p.good, p.warn)
	}
	if p.font != facePlex || p.display != faceJost || p.label != faceJost || p.round != 0 || p.glow.A != 0 {
		t.Errorf("face %q, display %q, label %q, round %v, glow %v", p.font, p.display, p.label, p.round, p.glow)
	}
	if p.pick != "bracket" || p.tags != "solid" || p.marker != "box" || !p.caps || !p.console {
		t.Errorf("pick %q, tags %q, marker %q, caps %v, console %v", p.pick, p.tags, p.marker, p.caps, p.console)
	}
	for _, col := range []ui.Color{p.ink2, p.mute, p.signal, p.warn, p.route["default"], p.route["ai"], p.route["media"], p.route["dev"]} {
		if c := contrast(col, p.paper); c < 4.5 {
			t.Errorf("%v on its paper is %.2f:1", col, c)
		}
	}
	u := fixtureUI()
	u.snap.Settings = s
	tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{Dark: true})
	tt.Frame()
	if !near(tt, 530, 401, p.paper, 24) {
		t.Errorf("the main window does not draw on its paper at 530, 401: %v", tt.Image().At(530, 401))
	}
	// A bracket's upright arm, top left, and a tick on the top edge.
	if !lighter(tt, 6, 14, p.paper, 60) {
		t.Errorf("no bracket in the window's corner: %v", tt.Image().At(6, 14))
	}
	if !lighter(tt, 125, 1, p.paper, 40) {
		t.Errorf("no tick on the window's edge: %v", tt.Image().At(125, 1))
	}
}

// TestSamaritanSkin: Samaritan follows the appearance, white as the show
// has it or dark, and brings its red, with white on it, in both; it is in
// one monospace with square corners; it asks for a block under the current
// exit, outlined tags, capitals, crosshairs on the map and a terminal's
// manners; and its texture leaves each paper its own, with a bracket in
// each corner of the window.
func TestSamaritanSkin(t *testing.T) {
	k := skinByID("samaritan")
	if k.id != "samaritan" || k.fixed() != "" || k.light == nil || k.dark == nil {
		t.Fatalf("skin %q keeps %q", k.id, k.fixed())
	}
	for _, app := range []string{"light", "dark"} {
		s := settings.Defaults.Clone()
		s.Appearance = app
		skinPick(&s, "samaritan")
		if s.Accent != "#E3202B" || appearanceOf(s) != app {
			t.Errorf("picked in the %s: accent %s, appearance %s", app, s.Accent, appearanceOf(s))
		}
		if err := s.Validate(); err != nil {
			t.Error(err)
		}
		dark := app == "dark"
		p := *k.pal(dark)
		p.setAccent(k.accent)
		if p.dark != dark {
			t.Fatalf("the %s palette is not", app)
		}
		if p.onAccent != ui.Hex("#ffffff") || p.accentLine != p.accent {
			t.Errorf("%s, on its red: text %v, line %v", app, p.onAccent, p.accentLine)
		}
		if p.font != facePlex || p.display != facePlex || p.label != facePlex || p.round != 0 || p.glow.A != 0 {
			t.Errorf("%s: face %q, display %q, label %q, round %v, glow %v", app, p.font, p.display, p.label, p.round, p.glow)
		}
		if p.pick != "block" || p.tags != "outline" || p.marker != "cross" || !p.caps || !p.console {
			t.Errorf("%s: pick %q, tags %q, marker %q, caps %v, console %v", app, p.pick, p.tags, p.marker, p.caps, p.console)
		}
		for _, col := range []ui.Color{p.ink2, p.mute, p.signal, p.warn, p.route["default"], p.route["media"], p.route["dev"]} {
			if c := contrast(col, p.paper); c < 4.5 {
				t.Errorf("%s: %v on its paper is %.2f:1", app, col, c)
			}
		}
		// Its red fills and draws lines; it is not the color of small text.
		if c := contrast(p.accent, p.paper); c < 3 {
			t.Errorf("%s: its red on its paper is %.2f:1", app, c)
		}
		u := fixtureUI()
		u.snap.Settings = s
		tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{Dark: dark})
		tt.Frame()
		if !near(tt, 530, 401, p.paper, 24) {
			t.Errorf("%s: the main window does not draw on its paper at 530, 401: %v", app, tt.Image().At(530, 401))
		}
		if near(tt, 6, 14, p.paper, 30) {
			t.Errorf("%s: no bracket in the window's corner: %v", app, tt.Image().At(6, 14))
		}
	}
}

// skinned is the fixture in skin id, and whether that skin draws in the dark
// here.
func skinned(u *UI, id string) (*UI, bool) {
	u.snap.Settings = settings.Defaults.Clone()
	skinPick(&u.snap.Settings, id)
	return u, skinByID(id).fixed() == "dark"
}

// palOf is skin id's palette with its own theme color, as the windows
// draw it.
func palOf(id string, dark bool) *pal {
	k := skinByID(id)
	p := *k.pal(dark)
	p.setAccent(k.accent)
	return &p
}

// reddish says whether the pixel at x, y is a red: much more red than
// green.
func reddish(tt *ui.Tester, x, y int) bool {
	r, g, _, _ := tt.Image().At(x, y).RGBA()
	return int(r>>8)-int(g>>8) > 50
}

// TestSquareSwitch: a skin with a terminal's manners draws its switches
// square, a box with a square knob in the theme color when on, and they
// still switch; the others keep the toolkit's round one.
func TestSquareSwitch(t *testing.T) {
	title, _ := traySpeedLabel(runtime.GOOS)
	for _, id := range []string{"paper", "mrrobot"} {
		u, dark := skinned(fixtureUI(), id)
		p := palOf(id, dark)
		tt := ui.NewTesterWith(u.settingsView, 760, 2000, ui.TesterOptions{Dark: dark})
		tt.Advance(time.Second)
		sw, ok := tt.AccessByRole(ui.RoleSwitch, title)
		if !ok {
			t.Fatalf("%s: no switch for the menu bar's speed among %q", id, tt.Texts())
		}
		b := sw.Bounds
		// A round switch leaves its box's corner to the paper.
		square := !near(tt, int(b.X), int(b.Y)+1, p.paper, 10)
		if square != p.console {
			t.Errorf("%s: the switch's corner is %v, square %v", id, tt.Image().At(int(b.X), int(b.Y)+1), square)
		}
		tt.ClickAt(b.X+b.W/2, b.Y+b.H/2)
		tt.Advance(time.Second)
		if !u.shownSettings().TraySpeed {
			t.Fatalf("%s: the switch did not switch", id)
		}
		sw, _ = tt.AccessByRole(ui.RoleSwitch, title)
		if !sw.Checked {
			t.Errorf("%s: the switch does not say it is on", id)
		}
		if p.console && !near(tt, int(b.X+b.W)-9, int(b.Y+b.H/2), p.accent, 40) {
			t.Errorf("%s: no knob in the theme color at the switch's end: %v", id, tt.Image().At(int(b.X+b.W)-9, int(b.Y+b.H/2)))
		}
	}
}

// TestFailTag: a skin whose red also marks what is chosen says a failure
// in a solid block of the signal color; the others keep the word in it.
func TestFailTag(t *testing.T) {
	for _, id := range []string{"paper", "mrrobot"} {
		u, dark := skinned(actUI(), id)
		u.act.status = slices.Index(actStatuses, "failed")
		p := palOf(id, dark)
		tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{Dark: dark})
		tt.Advance(time.Second)
		r, ok := tt.Find("失败")
		if !ok {
			t.Fatalf("%s: no failed connection among %q", id, tt.Texts())
		}
		// Beside the word, inside a tag's padding.
		x, y := int(r.X)-3, int(r.Y+r.H/2)
		if block := near(tt, x, y, p.signal, 45); block != p.console {
			t.Errorf("%s: beside 失败 is %v, a block %v", id, tt.Image().At(x, y), block)
		}
	}
}

// TestConsoleSidebar: in a skin with a terminal's manners the settings'
// sidebar keeps its icons gray, where the toolkit draws them in the theme
// color, eleven red marks under 黑客军团; the page chosen is in the theme
// color, focus or not.
func TestConsoleSidebar(t *testing.T) {
	u, dark := skinned(fixtureUI(), "mrrobot")
	p := palOf("mrrobot", dark)
	tt := ui.NewTesterWith(u.settingsView, 760, 760, ui.TesterOptions{Dark: dark})
	tt.Advance(time.Second)
	r, ok := tt.Find("入站")
	if !ok {
		t.Fatalf("no 入站 among %q", tt.Texts())
	}
	red, gray := 0, 0
	for x := int(r.X) - 26; x < int(r.X)-6; x++ {
		for y := int(r.Y) - 2; y < int(r.Y+r.H)+2; y++ {
			if reddish(tt, x, y) {
				red++
			} else if lighter(tt, x, y, p.paper2, 40) {
				gray++
			}
		}
	}
	if red > 0 || gray == 0 {
		t.Errorf("the icon beside 入站 has %d red pixels and %d gray ones", red, gray)
	}
	r, _ = tt.Find("通用")
	// The label fills the row: look before the icon, inside the row,
	// where the skin's vignette darkens its red.
	if x, y := int(r.X)-29, int(r.Y+r.H/2); !reddish(tt, x, y) {
		t.Errorf("the chosen page's row is %v, not the theme color", tt.Image().At(x, y))
	}
	if err := tt.Click("入站"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if u.set.section != "inbound" {
		t.Errorf("clicking 入站 chose %q", u.set.section)
	}
}

// TestConsoleQuiet: a skin with a terminal's manners puts no halo around
// what is filled with its theme color, and 添加 with nothing to add is an
// outline, not a dim block of a red that reads as an error.
func TestConsoleQuiet(t *testing.T) {
	u, dark := skinned(providersUIFixture(), "mrrobot")
	tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{Dark: dark})
	tt.Advance(time.Second)
	r, ok := tt.Find("提供商")
	if !ok {
		t.Fatalf("no tab among %q", tt.Texts())
	}
	// Just above the chosen tab, where a halo would spill, against the
	// same height above a tab that is not chosen.
	other, _ := tt.Find("日志")
	y := int(r.Y) - 6
	warm := func(x int) int {
		r, g, _, _ := tt.Image().At(x, y).RGBA()
		return int(r>>8) - int(g>>8)
	}
	if d := warm(int(r.X+r.W/2)) - warm(int(other.X+other.W/2)); d > 12 {
		t.Errorf("a halo above the chosen tab: %d redder than above another", d)
	}
	add, ok := tt.AccessByRole(ui.RoleButton, "添加")
	if !ok {
		t.Fatal("no 添加")
	}
	if x, y := int(add.Bounds.X)+5, int(add.Bounds.Y+add.Bounds.H/2); reddish(tt, x, y) {
		t.Errorf("添加 with nothing to add is filled: %v", tt.Image().At(x, y))
	}
	if skinByID("mrrobot").dark.console != true || skinByID("phosphor").dark.console {
		t.Error("黑客军团 has a terminal's manners, 荧光屏 keeps its glow")
	}
}

// TestCap: a skin that sets capitals says a short label in them when all
// its letters are Latin, whatever digits and marks it has; Chinese and
// Japanese, alone or mixed with Latin, stay as written, and so does
// everything in a skin that does not set them.
func TestCap(t *testing.T) {
	p := &pal{caps: true}
	for in, want := range map[string]string{
		"Proxy": "PROXY", "Route · Exit": "ROUTE · EXIT", "Active 13": "ACTIVE 13", "▶ Resume": "▶ RESUME", "Adding…": "ADDING…",
		"代理": "代理", "香港 03": "香港 03", "ログ": "ログ", "HK 香港": "HK 香港", "12 ms": "12 MS", "": "",
	} {
		if got := p.cap(in); got != want {
			t.Errorf("cap(%q) = %q, want %q", in, got, want)
		}
	}
	if got := (&pal{}).cap("Proxy"); got != "Proxy" {
		t.Errorf("without capitals, cap(Proxy) = %q", got)
	}
}

// TestMachineCaps: The Machine says its tabs, buttons, kickers and state
// in capitals in English and leaves Chinese alone; what a tab is called
// to assistive technology keeps its case, and sentences and names are not
// labels. A skin without capitals keeps "Proxy".
func TestMachineCaps(t *testing.T) {
	defer i18n.Set("zh")
	u, dark := skinned(fixtureUI(), "machine")
	tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{Dark: dark})
	tt.Advance(time.Second)
	for _, s := range []string{"代理", "日志", "提供商", "路由组"} {
		if !tt.HasText(s) {
			t.Errorf("no %s among %q", s, tt.Texts())
		}
	}
	i18n.Set("en")
	tt = ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{Dark: dark})
	tt.Advance(time.Second)
	// What is called Proxy for assistive technology is among the texts
	// too, so look for the capitals whole.
	for _, s := range []string{"PROXY", "LOG", "PROVIDER", "ROUTES", "SETTINGS"} {
		if !slices.Contains(tt.Texts(), s) {
			t.Errorf("no %s among %q", s, tt.Texts())
		}
	}
	if _, ok := tt.AccessByRole(ui.RoleButton, "Proxy"); !ok {
		t.Error("no button called Proxy for assistive technology")
	}
	if !hasTextPart(tt, "Shadowsocks") || hasTextPart(tt, "SHADOWSOCKS") {
		t.Errorf("a sentence was put in capitals: %q", tt.Texts())
	}
	u, dark = skinned(fixtureUI(), "paper")
	tt = ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{Dark: dark})
	tt.Advance(time.Second)
	if !slices.Contains(tt.Texts(), "Proxy") || slices.Contains(tt.Texts(), "PROXY") {
		t.Errorf("纸墨 has no capitals: %q", tt.Texts())
	}
}

// TestTagStyles: a small tag is tinted with its color, or as the skin
// says a solid block of it with text that reads on it, or a hairline box
// around nothing.
func TestTagStyles(t *testing.T) {
	for _, style := range []string{"", "solid", "outline"} {
		p := *palOf("paper", false)
		p.tags = style
		col := p.signal
		tt := ui.NewTesterWith(func(c *ui.Context) {
			ui.Row(c).Padding(40).AlignItems(ui.Start).Background(p.paper).Children(func() { tag(c, &p, "实测", col) })
		}, 200, 120, ui.TesterOptions{})
		tt.Frame()
		r, ok := tt.Find("实测")
		if !ok {
			t.Fatalf("%q: no tag among %q", style, tt.Texts())
		}
		y := int(r.Y + r.H/2)
		in := int(r.X) - 2 // inside the tag's padding
		edged := false     // a line of the color somewhere before the text
		for x := int(r.X) - 9; x < int(r.X); x++ {
			if !near(tt, x, y, p.paper, 60) && !near(tt, x, y, col, 12) {
				edged = true
			}
		}
		switch style {
		case "":
			if near(tt, in, y, p.paper, 3) || near(tt, in, y, col, 60) {
				t.Errorf("tinted: inside is %v", tt.Image().At(in, y))
			}
		case "solid":
			if !near(tt, in, y, col, 12) {
				t.Errorf("solid: inside is %v, not %v", tt.Image().At(in, y), col)
			}
		case "outline":
			if !near(tt, in, y, p.paper, 3) || !edged {
				t.Errorf("outline: inside is %v, a line %v", tt.Image().At(in, y), edged)
			}
		}
	}
}

// anyNear says whether a pixel of the rectangle from x0, y0 to x1, y1 is
// within d of col.
func anyNear(tt *ui.Tester, x0, y0, x1, y1 int, col ui.Color, d int) bool {
	img := tt.Image()
	for x := x0; x < x1; x++ {
		for y := y0; y < y1; y++ {
			if nearIn(img, x, y, col, d) {
				return true
			}
		}
	}
	return false
}

// TestBracketPick: The Machine marks what is chosen with a box of
// brackets in its yellow, not with a fill: the chosen tab is paper inside
// with yellow at its corner, the chosen mode and the exit's name stand in
// a box, and the settings' chosen page is not a yellow row. A skin
// without brackets keeps its filled tab and draws no box.
func TestBracketPick(t *testing.T) {
	for _, id := range []string{"paper", "machine"} {
		u, dark := skinned(fixtureUI(), id)
		p := palOf(id, dark)
		brackets := p.pick == "bracket"
		tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{Dark: dark})
		tt.Advance(time.Second)
		r, ok := tt.Find("代理")
		if !ok {
			t.Fatalf("%s: no tab among %q", id, tt.Texts())
		}
		// Inside the tab, before its word.
		x, y := int(r.X)-8, int(r.Y+r.H/2)
		if filled := near(tt, x, y, p.accent, 30); filled == brackets {
			t.Errorf("%s: inside the chosen tab is %v, filled %v", id, tt.Image().At(x, y), filled)
		}
		// The tab's top left corner, 16 before and 4 above its word.
		if corner := anyNear(tt, int(r.X)-18, int(r.Y)-6, int(r.X)-12, int(r.Y), p.accentLine, 40); brackets && !corner {
			t.Errorf("%s: no bracket at the chosen tab's corner", id)
		}
		r, ok = tt.Find("规则")
		if !ok {
			t.Fatalf("%s: no mode among %q", id, tt.Texts())
		}
		// Before the chosen mode's word, where its box begins.
		if box := anyNear(tt, int(r.X)-12, int(r.Y)-8, int(r.X), int(r.Y+r.H/2), p.accentLine, 40); box != brackets {
			t.Errorf("%s: a box round the chosen mode %v", id, box)
		}
		r, ok = tt.Find("香港 03 IPLC")
		if !ok {
			t.Fatalf("%s: no exit among %q", id, tt.Texts())
		}
		// Before the flag, where the box round the exit's name begins.
		if box := anyNear(tt, int(r.X)-75, int(r.Y)-8, int(r.X)-45, int(r.Y+r.H)+8, p.accentLine, 40); box != brackets {
			t.Errorf("%s: a box before the exit's flag %v", id, box)
		}
	}
	u, dark := skinned(fixtureUI(), "machine")
	p := palOf("machine", dark)
	tt := ui.NewTesterWith(u.settingsView, 760, 760, ui.TesterOptions{Dark: dark})
	tt.Advance(time.Second)
	r, ok := tt.Find("通用")
	if !ok {
		t.Fatalf("no 通用 among %q", tt.Texts())
	}
	if x, y := int(r.X)-29, int(r.Y+r.H/2); near(tt, x, y, p.accent, 60) {
		t.Errorf("the settings' chosen page is a filled row: %v", tt.Image().At(x, y))
	}
	if !anyNear(tt, int(r.X)-26, int(r.Y)-2, int(r.X)-6, int(r.Y+r.H)+2, p.accentLine, 60) {
		t.Error("the chosen page's icon is not in the theme color")
	}
}

// TestBlockHero: Samaritan stands the exit's name on a block of its red,
// which hugs the flag and the name; the tab stays filled.
func TestBlockHero(t *testing.T) {
	for _, id := range []string{"paper", "samaritan"} {
		u, dark := skinned(fixtureUI(), id)
		p := palOf(id, dark)
		tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{Dark: dark})
		tt.Advance(time.Second)
		r, ok := tt.Find("香港 03 IPLC")
		if !ok {
			t.Fatalf("%s: no exit among %q", id, tt.Texts())
		}
		// Between the flag and the name.
		x, y := int(r.X)-4, int(r.Y+r.H/2)
		if block := near(tt, x, y, p.accent, 30); block != (p.pick == "block") {
			t.Errorf("%s: beside the exit's name is %v, a block %v", id, tt.Image().At(x, y), block)
		}
		// The block ends with the name, which keeps to one line in it: the
		// far side of the cell is paper.
		if r.H > 70 {
			t.Errorf("%s: the exit's name is %v high, on two lines", id, r.H)
		}
		if x := int(r.X+r.W) + 14; x < 398 && near(tt, x, y, p.accent, 30) {
			t.Errorf("%s: the block runs on past the name to %d", id, x)
		}
		r, _ = tt.Find("代理")
		if x, y := int(r.X)-8, int(r.Y+r.H/2); !near(tt, x, y, p.accent, 30) {
			t.Errorf("%s: the chosen tab is not filled: %v", id, tt.Image().At(x, y))
		}
	}
}

// TestMapMarkers: the map marks its exits as the skin says. A ring, as
// ever, has none of the theme color; a box puts brackets in it round the
// exit in use; a cross stands that exit's name on a block of it.
func TestMapMarkers(t *testing.T) {
	k := skinByID("paper")
	defer func(m string) { k.light.marker = m }(k.light.marker)
	count := func(marker string) int {
		k.light.marker = marker
		u, _ := skinned(fixtureUI(), "paper")
		p := palOf("paper", false)
		tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{Preferences: ui.Preferences{ReduceMotion: true}})
		tt.Advance(time.Second)
		img, n := tt.Image(), 0
		for x := 402; x < 1058; x++ {
			for y := 52; y < 376; y++ {
				if nearIn(img, x, y, p.accentLine, 12) {
					n++
				}
			}
		}
		return n
	}
	ring, box, cross := count(""), count("box"), count("cross")
	if ring != 0 {
		t.Errorf("rings: %d pixels of the theme color on the map", ring)
	}
	if box < 8 {
		t.Errorf("boxes: %d pixels of the theme color, no brackets round the exit in use", box)
	}
	if cross < 150 || cross <= box {
		t.Errorf("crosses: %d pixels of the theme color, no block under the exit in use (boxes: %d)", cross, box)
	}
}

// TestMonoSkinsFit: a monospace face is wider than the one the layout
// was drawn for, and capitals wider again. In the skins that have them
// the settings' sidebar still holds "Developer networks", the empty
// provider page's "Subscription" stays on one line, the three buttons
// under the exit's name stay inside their cell, and the log's route
// filters go to a second line sooner than be cut off.
func TestMonoSkinsFit(t *testing.T) {
	defer i18n.Set("zh")
	for _, id := range []string{"phosphor", "mrrobot", "machine", "samaritan"} {
		i18n.Set("en")
		u, dark := skinned(fixtureUI(), id)
		p := palOf(id, dark)
		opts := ui.TesterOptions{Dark: dark}

		tt := ui.NewTesterWith(u.settingsView, 760, 760, opts)
		tt.Advance(time.Second)
		r, ok := tt.Find("Developer networks")
		if !ok {
			t.Fatalf("%s: no Developer networks among %q", id, tt.Texts())
		}
		if need, _ := ui.ShapeText("Developer networks", ui.Font{Family: p.font, Size: 13, Weight: 400}, 0).Size(); need > r.W+1 {
			t.Errorf("%s: Developer networks needs %.0f and the sidebar gives it %.0f", id, need, r.W)
		}

		u, _ = skinned(providersUIFixture(), id)
		u.snap.Providers, u.snap.Nodes = nil, nil
		tt = ui.NewTesterWith(u.view, 1060, 760, opts)
		tt.Advance(time.Second)
		if r, ok = tt.Find("Subscription"); !ok {
			t.Fatalf("%s: no Subscription among %q", id, tt.Texts())
		}
		if r.H > 32 {
			t.Errorf("%s: Subscription is %.0f high, broken over two lines", id, r.H)
		}

		u, _ = skinned(fixtureUI(), id)
		tt = ui.NewTesterWith(u.view, 1060, 760, opts)
		tt.Advance(time.Second)
		// The last of the three buttons; the mode's word is drawn larger.
		direct := ui.Rect{}
		for _, at := range []string{p.cap("Direct")} {
			if r, ok = tt.Find(at); ok && r.H < 24 {
				direct = r
			}
		}
		// The cell is 400 wide and padded by 26; the button by 10 or more.
		if direct.W == 0 || direct.X+direct.W+10 > 374+1 {
			t.Errorf("%s: the Direct button's word ends at %.0f, past its cell's padding", id, direct.X+direct.W)
		}

		// English has the longer words before the filters; the routes'
		// names are the fixture's in either.
		for _, lang := range []string{"en", "zh"} {
			i18n.Set(lang)
			u, _ = skinned(actUI(), id)
			tt = ui.NewTesterWith(u.view, 1060, 760, opts)
			tt.Advance(time.Second)
			// Find gives what shows of a text: two characters at 12 are
			// 24 wide, and less is a filter cut off.
			for _, name := range []string{"直连", "拦截"} {
				if chip, ok := tt.Find(name); !ok || chip.W < 23 {
					t.Errorf("%s, %s: the route filter %s shows %.0f wide", id, lang, name, chip.W)
				}
			}
		}
	}
}

func TestSkinShots(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	for _, k := range skins {
		for _, dark := range []bool{false, true} {
			if f := k.fixed(); f != "" && dark != (f == "dark") {
				continue
			}
			suffix := map[bool]string{false: "", true: "-dark"}[dark]
			u := fixtureUI()
			u.snap.Settings = settings.Defaults.Clone()
			skinPick(&u.snap.Settings, k.id)
			tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{Dark: dark})
			tt.Advance(time.Second)
			shot(t, tt, "skin-"+k.id+suffix)
			u.home.x = "routes"
			tt.Frame()
			tt.Advance(2 * time.Second)
			shot(t, tt, "skin-"+k.id+"-routes"+suffix)
			st := ui.NewTesterWith(u.settingsView, 760, 760, ui.TesterOptions{Dark: dark})
			st.Advance(time.Second)
			shot(t, st, "skin-"+k.id+"-settings"+suffix)
		}
	}
}
