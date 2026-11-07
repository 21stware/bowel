package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/settings"
)

// TestAccent: text on the accent is whichever reads, a pale accent
// leaves lines to ink, and each preset has a color that reads on the
// paper it is drawn on.
func TestAccent(t *testing.T) {
	for _, a := range accentPresets {
		if !settings.ValidAccent(a.hex) {
			t.Errorf("preset %s is not a valid setting", a.hex)
		}
	}
	p := lightPal
	p.setAccent("#ECFF8A") // typed in, drawn as it is
	if p.accent != ui.Hex("#ECFF8A") || p.onAccent != ui.Hex("#171615") || p.accentLine != p.ink {
		t.Errorf("pale: %v, text %v, line %v", p.accent, p.onAccent, p.accentLine)
	}
	p.setAccent("#DDFB4C")
	if p.accent != ui.Hex("#7A9E00") || p.accentLine != p.accent {
		t.Errorf("lime on paper: %v, line %v", p.accent, p.accentLine)
	}
	d := darkPal
	d.setAccent("#DDFB4C")
	if d.accent != ui.Hex("#DDFB4C") {
		t.Errorf("lime on graphite: %v", d.accent)
	}
	for _, a := range accentPresets {
		if a.hex == "ink" {
			continue
		}
		for _, base := range []pal{lightPal, darkPal} {
			q := base
			q.setAccent(a.hex)
			if c := contrast(q.accent, q.paper); c < 2.5 {
				t.Errorf("%s (dark %v): %v is %.2f:1 on its paper", a.hex, q.dark, q.accent, c)
			}
			if c := contrast(q.onAccent, q.accent); c < 3 {
				t.Errorf("%s (dark %v): text on it is %.2f:1", a.hex, q.dark, c)
			}
		}
	}
	p.setAccent("#4B3BF5")
	if p.onAccent != ui.Hex("#ffffff") || p.accentLine != p.accent {
		t.Errorf("indigo: text %v, line %v", p.onAccent, p.accentLine)
	}
	p.setAccent("ink")
	if p.accent != p.ink || p.onAccent != p.paper {
		t.Errorf("ink: %v on %v", p.onAccent, p.accent)
	}
	for _, bad := range []string{"", "DDFB4C", "#DDFB4", "#GGGGGG", "lime"} {
		if settings.ValidAccent(bad) {
			t.Errorf("%q passes as a theme color", bad)
		}
	}
}

func TestAccentShots(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	for _, a := range []string{"#4B3BF5", "#FF6A3D"} {
		u := fixtureUI()
		u.snap.Settings.Accent = a
		tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
		tt.Frame()
		shot(t, tt, "accent-"+a[1:])
	}
}
