package main

import (
	"bytes"
	"image/color"
	"image/png"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/settings"
)

func TestFmtRateShort(t *testing.T) {
	for b, want := range map[int64]string{
		-1:         "0B",
		0:          "0B",
		512:        "512B",
		999:        "999B",
		1000:       "1.0K",
		1536:       "1.5K",
		60 << 10:   "60K",
		999 << 10:  "999K",
		1000 << 10: "1.0M",
		1363149:    "1.3M",
		10 << 20:   "10M",
		2254857830: "2.1G",
		5 << 40:    "5.0T",
		5000 << 40: "5000T",
		10443817:   "10M", // 9.96 MB: not "10.0M"
	} {
		if got := fmtRateShort(b); got != want {
			t.Errorf("fmtRateShort(%d) = %q, want %q", b, got, want)
		}
	}
}

// The text beside the icon keeps its length whatever the rates are, so
// that the menu bar's other items don't move.
func TestTrayTextWidth(t *testing.T) {
	want := -1
	for _, b := range []int64{0, 7, 512, 1000, 60 << 10, 999 << 10, 1363149, 300 << 20, 2254857830} {
		title, tip := trayText("darwin", true, Tick{Down: b, Up: b / 3})
		if tip != trayName {
			t.Errorf("tooltip %q beside a title", tip)
		}
		n := utf8.RuneCountInString(title)
		if want < 0 {
			want = n
		}
		if n != want {
			t.Errorf("%q is %d characters, others are %d", title, n, want)
		}
	}
}

func TestTrayText(t *testing.T) {
	tick := Tick{Down: 1363149, Up: 60 << 10}
	for _, c := range []struct {
		goos       string
		show       bool
		title, tip string
	}{
		{"darwin", false, "", "bowel"},
		{"windows", false, "", "bowel"},
		{"linux", false, "", "bowel"},
		{"darwin", true, "↓1.3M ↑ 60K", "bowel"},
		{"linux", true, "↓1.3M ↑ 60K", "bowel"},
		// No text beside a tray icon on Windows: the tooltip tells.
		{"windows", true, "", "bowel · ↓ 1.30 MB/s  ↑ 60.0 KB/s"},
	} {
		title, tip := trayText(c.goos, c.show, tick)
		if title != c.title || tip != c.tip {
			t.Errorf("trayText(%s, %v) = %q, %q; want %q, %q", c.goos, c.show, title, tip, c.title, c.tip)
		}
	}
}

// The rates beside the icon are off unless asked for, are the app's own
// business, and cost polls only while they show.
// Not taken over, the tray offers the nodes only with traffic through the
// port.
func TestTrayNodes(t *testing.T) {
	for _, c := range []struct {
		on   bool
		tick Tick
		want bool
	}{
		{true, Tick{}, true},
		{false, Tick{}, false},
		{false, Tick{Active: 1}, true},
		{false, Tick{Down: 512}, true},
	} {
		if got := trayNodes(c.on, c.tick); got != c.want {
			t.Errorf("trayNodes(%v, %+v) = %v", c.on, c.tick, got)
		}
	}
}

func TestTraySpeedSetting(t *testing.T) {
	if settings.Defaults.TraySpeed {
		t.Fatal("the menu bar shows the speed by default")
	}
	old := settings.Defaults.Clone()
	next := old.Clone()
	next.TraySpeed = true
	if !appOnly(old, next) {
		t.Error("showing the speed in the menu bar reloads the core")
	}

	st, err := settings.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := &App{settings: st}
	set := func(fn func(*settings.Settings)) {
		t.Helper()
		if _, err := st.Update(fn); err != nil {
			t.Fatal(err)
		}
	}
	set(func(s *settings.Settings) { s.SystemProxy, s.Paused, s.TraySpeed = true, false, false })
	if got := a.pollEvery(); got != 5*time.Second {
		t.Errorf("polls every %v with nothing shown", got)
	}
	set(func(s *settings.Settings) { s.TraySpeed = true })
	if got := a.pollEvery(); got != traySpeedEvery {
		t.Errorf("polls every %v with the speed in the menu bar", got)
	}
	set(func(s *settings.Settings) { s.Paused = true })
	if got := a.pollEvery(); got != 5*time.Second {
		t.Errorf("polls every %v with the speed asked for but nothing taken over", got)
	}
	// Not taken over, an app pointed at the port still has rates to tell.
	a.lastTick = Tick{Active: 1}
	if got := a.pollEvery(); got != traySpeedEvery {
		t.Errorf("polls every %v with the speed asked for and a connection through the port", got)
	}
	a.lastTick = Tick{}
	set(func(s *settings.Settings) { s.Paused = false })
	a.winShown.Store(true)
	if got := a.pollEvery(); got != time.Second {
		t.Errorf("polls every %v with the window shown", got)
	}
}

// The icon is black only where the system recolours it (macOS's template);
// GNOME's dark top bar showed the black one as nothing (#161).
func TestTrayWhite(t *testing.T) {
	for _, c := range []struct {
		goos  string
		light bool
		want  bool
	}{
		{"darwin", false, false},
		{"darwin", true, false},
		{"linux", false, true},
		{"linux", true, true},
		{"freebsd", false, true},
		{"windows", false, true},
		{"windows", true, false},
	} {
		if got := trayWhite(c.goos, c.light); got != c.want {
			t.Errorf("trayWhite(%q, taskbar light %v) = %v, want %v", c.goos, c.light, got, c.want)
		}
	}
}

// The ink is the colour of every pixel drawn; off fades it, not darkens it.
func TestTrayIconInk(t *testing.T) {
	for _, white := range []bool{false, true} {
		var want uint8
		if white {
			want = 255
		}
		for _, on := range []bool{false, true} {
			img, err := png.Decode(bytes.NewReader(trayIcon(on, white)))
			if err != nil {
				t.Fatal(err)
			}
			var maxA uint8
			b := img.Bounds()
			for y := b.Min.Y; y < b.Max.Y; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
					if c.A == 0 {
						continue
					}
					if c.R != want || c.G != want || c.B != want {
						t.Fatalf("white %v on %v: pixel (%d,%d) is %v", white, on, x, y, c)
					}
					maxA = max(maxA, c.A)
				}
			}
			if on && maxA != 255 || !on && (maxA == 0 || maxA > 110) {
				t.Errorf("white %v on %v: strongest alpha %d", white, on, maxA)
			}
		}
	}
}

func TestTrayMenuControls(t *testing.T) {
	a := &App{}
	find := func(items []*mygo.MenuItem, label string) *mygo.MenuItem {
		t.Helper()
		for _, it := range items {
			if it.Label == label {
				return it
			}
		}
		t.Fatalf("menu item %q not found", label)
		return nil
	}

	// Case 1: Takeover on with System proxy on, TUN off, and 1 of 2 providers enabled.
	m1 := a.trayMenu(Snapshot{
		Takeover: Takeover{
			On:     true,
			System: Entry{Wanted: true},
			Tun:    Entry{Wanted: false},
		},
		Providers: []ProviderView{
			{ID: "p1", Name: "Sub A", Enabled: true},
			{ID: "p2", Name: "Sub B", Enabled: false},
		},
	})
	takeover := find(m1.Items(), i18n.T("接管", "Takeover"))
	sys := find(m1.Items(), i18n.T("系统代理", "System proxy"))
	tun := find(m1.Items(), "TUN")
	if takeover.Type != mygo.MenuItemCheckbox || !takeover.Checked {
		t.Errorf("takeover = %+v, want checked checkbox", takeover)
	}
	if sys.Type != mygo.MenuItemCheckbox || !sys.Checked {
		t.Errorf("system proxy = %+v, want checked checkbox", sys)
	}
	if tun.Type != mygo.MenuItemCheckbox || tun.Checked {
		t.Errorf("TUN = %+v, want unchecked checkbox", tun)
	}
	sub := find(m1.Items(), i18n.T("订阅", "Subscription")+"\tSub A")
	if sub.Disabled {
		t.Error("subscription menu unexpectedly disabled")
	}
	if len(sub.Submenu) != 2 {
		t.Fatalf("subscription submenu has %d items, want 2", len(sub.Submenu))
	}
	if sub.Submenu[0].Type != mygo.MenuItemRadio || sub.Submenu[0].Label != "Sub A" || !sub.Submenu[0].Checked {
		t.Errorf("submenu[0] = %+v", sub.Submenu[0])
	}
	if sub.Submenu[1].Type != mygo.MenuItemRadio || sub.Submenu[1].Label != "Sub B" || sub.Submenu[1].Checked {
		t.Errorf("submenu[1] = %+v", sub.Submenu[1])
	}

	// Case 2: Paused (Takeover.On false even though entries are wanted), and no providers.
	m2 := a.trayMenu(Snapshot{
		Takeover: Takeover{
			On:     false,
			System: Entry{Wanted: true},
			Tun:    Entry{Wanted: true},
		},
	})
	if find(m2.Items(), i18n.T("接管", "Takeover")).Checked {
		t.Error("paused takeover should be unchecked")
	}
	if find(m2.Items(), i18n.T("系统代理", "System proxy")).Checked {
		t.Error("system proxy should appear unchecked while takeover is off")
	}
	if find(m2.Items(), "TUN").Checked {
		t.Error("TUN should appear unchecked while takeover is off")
	}
	subEmpty := find(m2.Items(), i18n.T("订阅", "Subscription"))
	if !subEmpty.Disabled || len(subEmpty.Submenu) != 0 {
		t.Errorf("empty subscription menu = %+v, want disabled and empty", subEmpty)
	}

	// Case 3: Multiple providers enabled omits single active name suffix.
	m3 := a.trayMenu(Snapshot{
		Providers: []ProviderView{
			{ID: "p1", Name: "Sub A", Enabled: true},
			{ID: "p2", Name: "Sub B", Enabled: true},
		},
	})
	subMulti := find(m3.Items(), i18n.T("订阅", "Subscription"))
	if subMulti.Disabled || len(subMulti.Submenu) != 2 || !subMulti.Submenu[0].Checked || !subMulti.Submenu[1].Checked {
		t.Errorf("multi-enabled subscription menu = %+v", subMulti)
	}
}
