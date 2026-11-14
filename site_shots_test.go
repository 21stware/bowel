package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/settings"
)

// TestSiteShots renders the screenshots the README and the website show, at
// 2× for Retina screens, in each of the interface's languages, when BOWEL_SITE_SHOTS names
// a directory:
//
//	BOWEL_SITE_SHOTS=site/shots CGO_ENABLED=0 go test -run SiteShots .
//
// scripts/site-shots.sh runs it and compresses the PNGs.
func TestSiteShots(t *testing.T) {
	dir := os.Getenv("BOWEL_SITE_SHOTS")
	if dir == "" {
		t.Skip("BOWEL_SITE_SHOTS not set")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	save := func(tt *ui.Tester, name string) {
		t.Helper()
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	render := func(view func(*ui.Context), w, h int, dark bool, settle func(*ui.Tester)) *ui.Tester {
		// A still picture: what blinks holds on.
		tt := ui.NewTesterWith(view, w, h, ui.TesterOptions{Dark: dark, Preferences: ui.Preferences{ReduceMotion: true}})
		tt.SetScale(2)
		tt.Advance(time.Second)
		if settle != nil {
			settle(tt)
		}
		return tt
	}
	defer i18n.Set("zh")
	defer func(f func() string) { mcpCommand = f }(mcpCommand)
	mcpCommand = func() string { return "/Applications/bowel.app/Contents/MacOS/bowel" }
	for _, lang := range i18n.Languages {
		i18n.Set(lang)
		for _, dark := range []bool{false, true} {
			sfx := "-" + lang
			if dark {
				sfx += "-dark"
			}

			u := fixtureUI()
			save(render(u.view, 1060, 760, dark, nil), "home"+sfx)

			u = fixtureUI()
			save(render(u.view, 1060, 760, dark, func(tt *ui.Tester) {
				u.home.x, u.home.sub = "routes", "ai"
				tt.Frame()
				tt.Advance(2 * time.Second)
			}), "routes"+sfx)

			u = fixtureUI()
			save(render(u.view, 1060, 760, dark, func(tt *ui.Tester) {
				u.home.x = "map"
				tt.Frame()
				tt.Advance(2 * time.Second)
			}), "map"+sfx)

			u = actUI()
			save(render(u.view, 1060, 760, dark, nil), "activity"+sfx)

			u = providersUIFixture()
			save(render(u.view, 1060, 760, dark, nil), "providers"+sfx)

			u = fixtureUI()
			save(render(u.panelView, 320, 490, dark, nil), "panel"+sfx)

			for _, sec := range []string{"rules", "mcp"} {
				u = fixtureUI()
				u.set.section = sec
				save(render(u.settingsView, 760, 760, dark, nil), "settings-"+sec+sfx)
			}
		}

		for _, k := range skins {
			dark := k.fixed() == "dark"
			u := fixtureUI()
			u.snap.Settings = settings.Defaults.Clone()
			skinPick(&u.snap.Settings, k.id)
			save(render(u.view, 1060, 760, dark, nil), "skin-"+k.id+"-"+lang)
		}
	}
}
