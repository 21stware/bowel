package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/i18n"
)

// appPicker is what the rule editor offers for a rule of kind 应用: the
// applications running now and those with connections, each a click away,
// so that no one has to know a program's name or wait for it in the Log.
type appPicker struct {
	apps  []appChoice
	at    time.Time // when apps was listed; zero: never
	busy  bool      // a listing is under way
	icons map[string]*ui.Bitmap
	tried map[string]bool // bundles whose icon was looked for
}

// appsEvery is how old the list may get while the editor shows it.
const appsEvery = 5 * time.Second

// loadApps lists the applications again, off the window's thread, when the
// list is older than appsEvery and none is under way. Icons are read once
// a bundle.
func (u *UI) loadApps() {
	pk := &u.routes.picker
	if u.app == nil || u.win == nil || pk.busy || !pk.at.IsZero() && time.Since(pk.at) < appsEvery {
		return
	}
	pk.busy = true
	tried := make(map[string]bool, len(pk.tried))
	for b := range pk.tried {
		tried[b] = true
	}
	go func() {
		apps := u.app.runningApps()
		icons := map[string]image.Image{}
		for _, a := range apps {
			if a.Bundle == "" || tried[a.Bundle] {
				continue
			}
			tried[a.Bundle] = true
			if b := bundleIcon(a.Bundle, 36); b != nil {
				if img, err := png.Decode(bytes.NewReader(b)); err == nil {
					icons[a.Bundle] = img
				}
			}
		}
		u.win.Update(func() {
			pk.apps, pk.at, pk.busy, pk.tried = apps, time.Now(), false, tried
			if pk.icons == nil {
				pk.icons = map[string]*ui.Bitmap{}
			}
			for b, img := range icons {
				pk.icons[b] = ui.NewBitmap(img)
			}
		})
	}()
}

// pickable are the applications shown for what is typed: those whose name
// has it, or all of them when it is the name of one already, so that a
// choice made can be changed.
func pickable(apps []appChoice, typed string) []appChoice {
	q := strings.ToLower(strings.TrimSpace(typed))
	if q == "" || slices.ContainsFunc(apps, func(a appChoice) bool { return strings.ToLower(a.Name) == q }) {
		return apps
	}
	var out []appChoice
	for _, a := range apps {
		if strings.Contains(strings.ToLower(a.Name), q) {
			out = append(out, a)
		}
	}
	return out
}

// appPickerView offers the applications under the new rule: those with
// connections, then the others running. A click puts the name in the rule.
func (u *UI) appPickerView(c *ui.Context, p *pal) {
	st := &u.routes
	pk := &st.picker
	u.loadApps()
	if pk.at.IsZero() {
		if pk.busy {
			ui.Text(c, i18n.T("正在列出运行中的应用…", "Listing the running apps…")).FontSize(11.5).TextColor(p.faint)
		}
		return
	}
	shown := pickable(pk.apps, st.rule)
	if len(shown) == 0 {
		if q := strings.TrimSpace(st.rule); q != "" && len(pk.apps) > 0 {
			ui.Text(c, fmt.Sprintf(i18n.T("没有名字含「%s」的应用在运行，可以照样添加。", "No running app has “%s” in its name; it can be added all the same."), q)).
				FontSize(11.5).TextColor(p.faint)
		}
		return
	}
	i := slices.IndexFunc(shown, func(a appChoice) bool { return a.Conns == 0 })
	if i < 0 {
		i = len(shown)
	}
	ui.Scroll(c.Key("apps")).MaxHeight(176).Gap(6).Padding(2, 0, 2, 0).Children(func() {
		section := func(key, title string, apps []appChoice) {
			if len(apps) == 0 {
				return
			}
			ui.Text(c, title).FontSize(11).FontWeight(600).TextColor(p.mute)
			ui.Row(c.Key(key)).Wrap().GapX(6).GapY(6).Children(func() {
				for _, a := range apps {
					u.appChip(c, p, a)
				}
			})
		}
		section("with-conns", i18n.T("有连接的应用", "Apps with connections"), shown[:i])
		section("running", i18n.T("正在运行的应用", "Running apps"), shown[i:])
	})
}

// appChip is one application to pick: its icon (or its initial) and name.
func (u *UI) appChip(c *ui.Context, p *pal, a appChoice) {
	st := &u.routes
	on := strings.EqualFold(strings.TrimSpace(st.rule), a.Name)
	label := a.Name
	if a.Conns > 0 {
		label = fmt.Sprintf(i18n.T("%s，%d 个连接", "%s, %d connections"), a.Name, a.Conns)
	}
	b := ui.ButtonBase(c.Key("app:"+strings.ToLower(a.Name))).Padding(3, 10, 3, 4).Radius(p.r(99)).Border(1, p.hair).
		Shrink(0).MaxWidth(240).Label(label)
	switch {
	case on:
		b.Background(p.hair2).BorderColor(p.ink)
	case b.Hovered():
		b.Background(p.hover)
	}
	b.OnClick(func() {
		st.rule, st.ruleErr, st.conflict, st.focusRule = a.Name, "", nil, true
	}).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).MinWidth(0).Children(func() {
			if bmp := st.picker.icons[a.Bundle]; bmp != nil {
				ui.Image(c, bmp).Size(18, 18).Shrink(0).AccessHidden(true)
			} else {
				r, _ := utf8.DecodeRuneInString(strings.TrimSuffix(a.Name, ".app"))
				ui.Box(c).Size(18, 18).Shrink(0).Radius(p.r(5)).Background(p.hair2).Justify(ui.Center).AlignItems(ui.Center).
					AccessHidden(true).Children(func() {
					ui.Text(c, strings.ToUpper(string(r))).FontSize(10).FontWeight(600).TextColor(p.ink2)
				})
			}
			col, w := p.ink2, 500
			if on {
				col, w = p.ink, 600
			}
			ui.Text(c, displayApp(a.Name)).FontSize(12).FontWeight(w).TextColor(col).SingleLine().MinWidth(0)
		})
	})
}

// displayApp is an application's name as a person knows it: a macOS
// bundle without ".app". The rule keeps the whole name.
func displayApp(name string) string {
	if runtime.GOOS == "darwin" && len(name) > 4 && strings.EqualFold(name[len(name)-4:], ".app") {
		return name[:len(name)-4]
	}
	return name
}
