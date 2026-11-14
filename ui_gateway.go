package main

// Gateway mode in the interface: its switch, the two addresses to fill in
// on a device (manual), and under them the step to automatic, where bowel
// hands out the LAN's addresses. Both windows show them: the settings'
// 入站 page under 局域网, and the home board's takeover.

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/i18n"
)

// gatewayUI is the switch's flick while the app applies it.
type gatewayUI struct {
	want     bool
	flipping bool
	// ask is a question waiting for its answer, in place of the automatic
	// step's line: "off" (the gateway's switch) or "manual" (back from
	// automatic). Either leaves the devices bowel gave addresses without
	// a way out.
	ask string
}

// gatewayStep is where the gateway stands, in a word.
func gatewayStep(g GatewayView) string {
	switch {
	case !g.Wanted:
		return i18n.T("关闭", "Off")
	case g.Auto.Wanted:
		return i18n.T("自动", "Automatic")
	}
	return i18n.T("手动", "Manual")
}

// gatewayShownOn is the switch's position: the user's flick while it is
// applied, the saved choice otherwise.
func (u *UI) gatewayShownOn() bool {
	if u.gw.flipping {
		return u.gw.want
	}
	return u.snap.Takeover.Gateway.Wanted
}

// setGateway flicks the switch; failures toast in the window it was
// flicked in.
func (u *UI) setGateway(on bool, toast func(string)) {
	u.gw.want, u.gw.flipping = on, true
	if u.app == nil { // a test's UI
		u.snap.Settings.Gateway, u.snap.Takeover.Gateway.Wanted = on, on
		u.gw.flipping = false
		return
	}
	go func() {
		err := u.app.setGateway(on)
		u.settingsUpdate(func() { u.gw.flipping = false })
		if err != nil {
			toast(err.Error())
		}
	}()
}

// gatewaySwitch is the switch, for a window whose toasts are toast.
func (u *UI) gatewaySwitch(c *ui.Context, key string, toast func(string)) {
	on := u.gatewayShownOn()
	if ui.Switch(c.Key(key), &on).Label(i18n.T("网关模式", "Gateway mode")).Disabled(u.gw.flipping).Changed() {
		if !on && u.snap.Takeover.Gateway.Auto.Status == "on" {
			u.gw.ask = "off" // devices have their addresses from here: ask first
			return
		}
		u.setGateway(on, toast)
	}
}

// gatewayStatus is the gateway's state as a dot and a few words.
func (u *UI) gatewayStatus(p *pal) (ui.Color, string) {
	g := u.snap.Takeover.Gateway
	switch {
	case u.gw.flipping && u.gw.want && (!g.Ready || u.snap.Takeover.Tun.Status == "install"):
		return p.warn, i18n.T("请在系统弹窗中确认…", "Confirm in the system dialog…")
	case u.gw.flipping || u.pending("gateway"):
		return p.faint, i18n.T("正在应用…", "Applying…")
	case !g.Wanted && !g.Ready:
		return p.faint, i18n.T("未使用 · 首次开启需要授权一次", "Not in use · Needs authorization the first time")
	case !g.Wanted:
		return p.faint, i18n.T("未使用", "Not in use")
	case g.Status == "checking":
		return p.faint, i18n.T("正在检查…", "Checking…")
	case g.Status == "on" && g.Auto.Wanted && g.Auto.Status == "error":
		return p.signal, g.Auto.Detail
	case g.Status == "on":
		return p.good, i18n.T("生效中 · ", "Active · ") + gatewayStep(g)
	case g.Status == "error":
		return p.signal, g.Detail
	}
	return p.warn, g.Detail // paused, install
}

// gatewayDesc says what gateway mode is, beside its switch.
func gatewayDesc() string {
	return i18n.T("让手机、电视、游戏机把这台 Mac 当网关，按 bowel 的规则上网。会同时打开 TUN",
		"Phones, TVs and consoles use this Mac as their gateway and follow bowel's rules. Turns TUN on too")
}

// gatewayGuide is the gateway while it is wanted: the two addresses to
// fill in on a device, the step to automatic under them, and what about
// this computer stands in the way of either.
func (u *UI) gatewayGuide(c *ui.Context, p *pal, toast func(string)) {
	g := u.snap.Takeover.Gateway
	ui.Column(c.Key("gw-guide")).Gap(6).Children(func() {
		if g.IP == "" {
			ui.Text(c, i18n.T("这台 Mac 还没有局域网地址：先连上 Wi‑Fi 或网线。", "This Mac has no LAN address yet: connect it to Wi‑Fi or a cable first.")).FontSize(12).TextColor(p.mute)
			return
		}
		u.gatewayAddr(c, p, "gw-copy-gateway", i18n.T("网关", "Gateway"), g.IP, toast)
		u.gatewayAddr(c, p, "gw-copy-dns", "DNS", g.IP, toast)
		ui.Box(c).Height(1).Margin(2, 0).BorderStyle(ui.BorderDashed).BorderWidth(1, 0, 0, 0).BorderColor(p.hair)
		u.gatewayAuto(c, p, toast)
		if g.DHCP || g.WiFi || g.Firewall {
			ui.Column(c).Gap(5).Margin(4, 0, 0, 0).Children(func() {
				if g.DHCP {
					gatewayTip(c, p, p.warn, i18n.T("这台 Mac 的地址是自动获取的（DHCP），可能会变：在系统的网络设置里改成手动的固定地址。设为内网代理网关之前必须先改。",
						"This Mac gets its address from DHCP, so it may change: set a fixed (manual) address in the system's network settings. It has to be fixed before this Mac can become the LAN's gateway for every device."))
				}
				if g.WiFi {
					gatewayTip(c, p, p.warn, i18n.T("这台 Mac 走的是 Wi‑Fi：其他设备的流量都要经过它，改用网线会更快更稳。",
						"This Mac is on Wi‑Fi: every device's traffic goes through it, so a cable is faster and steadier."))
				}
				if g.Firewall {
					gatewayTip(c, p, p.warn, firewallTip())
				}
			})
		}
	})
}

// gatewayAuto is the line under the addresses: what stands between manual
// and automatic, and the one thing to do about it.
func (u *UI) gatewayAuto(c *ui.Context, p *pal, toast func(string)) {
	g := u.snap.Takeover.Gateway
	a := g.Auto
	set := func(on bool) {
		u.gw.ask = ""
		if u.app == nil { // a test's UI
			return
		}
		go func() {
			if err := u.app.setGatewayAuto(on); err != nil {
				toast(err.Error())
			}
		}()
	}
	check := func(label string) {
		btn(c.Key("gw-check"), p, label).Shrink(0).OnClick(func() {
			if u.app != nil {
				go func() { _, _ = u.app.checkDHCP() }()
			}
		})
	}
	title, sub, col := "", "", p.ink
	var acts func()
	switch {
	case u.gw.ask != "":
		title, col = i18n.T("关闭后，从这里领到地址的设备会断网", "Devices that got their address here will lose the network"), p.signal
		sub = i18n.T("直到路由器的 DHCP 重新打开", "Until the router's DHCP is on again")
		acts = func() {
			btn(c.Key("gw-ask-no"), p, i18n.T("取消", "Cancel")).Shrink(0).OnClick(func() { u.gw.ask = "" })
			dangerBtn(c.Key("gw-ask-yes"), p, i18n.T("仍然关闭", "Turn off anyway"), false).Shrink(0).OnClick(func() {
				ask := u.gw.ask
				u.gw.ask = ""
				if ask == "off" {
					u.setGateway(false, toast)
				} else {
					set(false)
				}
			})
		}
	case u.pending("gateway:auto") && a.Wanted:
		title, col = i18n.T("正在关闭…", "Turning off…"), p.mute
	case u.pending("gateway:auto"):
		title, col = i18n.T("正在开启…", "Turning on…"), p.mute
		sub = i18n.T("检测局域网，启动地址分配", "Checking the LAN, starting to hand out addresses")
	case a.Wanted && a.Status == "on":
		title, col = i18n.T("已开启", "On"), p.good
		sub = i18n.T("所有内网流量都通过本服务代理", "All of the LAN's traffic goes through this computer")
		acts = func() {
			link(c.Key("gw-devices"), p, i18n.T("查看设备", "Devices"), func() { u.act.showDevices(); u.page = "activity" })
			btn(c.Key("gw-auto-off"), p, i18n.T("关闭", "Turn off")).Shrink(0).OnClick(func() { u.gw.ask = "manual" })
		}
	case a.Wanted && a.Status == "starting":
		title, col = a.Detail, p.mute
	case a.Wanted:
		title, col = a.Detail, p.signal
		if title == "" {
			title = i18n.T("没有在分配地址", "Not handing out addresses")
		}
		acts = func() {
			check(i18n.T("重新检测", "Check again"))
			btn(c.Key("gw-auto-off"), p, i18n.T("关闭", "Turn off")).Shrink(0).OnClick(func() { set(false) })
		}
	case !a.Supported:
		title, col = i18n.T("这个系统上还不能设为内网代理网关", "Not available on this system yet"), p.mute
	case a.Checking:
		title, col = i18n.T("正在检测局域网里的 DHCP 服务器…", "Checking the LAN for DHCP servers…"), p.mute
	case a.CheckError != "":
		title, col = a.CheckError, p.signal
		acts = func() { check(i18n.T("重新检测", "Check again")) }
	case a.Checked.IsZero():
		title, col = i18n.T("还没有检测局域网里的 DHCP 服务器", "The LAN hasn't been checked for DHCP servers yet"), p.mute
		acts = func() { check(i18n.T("检测", "Check")) }
	case len(a.Servers) > 0:
		title = fmt.Sprintf(i18n.T("DHCP 服务器 %s", "DHCP server %s"), strings.Join(a.Servers, ", "))
		sub = i18n.T("关闭它的 DHCP 后，可以将当前服务设为内网代理网关", "With its DHCP off, this computer can become the LAN's gateway for every device")
		acts = func() { check(i18n.T("重新检测", "Check again")) }
	default:
		title = i18n.T("可以设为内网代理网关", "This computer can become the LAN's gateway")
		sub = i18n.T("开启后所有内网流量都将通过本服务代理", "All of the LAN's traffic will then go through it")
		acts = func() {
			pill(c.Key("gw-auto-on"), p, i18n.T("确认开启", "Turn on"), true).Shrink(0).OnClick(func() { set(true) })
		}
	}
	ui.Row(c.Key("gw-auto")).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, i18n.T("内网代理", "LAN gateway")).FontSize(12).TextColor(p.mute).Width(gatewayLabel).Shrink(0).NoWrap().AlignSelf(ui.Start).Margin(1, 0, 0, 0)
		ui.Column(c).Gap(2).Grow(1).Basis(0).MinWidth(0).Children(func() {
			ui.Text(c, title).FontSize(12.5).FontWeight(600).LineHeight(1.4).TextColor(col)
			if sub != "" {
				ui.Text(c, sub).FontSize(11.5).LineHeight(1.45).TextColor(p.mute)
			}
		})
		if acts != nil {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Shrink(0).Children(acts)
		}
	})
}

// gatewayLabel is the width of the labels, as the rows above them have
// (kv), so that the gateway's lines continue the same table.
const gatewayLabel = 110

// gatewayAddr is one address to fill in, with its copy button.
func (u *UI) gatewayAddr(c *ui.Context, p *pal, key, label, addr string, toast func(string)) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(p.mute).Width(gatewayLabel).Shrink(0).NoWrap()
		ui.Text(c, addr).Font(actMono).FontSize(13).FontWeight(600).TextColor(p.ink).Selectable().Grow(1).MinWidth(0)
		btn(c.Key(key), p, i18n.T("复制", "Copy")).Shrink(0).OnClick(func() {
			c.WriteClipboard(addr)
			toast(fmt.Sprintf(i18n.T("已复制 %s", "Copied %s"), addr))
		})
	})
}

func gatewayTip(c *ui.Context, p *pal, dot ui.Color, text string) {
	ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
		ui.Box(c).Size(5, 5).Radius(p.r(2.5)).Background(dot).Margin(6, 0, 0, 0).Shrink(0)
		ui.Text(c, text).FontSize(11.5).LineHeight(1.45).TextColor(p.mute).Grow(1).Basis(0).MinWidth(0)
	})
}
