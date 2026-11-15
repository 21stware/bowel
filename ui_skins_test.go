package main

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

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
	r, g, b, _ := tt.Image().At(x, y).RGBA()
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
