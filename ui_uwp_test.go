package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/settings"
	"github.com/oboo/moho/internal/uwploop"
)

// TestSettingsUWP: on Windows, 入站 › 系统代理 has the option that lets
// UWP apps reach the proxy (issue #320), with the count and the buttons
// that fit it; elsewhere there is no such option.
func TestSettingsUWP(t *testing.T) {
	defer i18n.Set("zh")
	i18n.Set("zh")
	u := fixtureUI()
	u.snap.Settings = settings.Defaults.Clone()
	u.set.section = "inbound"
	tt := ui.NewTesterWith(u.settingsView, 760, 1200, ui.TesterOptions{})
	tt.Advance(time.Second)
	if !uwploop.Supported {
		if tt.HasText("允许 UWP 应用使用代理") {
			t.Fatal("the UWP option shows where there are no UWP apps")
		}
		return
	}
	cases := []struct {
		st          uwploop.Status
		text        string
		allow, undo bool
	}{
		{uwploop.Status{Total: 97}, "（已允许 0 / 97 个）", true, false},
		{uwploop.Status{Total: 97, Exempt: 40, Added: 3}, "（已允许 40 / 97 个）", true, true},
		{uwploop.Status{Total: 97, Exempt: 97, Added: 97}, "这台电脑上的 97 个 UWP 应用都可以使用代理", false, true},
	}
	for _, c := range cases {
		st := c.st
		u.set.uwp = &st
		tt.Frame()
		if !tt.HasText("允许 UWP 应用使用代理") || !tt.HasText(c.text) {
			t.Errorf("%+v: %q", st, tt.Texts())
		}
		if got := tt.HasText("全部允许"); got != c.allow {
			t.Errorf("%+v: Allow all shown %v", st, got)
		}
		if got := tt.HasText("撤销"); got != c.undo {
			t.Errorf("%+v: Undo shown %v", st, got)
		}
	}
	shot(t, tt, "uwp-settings")
}
