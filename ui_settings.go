package main

// 设置: Moho's and the core's options, in a window of their own. Each change is
// checked, saved and applied at once (the core reloads its config); there
// is no Save button. While a change is on its way the window shows it as
// made, and goes back if the core refuses it.
//
// The window draws from the same UI as the main one: u.snap arrives
// through the main window's Update, on the main thread, and setSnapshot
// redraws this window after it.

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/settings"
)

type settingsUI struct {
	// win is the open settings window, nil when closed. Set and cleared on
	// the main thread, read from the goroutines that redraw it.
	win atomic.Pointer[mygo.Window]
	// over is the settings as the window shows them while changes are
	// applied; nil shows the snapshot's. seq tells the last change's end.
	over   *settings.Settings
	seq    int
	fields map[string]*optField
	toasts []string
	// section is the page the sidebar has chosen.
	section string
	// busy lists the actions running, by their button's key.
	busy map[string]bool
	// rule is 规则's typing.
	rule ruleEdit
	// opened counts the times a test's UI, which has no app and so no
	// windows, was asked to open the window.
	opened int
}

// openSettings brings up the settings window, opening it the first time.
// On the main thread.
func (u *UI) openSettings() {
	if w := u.set.win.Load(); w != nil {
		w.Show()
		w.Focus()
		return
	}
	if u.app == nil {
		u.set.opened++
		return
	}
	w := mygo.NewWindow(mygo.WindowOptions{
		Title: i18n.T("设置", "Settings"), Width: 760, Height: 560, MinWidth: 680, MinHeight: 440,
		TitleBarStyle: mygo.TitleBarHidden, StateKey: "settings",
		TrafficLightPosition: &mygo.Point{X: 18, Y: 18},
		BackgroundColor:      "light-dark(#f6f4ef, #171615)",
		DisableMinimize:      true, DisableFullScreen: true,
		Content: ui.View(u.settingsView),
	})
	u.set.win.Store(w)
	w.OnClosed(func() {
		u.set.win.CompareAndSwap(w, nil)
		u.set.fields = nil // edits not kept are dropped with the window
		u.app.syncDock()
	})
	u.app.syncDock()
	w.Focus()
	mygo.App.Focus()
}

// settingsRedraw redraws the open settings window. Safe from any goroutine.
func (u *UI) settingsRedraw() {
	if w := u.set.win.Load(); w != nil {
		w.Invalidate()
	}
}

// settingsUpdate runs fn on the main thread and redraws the settings window.
func (u *UI) settingsUpdate(fn func()) {
	if w := u.set.win.Load(); w != nil {
		w.Update(fn)
	} else if u.win != nil {
		u.win.Update(fn)
	}
}

func (u *UI) settingsToast(msg string) {
	u.settingsUpdate(func() { u.set.toasts = append(u.set.toasts, msg) })
}

// optField is a text option's edit: what is typed, and its focus; for a
// list, its entries and the saved ones they started from.
type optField struct {
	text       string
	h          ui.Handle
	list, base []string
}

func (s *settingsUI) field(key string) *optField {
	if s.fields == nil {
		s.fields = map[string]*optField{}
	}
	f := s.fields[key]
	if f == nil {
		f = &optField{}
		s.fields[key] = f
	}
	return f
}

// shown is the settings the page shows.
func (u *UI) shownSettings() settings.Settings {
	if u.set.over != nil {
		return *u.set.over
	}
	return u.snap.Settings
}

// setOpt changes an option: at once on the page, and in the app. When the
// app is done the page shows the settings as saved, kept or not.
func (u *UI) setOpt(fn func(*settings.Settings)) { u.setOptThen(fn, nil) }

// setOptThen is setOpt, and then, off the main thread and once the change
// is saved and applied, after.
func (u *UI) setOptThen(fn func(*settings.Settings), after func()) {
	next := u.shownSettings().Clone()
	fn(&next)
	u.set.over = &next
	u.set.seq++
	if u.app == nil { // a test's UI, with no app behind it
		return
	}
	seq := u.set.seq
	go func() {
		err := u.app.updateSettings(fn)
		if err == nil && after != nil {
			after()
		}
		saved := u.app.settings.Get()
		u.settingsUpdate(func() {
			u.snap.Settings = saved
			if u.set.seq == seq {
				u.set.over = nil
			}
			if err != nil {
				u.set.toasts = append(u.set.toasts, err.Error())
			}
		})
	}()
}

// gear is the settings icon (Lucide's, ISC licence).
var gear = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/></svg>`))

// settingsButton is the title bar's last control: a gear and its name. It
// opens the settings window.
func (u *UI) settingsButton(c *ui.Context, p *pal) {
	b := ui.ButtonBase(c.Key("settings")).Padding(4, 11).Radius(7).Shrink(0).Tooltip(i18n.T("设置", "Settings") + " ⌘,")
	col := p.ink2
	if b.Hovered() {
		b.Background(p.hair2)
	}
	b.OnClick(u.openSettings).Children(func() {
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, gear).Size(14, 14).TextColor(col)
			ui.Text(c, i18n.T("设置", "Settings")).FontSize(13).FontWeight(600).TextColor(col)
		})
	})
}

// ---- The window ----

func (u *UI) settingsView(c *ui.Context) {
	useAccent(u.shownSettings().Accent)
	p := palette(c)
	t := *c.Theme()
	t.Background, t.Text, t.TextMuted, t.Border = p.paper, p.ink, p.mute, p.hair
	t.Surface, t.SurfaceHover = p.card, p.paper2
	accentTheme(&t, p)
	t.Inverse, t.InverseText = p.ink, p.paper
	c.SetTheme(&t)
	c.Root().CaretColor(p.ink)
	for _, m := range u.set.toasts {
		c.Toast(m)
	}
	u.set.toasts = nil
	c.OnShortcut(ui.Cmd, ui.KeyW, func() {
		if w := u.set.win.Load(); w != nil {
			w.Close()
		}
	})
	sec := u.settingsSection()
	ui.Row(c).Absolute().Fill().AlignItems(ui.Stretch).Background(p.paper).Children(func() {
		u.settingsSidebar(c, p)
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
			// The pane's title, in the title bar, which it drags by.
			ui.Row(c).Height(max(c.TitleBar().Height, 50)).Shrink(0).Padding(0, 26).Gap(12).AlignItems(ui.Center).DragWindow().Children(func() {
				ui.Text(c, sec.name()).FontSize(17).FontWeight(700).TextColor(p.ink).Grow(1)
				if busy := u.pending("settings"); busy {
					shimmer(ui.Text(c, i18n.T("正在应用…", "Applying…")).FontSize(12).FontWeight(500).TextColor(p.mute), busy, p)
				}
			})
			ui.Scroll(c.Key(sec.id)).Grow(1).Padding(0, 26, 24, 26).Children(func() {
				ui.Column(c).Children(func() { sec.body(u, c, p, u.shownSettings()) })
			})
		})
	})
}

// settingsSection is a page of the settings window, as the sidebar lists it.
type settingsSection struct {
	id, zh, en string
	icon       *ui.SVG
	body       func(u *UI, c *ui.Context, p *pal, s settings.Settings)
}

func (sec settingsSection) name() string { return i18n.T(sec.zh, sec.en) }

func icon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

// settingsSections is filled in init: its bodies reach the window that
// lists it, which a variable's initializer may not.
var settingsSections []settingsSection

func init() {
	settingsSections = []settingsSection{
		{"general", "通用", "General", icon(`<path d="M4 6h10M18 6h2M4 12h4M12 12h8M4 18h12M20 18h0"/><circle cx="16" cy="6" r="2"/><circle cx="10" cy="12" r="2"/><circle cx="18" cy="18" r="2"/>`), (*UI).generalSection},
		{"inbound", "入站", "Inbound", icon(`<path d="M12 3v12"/><path d="m7 10 5 5 5-5"/><path d="M4 15v4a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-4"/>`), (*UI).inboundSection},
		{"core", "内核", "Core", icon(`<rect x="5" y="5" width="14" height="14" rx="2"/><rect x="9" y="9" width="6" height="6" rx="1"/><path d="M9 2v3M15 2v3M9 19v3M15 19v3M2 9h3M2 15h3M19 9h3M19 15h3"/>`), (*UI).coreSection},
		{"rules", "规则", "Rules", icon(`<path d="M4 6h9M4 12h6M4 18h9"/><path d="m15 9 3 3-3 3"/><path d="M10 12h8"/>`), (*UI).rulesSection},
		{"dns", "DNS", "DNS", icon(`<circle cx="12" cy="12" r="9"/><path d="M3 12h18"/><path d="M12 3a14 14 0 0 1 0 18a14 14 0 0 1 0-18z"/>`), (*UI).dnsSection},
		{"sniffer", "嗅探", "Sniffing", icon(`<circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/><path d="M8 11h6"/>`), (*UI).snifferSection},
		{"tun", "TUN", "TUN", icon(`<path d="M12 3 3 8l9 5 9-5-9-5z"/><path d="m3 13 9 5 9-5"/>`), (*UI).tunSection},
		{"geo", "GeoData", "GeoData", icon(`<circle cx="12" cy="12" r="9"/><path d="M3.6 9h16.8M3.6 15h16.8"/><path d="M12 3c2.5 2.7 3.8 5.7 3.8 9s-1.3 6.3-3.8 9c-2.5-2.7-3.8-5.7-3.8-9S9.5 5.7 12 3z"/>`), (*UI).geoSection},
		{"controller", "外部控制", "Controller", icon(`<rect x="3" y="4" width="18" height="14" rx="2"/><path d="M7 9l3 3-3 3M12 15h5"/>`), (*UI).controllerSection},
		{"mcp", "MCP", "MCP", icon(`<path d="M12 8V4H8"/><rect x="4" y="8" width="16" height="12" rx="2"/><path d="M2 14h2M20 14h2M9 13v2M15 13v2"/>`), (*UI).mcpSection},
		{"about", "关于", "About", icon(`<circle cx="12" cy="12" r="9"/><path d="M12 11v5"/><path d="M12 8h.01"/>`), (*UI).aboutSection},
	}
}

// settingsSection is the page chosen in the sidebar, the first at first.
func (u *UI) settingsSection() settingsSection {
	for _, sec := range settingsSections {
		if sec.id == u.set.section {
			return sec
		}
	}
	u.set.section = settingsSections[0].id
	return settingsSections[0]
}

func coreStateWord(state string) string {
	switch state {
	case "running":
		return i18n.T("运行中", "Running")
	case "starting":
		return i18n.T("正在启动", "Starting")
	case "stopped":
		return i18n.T("未运行", "Stopped")
	case "error":
		return i18n.T("出错", "Error")
	}
	return ""
}

// settingsSidebar lists the sections, under the window's controls, with
// the core's version and state at its foot.
func (u *UI) settingsSidebar(c *ui.Context, p *pal) {
	ui.Column(c).Width(196).Shrink(0).Background(p.paper2).BorderWidth(0, 1, 0, 0).BorderColor(p.hair2).Children(func() {
		ui.Row(c).Height(max(c.TitleBar().Height, 50)).Shrink(0).DragWindow()
		ui.Sidebar(c.Key("nav"), &u.set.section, func() {
			for _, sec := range settingsSections {
				ui.SidebarItem(c.Key(sec.id), sec.id, sec.icon, sec.name())
			}
		}).Grow(1).Padding(0, 10).Background(p.paper2).Label(i18n.T("设置", "Settings"))
		cs := u.snap.Core
		ui.Row(c).Gap(7).Padding(12, 18, 16, 18).AlignItems(ui.Center).MinWidth(0).Children(func() {
			col := p.faint
			switch cs.State {
			case "running":
				col = p.good
			case "error":
				col = p.signal
			case "starting":
				col = p.warn
			}
			ui.Box(c).Size(6, 6).Radius(3).Background(col).Shrink(0)
			ver := "mihomo"
			if cs.Version != "" {
				ver += " " + cs.Version
			}
			// MinWidth(0): a line that doesn't wrap is otherwise as wide as
			// its words, and a long version runs into the pane.
			ui.Text(c, ver).FontSize(11.5).TextColor(p.mute).NoWrap().Ellipsis("…").Shrink(1).MinWidth(0).Tooltip(ver)
		})
	})
}

// sectionNote is the line under a section's title saying what it is about.
func sectionNote(c *ui.Context, p *pal, s string) {
	ui.Text(c, s).FontSize(12).TextColor(p.mute).Padding(0, 0, 6, 0).BorderWidth(0, 0, 1, 0).BorderColor(p.hair)
}

func (u *UI) generalSection(c *ui.Context, p *pal, s settings.Settings) {
	sectionNote(c, p, i18n.T("bowel 自己的偏好", "bowel's own preferences"))
	optRow(c, p, i18n.T("语言", "Language"), i18n.T("界面使用的语言", "The language of the interface"), func() {
		names := make([]string, len(i18n.Languages))
		for i, l := range i18n.Languages {
			names[i] = i18n.Names[l]
		}
		sel := max(0, slices.Index(i18n.Languages, s.Language))
		if actSeg(c.Key("lang"), p, &sel, names).Changed() && i18n.Languages[sel] != s.Language {
			l := i18n.Languages[sel]
			i18n.Set(l) // at once; the app saves it after
			u.setOpt(func(s *settings.Settings) { s.Language = l })
		}
	})
	optRow(c, p, i18n.T("外观", "Appearance"), i18n.T("浅色、深色，或跟随 Mac 的设置", "Light, dark, or as the Mac is set"), func() {
		names := []string{i18n.T("跟随系统", "System"), i18n.T("浅色", "Light"), i18n.T("深色", "Dark")}
		sel := max(0, slices.Index(settings.Appearances, s.Appearance))
		if actSeg(c.Key("appearance"), p, &sel, names).Changed() && settings.Appearances[sel] != s.Appearance {
			a := settings.Appearances[sel]
			u.setOpt(func(s *settings.Settings) { s.Appearance = a })
		}
	})
	optRow(c, p, i18n.T("边框", "Borders"), i18n.T("首页各块之间的线：笔直，或像 logo 一样弯曲", "The lines between the home board's blocks: straight, or bending like the logo's"), func() {
		names := []string{i18n.T("笔直", "Straight"), i18n.T("弯曲", "Twisted")}
		sel := max(0, slices.Index(settings.BorderStyles, s.Borders))
		if actSeg(c.Key("borders"), p, &sel, names).Changed() && settings.BorderStyles[sel] != s.Borders {
			b := settings.BorderStyles[sel]
			u.setOpt(func(s *settings.Settings) { s.Borders = b })
		}
	})
	optRow(c, p, i18n.T("主题色", "Theme color"), i18n.T("按钮、开关和选中项的颜色", "The color of buttons, switches and what is selected"), func() {
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			for _, a := range accentPresets {
				u.accentSwatch(c, p, a.hex, i18n.T(a.zh, a.en), strings.EqualFold(s.Accent, a.hex))
			}
		})
		custom := s.Accent
		if custom == "ink" {
			custom = ""
		}
		u.optText(c, p, "accent", i18n.T("主题色", "Theme color"), custom, 86, func(v string) {
			v = strings.ToUpper(strings.TrimSpace(v))
			if v != "" && v[0] != '#' {
				v = "#" + v
			}
			if v == "" {
				v = "ink"
			}
			if !settings.ValidAccent(v) {
				u.settingsToast(i18n.T("主题色要写成 #RRGGBB", "The theme color must be #RRGGBB"))
				return
			}
			u.setOpt(func(s *settings.Settings) { s.Accent = v })
		}).Margin(0, 0, 0, 6)
	})
	optRow(c, p, i18n.T("开机启动", "Open at login"), i18n.T("登录 Mac 时自动打开 bowel", "Open bowel when you log in to this Mac"), func() {
		on := u.snap.OpenAtLogin
		if ui.Switch(c.Key("login"), &on).Label(i18n.T("开机启动", "Open at login")).Changed() {
			u.snap.OpenAtLogin = on // shown at once; the next snapshot says what the system did
			if u.app != nil {
				go func() {
					if err := u.app.setOpenAtLogin(on); err != nil {
						u.settingsToast(err.Error())
						u.app.refresh()
					}
				}()
			}
		}
	})
	u.optSwitch(c, p, "hidden", i18n.T("静默启动", "Start hidden"), i18n.T("开机启动时只留在菜单栏，不打开窗口", "When opened at login, stay in the menu bar without a window"), s.StartHidden,
		func(s *settings.Settings, on bool) { s.StartHidden = on })
}

// accentSwatch is one theme color to pick: a dot of it, ringed when it
// is the one in use.
func (u *UI) accentSwatch(c *ui.Context, p *pal, hex, name string, on bool) {
	col := p.ink
	if hex != "ink" {
		col = ui.Hex(accentHex(hex, p.dark))
	}
	ring := ui.Transparent
	if on {
		ring = p.ink
	}
	ui.ButtonBase(c.Key("accent-"+hex)).Size(26, 26).Radius(13).Padding(3).Border(1.5, ring).
		Label(name).Tooltip(name).OnClick(func() {
		u.setOpt(func(s *settings.Settings) { s.Accent = hex })
	}).Children(func() {
		ui.Box(c).Size(17, 17).Radius(8.5).Background(col).Border(1, p.hair)
	})
}

func (u *UI) inboundSection(c *ui.Context, p *pal, s settings.Settings) {
	sectionNote(c, p, i18n.T("流量从哪里进到 mihomo", "Where traffic enters mihomo"))
	optRow(c, p, i18n.T("混合端口", "Mixed port"), i18n.T("HTTP 和 SOCKS5 共用一个端口，系统代理指向它", "One port for both HTTP and SOCKS5; the system proxy points at it"), func() {
		u.optNumber(c, p, "port", i18n.T("混合端口", "Mixed port"), s.MixedPort, "", func(v int) {
			u.setOpt(func(s *settings.Settings) { s.MixedPort = v })
		})
	})
	optRow(c, p, i18n.T("HTTP 端口", "HTTP port"), i18n.T("只收 HTTP 代理的端口，0 不开", "A port for HTTP proxying only; 0 is off"), func() {
		u.optNumber(c, p, "http", i18n.T("HTTP 端口", "HTTP port"), s.HTTPPort, "", func(v int) {
			u.setOpt(func(s *settings.Settings) { s.HTTPPort = v })
		})
	})
	optRow(c, p, i18n.T("SOCKS 端口", "SOCKS port"), i18n.T("只收 SOCKS5 的端口，0 不开", "A port for SOCKS5 only; 0 is off"), func() {
		u.optNumber(c, p, "socks", i18n.T("SOCKS 端口", "SOCKS port"), s.SocksPort, "", func(v int) {
			u.setOpt(func(s *settings.Settings) { s.SocksPort = v })
		})
	})

	optHeading(c, p, i18n.T("局域网", "LAN"))
	lan := i18n.T("同一网络里的设备可以把 bowel 当代理用", "Devices on the same network can use bowel as their proxy")
	if s.AllowLan && u.snap.Takeover.LanAddr != "" {
		lan = i18n.T("其他设备连 ", "Other devices connect to ") + u.snap.Takeover.LanAddr
	}
	optRow(c, p, i18n.T("允许局域网连接", "Allow LAN connections"), lan, func() {
		on := s.AllowLan
		if ui.Switch(c.Key("lan"), &on).Label(i18n.T("允许局域网连接", "Allow LAN connections")).Changed() {
			u.setOpt(func(s *settings.Settings) { s.AllowLan = on })
		}
	})
	optWide(c, p, i18n.T("允许的地址", "Allowed addresses"), i18n.T("只有这些网段的设备能连，如 192.168.0.0/16", "Only devices in these networks may connect, as 192.168.0.0/16"), func() {
		u.optList(c, p, "lanips", i18n.T("允许的地址", "Allowed addresses"), s.LanAllowedIPs, func(v []string) {
			u.setOpt(func(s *settings.Settings) { s.LanAllowedIPs = v })
		})
	})
	optWide(c, p, i18n.T("认证", "Authentication"), i18n.T("用户名:密码；局域网设备要先登录，本机不用", "user:password; LAN devices sign in first, this Mac never does"), func() {
		u.optList(c, p, "auth", i18n.T("认证", "Authentication"), s.Authentication, func(v []string) {
			u.setOpt(func(s *settings.Settings) { s.Authentication = v })
		})
	})

	optHeading(c, p, i18n.T("系统代理", "System proxy"))
	optWide(c, p, i18n.T("不走代理", "Bypass"), i18n.T("系统代理跳过这些域名和网段", "Domains and networks the system proxy skips"), func() {
		u.optList(c, p, "bypass", i18n.T("不走代理", "Bypass"), s.Bypass, func(v []string) {
			u.setOpt(func(s *settings.Settings) { s.Bypass = v })
		})
	})
}

func (u *UI) coreSection(c *ui.Context, p *pal, s settings.Settings) {
	sectionNote(c, p, i18n.T("mihomo 自己的行为", "How mihomo itself behaves"))
	optRow(c, p, i18n.T("日志级别", "Log level"), i18n.T("「日志」页里内核日志的详细程度", "How detailed the core's log on the Log page is"), func() {
		u.optChoice(c, p, "log", settings.LogLevels, s.LogLevel, func(v string) {
			u.setOpt(func(s *settings.Settings) { s.LogLevel = v })
		})
	})
	u.optSwitch(c, p, "ipv6", "IPv6", i18n.T("解析和连接 IPv6 地址", "Resolve and connect to IPv6 addresses"), s.IPv6, func(s *settings.Settings, on bool) { s.IPv6 = on })
	u.optSwitch(c, p, "unified", i18n.T("统一延迟", "Unified delay"), i18n.T("测速时扣掉握手，不同协议的延迟可以直接比较", "Leave out the handshake when testing, so delays of different protocols compare"), s.UnifiedDelay,
		func(s *settings.Settings, on bool) { s.UnifiedDelay = on })
	u.optSwitch(c, p, "tcpc", i18n.T("TCP 并发", "TCP concurrent"), i18n.T("一个域名有多个 IP 时同时连，用最先连上的", "When a domain has several IPs, dial them all and use the first to connect"), s.TCPConcurrent,
		func(s *settings.Settings, on bool) { s.TCPConcurrent = on })
	optRow(c, p, i18n.T("进程匹配", "Process matching"), i18n.T("找出连接来自哪个 App；strict 只在规则用到时找", "Find the app behind a connection; strict only when a rule needs it"), func() {
		u.optChoice(c, p, "findproc", core.FindProcess, s.FindProcess, func(v string) {
			u.setOpt(func(s *settings.Settings) { s.FindProcess = v })
		})
	})
	optRow(c, p, i18n.T("TLS 指纹", "TLS fingerprint"), i18n.T("所有节点的 TLS 握手模仿的浏览器", "The browser every node's TLS handshake imitates"), func() {
		labels := make([]string, len(core.Fingerprints))
		for i, f := range core.Fingerprints {
			labels[i] = f
			if f == "" {
				labels[i] = i18n.T("不模仿", "None")
			}
		}
		u.optSelect(c, p, "fp", i18n.T("TLS 指纹", "TLS fingerprint"), core.Fingerprints, labels, s.Fingerprint, func(v string) {
			u.setOpt(func(s *settings.Settings) { s.Fingerprint = v })
		})
	})
	optRow(c, p, i18n.T("出站网卡", "Outbound interface"), i18n.T("流量从哪块网卡出去，如 en0；留空自动", "The interface traffic leaves by, as en0; empty picks it"), func() {
		u.optText(c, p, "iface", i18n.T("出站网卡", "Outbound interface"), s.Interface, 120, func(v string) {
			v = strings.TrimSpace(v)
			u.setOpt(func(s *settings.Settings) { s.Interface = v })
		}).Placeholder(i18n.T("自动", "Automatic"))
	})

	optHeading(c, p, i18n.T("连接保活", "Keep-alive"))
	optRow(c, p, i18n.T("保活间隔", "Keep-alive interval"), i18n.T("TCP keep-alive 探测的间隔", "How often TCP keep-alive probes"), func() {
		u.optNumber(c, p, "keepalive", i18n.T("保活间隔", "Keep-alive interval"), s.KeepAliveInterval, i18n.T("秒", "s"), func(v int) {
			u.setOpt(func(s *settings.Settings) { s.KeepAliveInterval = v })
		})
	})
	u.optSwitch(c, p, "nokeepalive", i18n.T("关闭保活", "Disable keep-alive"), i18n.T("省电，但空闲连接可能被中间设备断开", "Saves power, but idle connections may be dropped on the way"), s.DisableKeepAlive,
		func(s *settings.Settings, on bool) { s.DisableKeepAlive = on })

	optHeading(c, p, i18n.T("测速", "Testing"))
	optWide(c, p, i18n.T("测速地址", "Test URL"), i18n.T("测节点延迟和自动选择时请求的地址", "The address requested to test node delay and for automatic selection"), func() {
		u.optText(c, p, "testurl", i18n.T("测速地址", "Test URL"), s.TestURL, 0, func(v string) {
			v = strings.TrimSpace(v)
			u.setOpt(func(s *settings.Settings) { s.TestURL = v })
		})
	})
	optRow(c, p, i18n.T("自动测速间隔", "Test interval"), i18n.T("「自动」多久测一次所有节点", "How often Auto tests every node"), func() {
		u.optNumber(c, p, "urltest", i18n.T("自动测速间隔", "Test interval"), s.URLTestInterval, i18n.T("秒", "s"), func(v int) {
			u.setOpt(func(s *settings.Settings) { s.URLTestInterval = v })
		})
	})
	optRow(c, p, i18n.T("切换容差", "Tolerance"), i18n.T("别的节点快过这么多，「自动」才换过去", "How much faster another node must be before Auto switches"), func() {
		u.optNumber(c, p, "tolerance", i18n.T("切换容差", "Tolerance"), s.URLTestTolerance, "ms", func(v int) {
			u.setOpt(func(s *settings.Settings) { s.URLTestTolerance = v })
		})
	})
	optRow(c, p, i18n.T("健康检查间隔", "Health check interval"), i18n.T("Provider 多久检查一次节点是否可用", "How often providers check that their nodes work"), func() {
		u.optNumber(c, p, "health", i18n.T("健康检查间隔", "Health check interval"), s.HealthInterval, i18n.T("秒", "s"), func(v int) {
			u.setOpt(func(s *settings.Settings) { s.HealthInterval = v })
		})
	})
}

func (u *UI) dnsSection(c *ui.Context, p *pal, s settings.Settings) {
	tunOn := u.snap.Takeover.Tun.Status == "on"
	note := i18n.T("域名怎么解析成 IP", "How domains resolve to IPs")
	if tunOn {
		note = i18n.T("TUN 开着，内置 DNS 总是启用", "TUN is on, so the built-in DNS is always on")
	}
	sectionNote(c, p, note)
	u.optSwitch(c, p, "dns", i18n.T("内置 DNS", "Built-in DNS"), i18n.T("不开 TUN 时也由 mihomo 解析域名，而不是系统", "Let mihomo resolve domains instead of the system, even without TUN"), s.DNS || tunOn,
		func(s *settings.Settings, on bool) { s.DNS = on }, tunOn)
	optRow(c, p, i18n.T("增强模式", "Enhanced mode"), i18n.T("fake-ip 更快；redir-host 返回真实 IP，兼容性更好", "fake-ip is faster; redir-host returns real IPs and is more compatible"), func() {
		u.optChoice(c, p, "dnsmode", core.DNSModes, s.DNSMode, func(v string) {
			u.setOpt(func(s *settings.Settings) { s.DNSMode = v })
		})
	})
	if s.DNSMode == "fake-ip" {
		optRow(c, p, i18n.T("Fake-IP 网段", "Fake-IP range"), i18n.T("分给域名的假地址从这里取", "Where the fake addresses for domains come from"), func() {
			u.optText(c, p, "fakerange", i18n.T("Fake-IP 网段", "Fake-IP range"), s.FakeIPRange, 150, func(v string) {
				v = strings.TrimSpace(v)
				u.setOpt(func(s *settings.Settings) { s.FakeIPRange = v })
			})
		})
		optWide(c, p, i18n.T("Fake-IP 过滤", "Fake-IP filter"), i18n.T("这些域名拿真实 IP，如 +.lan、time.*.com", "These domains get real IPs, as +.lan or time.*.com"), func() {
			u.optList(c, p, "fakefilter", i18n.T("Fake-IP 过滤", "Fake-IP filter"), s.FakeIPFilter, func(v []string) {
				u.setOpt(func(s *settings.Settings) { s.FakeIPFilter = v })
			})
		})
	}

	optHeading(c, p, i18n.T("服务器", "Servers"))
	optWide(c, p, i18n.T("上游服务器", "Upstream servers"), i18n.T("DoH、DoT 链接或 IP", "DoH or DoT links, or IPs"), func() {
		u.optList(c, p, "ns", i18n.T("上游服务器", "Upstream servers"), s.Nameservers, func(v []string) {
			u.setOpt(func(s *settings.Settings) { s.Nameservers = v })
		})
	})
	optWide(c, p, i18n.T("引导服务器", "Bootstrap servers"), i18n.T("解析上面服务器自己的域名，要写 IP", "Resolve the servers above by name; must be IPs"), func() {
		u.optList(c, p, "defns", i18n.T("引导服务器", "Bootstrap servers"), s.DefaultNameservers, func(v []string) {
			u.setOpt(func(s *settings.Settings) { s.DefaultNameservers = v })
		})
	})
	optWide(c, p, i18n.T("备用服务器", "Fallback servers"), i18n.T("上游的结果不可信时（如境外 IP）改问它们；可留空", "Asked when the upstream answer looks wrong, as a foreign IP; may be empty"), func() {
		u.optList(c, p, "fallback", i18n.T("备用服务器", "Fallback servers"), s.Fallback, func(v []string) {
			u.setOpt(func(s *settings.Settings) { s.Fallback = v })
		})
	})
	optWide(c, p, i18n.T("节点域名服务器", "Servers for node names"), i18n.T("只用来解析节点的地址；可留空", "Only resolve the nodes' addresses; may be empty"), func() {
		u.optList(c, p, "proxyns", i18n.T("节点域名服务器", "Servers for node names"), s.ProxyNameservers, func(v []string) {
			u.setOpt(func(s *settings.Settings) { s.ProxyNameservers = v })
		})
	})
	optWide(c, p, i18n.T("域名策略", "Nameserver policy"), i18n.T("域名=服务器，如 geosite:cn=223.5.5.5；同一域名可写多条", "domain=server, as geosite:cn=223.5.5.5; a domain may repeat"), func() {
		u.optList(c, p, "policy", i18n.T("域名策略", "Nameserver policy"), s.NameserverPolicy, func(v []string) {
			u.setOpt(func(s *settings.Settings) { s.NameserverPolicy = v })
		})
	})
	u.optSwitch(c, p, "respect", i18n.T("遵循规则", "Respect rules"), i18n.T("DNS 查询也按分流规则走代理；要先填节点域名服务器", "DNS queries follow the routing rules too; needs servers for node names"), s.RespectRules,
		func(s *settings.Settings, on bool) { s.RespectRules = on })
	u.optSwitch(c, p, "h3", i18n.T("优先 HTTP/3", "Prefer HTTP/3"), i18n.T("DoH 服务器支持时用 HTTP/3", "Use HTTP/3 with DoH servers that support it"), s.PreferH3,
		func(s *settings.Settings, on bool) { s.PreferH3 = on })

	optHeading(c, p, "Hosts")
	u.optSwitch(c, p, "usehosts", i18n.T("使用 Hosts", "Use hosts"), i18n.T("先查下面的 Hosts", "Look up the hosts below first"), s.UseHosts,
		func(s *settings.Settings, on bool) { s.UseHosts = on })
	u.optSwitch(c, p, "syshosts", i18n.T("使用系统 Hosts", "Use the system's hosts"), i18n.T("也查 /etc/hosts", "Look up /etc/hosts too"), s.UseSystemHosts,
		func(s *settings.Settings, on bool) { s.UseSystemHosts = on })
	optWide(c, p, "Hosts", i18n.T("域名=IP，如 router.lan=192.168.1.1", "domain=IP, as router.lan=192.168.1.1"), func() {
		u.optList(c, p, "hosts", "Hosts", s.Hosts, func(v []string) {
			u.setOpt(func(s *settings.Settings) { s.Hosts = v })
		})
	})
	optRow(c, p, i18n.T("清除 DNS 缓存", "Flush DNS cache"), i18n.T("忘掉解析结果和 Fake-IP 映射，下次重新解析", "Forget resolved names and fake-ip mappings; they resolve again"), func() {
		u.optAction(c, p, "flush", i18n.T("清除", "Flush"), i18n.T("已清除 DNS 缓存", "DNS cache flushed"), func() error { return u.app.flushDNS() })
	})
}

func (u *UI) snifferSection(c *ui.Context, p *pal, s settings.Settings) {
	sectionNote(c, p, i18n.T("从流量里读出域名，让只有 IP 的连接也能按域名分流", "Read domains from the traffic, so connections with only an IP route by domain too"))
	u.optSwitch(c, p, "sniff", i18n.T("域名嗅探", "Sniffing"), i18n.T("读 HTTP 的 Host、TLS 和 QUIC 的 SNI", "Read HTTP's Host, and TLS's and QUIC's SNI"), s.Sniff,
		func(s *settings.Settings, on bool) { s.Sniff = on })
	u.optSwitch(c, p, "override", i18n.T("覆盖目标地址", "Override destination"), i18n.T("用嗅到的域名替换连接的目标，而不只用来匹配规则", "Connect to the sniffed domain, not only match rules with it"), s.SniffOverride,
		func(s *settings.Settings, on bool) { s.SniffOverride = on })
	u.optSwitch(c, p, "forcedns", i18n.T("强制 DNS 映射", "Force DNS mapping"), i18n.T("经 DNS 映射来的连接也嗅探", "Sniff connections mapped by DNS too"), s.SniffForceDNSMapping,
		func(s *settings.Settings, on bool) { s.SniffForceDNSMapping = on })
	u.optSwitch(c, p, "pureip", i18n.T("嗅探纯 IP", "Sniff pure IPs"), i18n.T("直接连 IP 的连接也嗅探", "Sniff connections made straight to an IP"), s.SniffParsePureIP,
		func(s *settings.Settings, on bool) { s.SniffParsePureIP = on })
	optHeading(c, p, i18n.T("端口", "Ports"))
	for _, pr := range []struct {
		key, name string
		val       []string
		set       func(*settings.Settings, []string)
	}{
		{"shttp", "HTTP", s.SniffHTTP, func(s *settings.Settings, v []string) { s.SniffHTTP = v }},
		{"stls", "TLS", s.SniffTLS, func(s *settings.Settings, v []string) { s.SniffTLS = v }},
		{"squic", "QUIC", s.SniffQUIC, func(s *settings.Settings, v []string) { s.SniffQUIC = v }},
	} {
		optWide(c, p, pr.name, i18n.T("在这些端口上嗅探 ", "Sniff ")+pr.name+i18n.T("，如 443 或 8080-8880", " on these ports, as 443 or 8080-8880"), func() {
			u.optList(c, p, pr.key, pr.name, pr.val, func(v []string) {
				u.setOpt(func(s *settings.Settings) { pr.set(s, v) })
			})
		})
	}
	optWide(c, p, i18n.T("跳过", "Skip"), i18n.T("这些域名不嗅探，如 +.push.apple.com", "Domains never sniffed, as +.push.apple.com"), func() {
		u.optList(c, p, "sskip", i18n.T("跳过", "Skip"), s.SniffSkip, func(v []string) {
			u.setOpt(func(s *settings.Settings) { s.SniffSkip = v })
		})
	})
}

func (u *UI) tunSection(c *ui.Context, p *pal, s settings.Settings) {
	sectionNote(c, p, i18n.T("在「代理」页的接管里开关", "Turned on and off under Takeover on the Proxy page"))
	optRow(c, p, i18n.T("协议栈", "Stack"), i18n.T("mixed：TCP 用 system，UDP 用 gVisor", "mixed: system for TCP, gVisor for UDP"), func() {
		u.optChoice(c, p, "stack", core.TunStacks, s.TunStack, func(v string) {
			u.setOpt(func(s *settings.Settings) { s.TunStack = v })
		})
	})
	u.optSwitch(c, p, "strict", i18n.T("严格路由", "Strict route"), i18n.T("不让流量绕过 TUN；可能影响访问局域网", "Keep traffic from going around TUN; may affect reaching the LAN"), s.StrictRoute,
		func(s *settings.Settings, on bool) { s.StrictRoute = on })
	u.optSwitch(c, p, "autoif", i18n.T("自动选择网卡", "Detect interface"), i18n.T("出站跟着系统的默认网卡走", "Send traffic out by the system's default interface"), s.AutoDetectInterface,
		func(s *settings.Settings, on bool) { s.AutoDetectInterface = on })
	u.optSwitch(c, p, "einat", i18n.T("端点独立 NAT", "Endpoint-independent NAT"), i18n.T("改善游戏和语音通话的 UDP；略增开销", "Better UDP for games and calls, at a little cost"), s.EndpointIndependentNAT,
		func(s *settings.Settings, on bool) { s.EndpointIndependentNAT = on })
	optRow(c, p, i18n.T("网卡名", "Device"), i18n.T("TUN 网卡的名字，如 utun1024；留空由系统分配", "The TUN device's name, as utun1024; empty lets the system pick"), func() {
		u.optText(c, p, "device", i18n.T("网卡名", "Device"), s.TunDevice, 120, func(v string) {
			v = strings.TrimSpace(v)
			u.setOpt(func(s *settings.Settings) { s.TunDevice = v })
		}).Placeholder(i18n.T("自动", "Automatic"))
	})
	optRow(c, p, "MTU", i18n.T("一个包最大多少字节", "The largest packet, in bytes"), func() {
		u.optNumber(c, p, "mtu", "MTU", s.TunMTU, "", func(v int) {
			u.setOpt(func(s *settings.Settings) { s.TunMTU = v })
		})
	})
	optWide(c, p, i18n.T("DNS 劫持", "DNS hijack"), i18n.T("发到这些地址的 DNS 查询交给 mihomo，如 any:53", "DNS queries to these addresses go to mihomo, as any:53"), func() {
		u.optList(c, p, "hijack", i18n.T("DNS 劫持", "DNS hijack"), s.DNSHijack, func(v []string) {
			u.setOpt(func(s *settings.Settings) { s.DNSHijack = v })
		})
	})
}

func (u *UI) geoSection(c *ui.Context, p *pal, s settings.Settings) {
	sectionNote(c, p, i18n.T("GEOIP、GEOSITE 规则用的地址库", "The databases GEOIP and GEOSITE rules look up"))
	u.optSwitch(c, p, "geoauto", i18n.T("自动更新", "Update automatically"), i18n.T("按下面的间隔重新下载", "Download them again at the interval below"), s.GeoAutoUpdate,
		func(s *settings.Settings, on bool) { s.GeoAutoUpdate = on })
	optRow(c, p, i18n.T("更新间隔", "Interval"), i18n.T("多久下载一次", "How often to download them"), func() {
		u.optNumber(c, p, "geoint", i18n.T("更新间隔", "Interval"), s.GeoUpdateInterval, i18n.T("小时", "h"), func(v int) {
			u.setOpt(func(s *settings.Settings) { s.GeoUpdateInterval = v })
		})
	})
	optRow(c, p, i18n.T("立即更新", "Update now"), i18n.T("从下面的地址重新下载，在后台进行", "Download again from the links below, in the background"), func() {
		u.optAction(c, p, "geonow", i18n.T("更新", "Update"), i18n.T("已开始下载，内核下载完会自动载入", "Downloading; the core loads the files when done"), func() error { return u.app.updateGeo() })
	})
	optHeading(c, p, i18n.T("下载地址", "Links"))
	for _, g := range []struct {
		key, name, val string
		set            func(*settings.Settings, string)
	}{
		{"geoip", "GeoIP", s.GeoIPURL, func(s *settings.Settings, v string) { s.GeoIPURL = v }},
		{"geosite", "GeoSite", s.GeoSiteURL, func(s *settings.Settings, v string) { s.GeoSiteURL = v }},
		{"mmdb", "MMDB", s.MMDBURL, func(s *settings.Settings, v string) { s.MMDBURL = v }},
	} {
		optWide(c, p, g.name, fmt.Sprintf(i18n.T("%s 的下载链接", "Where %s downloads from"), g.name), func() {
			u.optText(c, p, g.key, g.name, g.val, 0, func(v string) {
				v = strings.TrimSpace(v)
				u.setOpt(func(s *settings.Settings) { g.set(s, v) })
			})
		})
	}
}

func (u *UI) controllerSection(c *ui.Context, p *pal, s settings.Settings) {
	own := u.snap.ControllerTCP
	note := i18n.T("给 metacubexd、yacd 等网页面板一个控制接口", "A controller for web dashboards such as metacubexd and yacd")
	if own != "" {
		note = i18n.T("bowel 的控制接口这次改用了 TCP（", "bowel's own controller runs on TCP this time (") + own + i18n.T("），这里的设置暂不生效", "), so these do not apply")
	}
	sectionNote(c, p, note)
	optRow(c, p, i18n.T("地址", "Address"), i18n.T("如 127.0.0.1:9090；留空关闭。0.0.0.0 会让局域网也能控制，要设密钥", "As 127.0.0.1:9090; empty is off. 0.0.0.0 lets the LAN control it too, so set a secret"), func() {
		u.optText(c, p, "ctl", i18n.T("地址", "Address"), s.ExtController, 170, func(v string) {
			v = strings.TrimSpace(v)
			u.setOpt(func(s *settings.Settings) { s.ExtController = v })
		}).Placeholder(i18n.T("关闭", "Off")).Disabled(own != "")
	})
	optRow(c, p, i18n.T("密钥", "Secret"), i18n.T("面板连接时要填的密钥", "What dashboards enter to connect"), func() {
		u.optText(c, p, "secret", i18n.T("密钥", "Secret"), s.ExtSecret, 170, func(v string) {
			v = strings.TrimSpace(v)
			u.setOpt(func(s *settings.Settings) { s.ExtSecret = v })
		}).Password().Disabled(own != "")
	})
}

func (u *UI) aboutSection(c *ui.Context, p *pal, _ settings.Settings) {
	cs := u.snap.Core
	ui.Column(c).Padding(4, 0, 14, 0).BorderWidth(0, 0, 1, 0).BorderColor(p.hair).Children(func() {
		ver := cs.Version
		if ver == "" {
			ver = "—"
		}
		state := coreStateWord(cs.State)
		if cs.Error != "" {
			state += " · " + firstLine(cs.Error)
		}
		path := u.snap.ConfigPath
		if path == "" {
			path = "—"
		}
		moho := u.snap.Version
		if moho == "" {
			moho = "—"
		}
		kv(c, p, [][2]string{
			{i18n.T("内核版本", "Core version"), ver},
			{i18n.T("内核状态", "Core state"), state},
			{i18n.T("运行配置", "Running config"), path},
			{i18n.T("bowel 版本", "bowel version"), moho},
			{i18n.T("界面字体", "Interface font"), i18n.T("OPPO Sans 4.0 · © OPPO 广东移动通信有限公司，依 OPPO Sans 字体许可使用", "OPPO Sans 4.0 · © Guangdong OPPO Mobile Telecommunications Corp., Ltd., used under the OPPO Sans font license")},
		})
	})
	optRow(c, p, i18n.T("运行配置", "Running config"), i18n.T("bowel 生成、mihomo 正在用的配置文件", "The config file bowel writes and mihomo runs on"), func() {
		btn(c.Key("reveal"), p, i18n.T("在 Finder 中显示", "Show in Finder")).OnClick(func() {
			if u.app != nil {
				go func() {
					if err := u.app.revealConfig(); err != nil {
						u.settingsToast(err.Error())
					}
				}()
			}
		})
	})
	optRow(c, p, i18n.T("恢复默认", "Restore defaults"), i18n.T("内核的选项回到 bowel 的默认；端口、局域网、接管和语言不变", "The core's options go back to bowel's defaults; the port, LAN, takeover and language stay"), func() {
		btn(c.Key("reset"), p, i18n.T("恢复默认", "Restore defaults")).OnClick(func() {
			u.setOpt((*settings.Settings).ResetCore)
		})
	})
}

// ---- Parts ----

// optHeading starts a group of options within a section.
func optHeading(c *ui.Context, p *pal, s string) {
	ui.Text(c, s).FontSize(11.5).FontWeight(700).TextColor(p.faint).Padding(20, 0, 2, 0)
}

// optLabel is an option's name and what it does.
func optLabel(c *ui.Context, p *pal, title, desc string) ui.Element {
	return ui.Column(c).Gap(2).Children(func() {
		ui.Text(c, title).FontSize(13).FontWeight(600).TextColor(p.ink)
		ui.Text(c, desc).FontSize(11.5).TextColor(p.mute)
	})
}

// optRow is an option with its control on the right.
func optRow(c *ui.Context, p *pal, title, desc string, control func()) {
	ui.Row(c).Gap(18).Padding(11, 0).MinHeight(52).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.hair2).Children(func() {
		optLabel(c, p, title, desc).Grow(1).Basis(0).MinWidth(0)
		ui.Row(c).Shrink(0).Gap(6).AlignItems(ui.Center).Children(control)
	})
}

// optWide is an option whose control, a long text or a list, goes under it.
func optWide(c *ui.Context, p *pal, title, desc string, control func()) {
	ui.Column(c).Gap(8).Padding(11, 0).BorderWidth(0, 0, 1, 0).BorderColor(p.hair2).Children(func() {
		optLabel(c, p, title, desc)
		ui.Row(c).Children(control)
	})
}

func (u *UI) optSwitch(c *ui.Context, p *pal, key, title, desc string, val bool, set func(*settings.Settings, bool), disabled ...bool) {
	optRow(c, p, title, desc, func() {
		on := val
		sw := ui.Switch(c.Key(key), &on).Label(title)
		if len(disabled) > 0 && disabled[0] {
			sw.Disabled(true)
		}
		if sw.Changed() {
			u.setOpt(func(s *settings.Settings) { set(s, on) })
		}
	})
}

// optChoice is a segmented choice among a few values.
func (u *UI) optChoice(c *ui.Context, p *pal, key string, values []string, val string, set func(string)) {
	sel := max(0, slices.Index(values, val))
	if actSeg(c.Key(key), p, &sel, values).Changed() && values[sel] != val {
		set(values[sel])
	}
}

// optSelect is a drop-down choice among values, shown as labels.
func (u *UI) optSelect(c *ui.Context, p *pal, key, label string, values, labels []string, val string, set func(string)) {
	cur := labels[max(0, slices.Index(values, val))]
	if ui.Select(c.Key(key), &cur, labels).Label(label).Width(140).Changed() {
		if v := values[max(0, slices.Index(labels, cur))]; v != val {
			set(v)
		}
	}
}

// optText is a text option. It shows the saved value until edited, and
// keeps the edit on Enter or when the focus leaves it. width 0 fills the row.
func (u *UI) optText(c *ui.Context, p *pal, key, label, saved string, width float32, commit func(string)) ui.Element {
	f := u.set.field(key)
	focused := f.h.Focused(c)
	if !focused {
		f.text = saved
	}
	in := ui.TextInput(c.Key(key), &f.text).Bind(&f.h).Label(label).Padding(5, 9).Radius(7).
		Background(p.card).Border(1, p.hair).FontSize(12.5).Font("SF Mono, Menlo")
	if width > 0 {
		in.Width(width)
	} else {
		in.Grow(1).Basis(0).MinWidth(0)
	}
	if focused {
		in.BorderColor(p.ink)
	}
	if (in.Submitted() || in.FocusLeft()) && strings.TrimSpace(f.text) != saved {
		commit(f.text)
	}
	return in
}

// optNumber is a whole-number option, kept as optText keeps text, with
// its unit after it.
func (u *UI) optNumber(c *ui.Context, p *pal, key, label string, saved int, unit string, commit func(int)) {
	u.optText(c, p, key, label, strconv.Itoa(saved), 76, func(v string) {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			c.Toast(label + i18n.T("要是整数", " must be a whole number"))
			return
		}
		if n != saved {
			commit(n)
		}
	})
	if unit != "" {
		ui.Text(c, unit).FontSize(12).TextColor(p.mute).Width(26)
	}
}

// optList is a list option: its entries as chips; Enter or a comma adds
// what was typed, each chip's button takes it out. Every change applies.
func (u *UI) optList(c *ui.Context, p *pal, key, label string, saved []string, commit func([]string)) {
	f := u.set.field(key)
	if !slices.Equal(f.base, saved) { // saved anew, or a change refused
		f.list, f.base = slices.Clone(saved), slices.Clone(saved)
	}
	// The field edits f.list between frames; a difference is an edit.
	ui.TokenField(c.Key(key), &f.list, nil).Label(label).Grow(1).Basis(0).MinWidth(0).FontSize(12.5)
	if !slices.Equal(f.list, f.base) {
		var clean []string
		for _, e := range f.list {
			if e = strings.TrimSpace(e); e != "" {
				if k, v, ok := core.Pair(e); ok {
					e = k + "=" + v // "a = b" as "a=b"
				}
				clean = append(clean, e)
			}
		}
		f.list, f.base = clean, slices.Clone(clean)
		if !slices.Equal(clean, saved) {
			commit(clean)
		}
	}
}

// optAction is a button running an action of the app, which says when it
// is done.
func (u *UI) optAction(c *ui.Context, p *pal, key, label, done string, run func() error) {
	if u.set.busy[key] {
		label += "…"
	}
	btn(c.Key(key), p, label).OnClick(func() {
		if u.app == nil || u.set.busy[key] {
			return
		}
		if u.set.busy == nil {
			u.set.busy = map[string]bool{}
		}
		u.set.busy[key] = true
		go func() {
			err := run()
			u.settingsUpdate(func() {
				delete(u.set.busy, key)
				if err != nil {
					u.set.toasts = append(u.set.toasts, err.Error())
				} else {
					u.set.toasts = append(u.set.toasts, done)
				}
			})
		}()
	})
}
