package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/settings"
)

// TestAccent: text on the accent is whichever reads, and a pale accent
// leaves lines to ink.
func TestAccent(t *testing.T) {
	for _, a := range accentPresets {
		if !settings.ValidAccent(a.hex) {
			t.Errorf("preset %s is not a valid setting", a.hex)
		}
	}
	p := lightPal
	p.setAccent("#DDFB4C")
	if p.onAccent != ui.Hex("#171615") || p.accentLine != p.ink {
		t.Errorf("lime: text %v, line %v", p.onAccent, p.accentLine)
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
