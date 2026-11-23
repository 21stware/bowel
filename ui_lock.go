package main

import (
	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

// The exit lock: a route group whose traffic leaves by a node or not at
// all. Takeover's switch keeps its meaning; a locked group's traffic is
// refused while it is off, and never goes direct for want of a node.

var lockIcon = icon(`<rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>`)

// lockSay is what the lock does, in one line.
func lockSay() string {
	return i18n.T("只走节点；走不了就断开，不改走直连", "Only through a node; if none can carry it, it's cut, never sent direct")
}

// refusedSay is why a locked group's connections fail now.
func refusedSay() string {
	return i18n.T("接管已关闭，这个路由组走系统代理的连接被拦下", "Takeover is off, so this group's connections through the system proxy are refused")
}

// proxyOnlySay is what the lock cannot do without TUN.
func proxyOnlySay() string {
	return i18n.T("没开 TUN 时，出口锁定只管得住跟随系统代理的程序；不读系统代理的命令行工具、SDK 和应用仍会直连", "Without TUN the exit lock only holds programs that follow the system proxy; command-line tools, SDKs and apps that ignore it still go direct")
}

// lockNow is how a locked group's lock stands when that is not simply
// "held": a word for its chip, a sentence for its row and tooltip. Both
// are "" while it goes by a node as set.
func lockNow(r RouteView) (word, say string) {
	switch {
	case !r.Locked:
	case r.Unheld:
		return i18n.T("现在没有守住", "Not holding now"), unheldSay()
	case r.Refused:
		return i18n.T("接管已关，正在拦下", "Takeover off, refusing"), refusedSay()
	case r.Broken != "" && !r.Direct:
		return i18n.T("没有可用节点，正在拦下", "No usable node, refusing"), lockedOutSay()
	}
	return "", ""
}

// lockMark is the lock beside a group's name: quiet while the group goes by
// a node, in the signal color while its traffic is refused or the lock
// does not hold.
func lockMark(c *ui.Context, p *pal, r RouteView, size float32) ui.Element {
	col, tip := p.mute, i18n.T("出口锁定：", "Exit lock: ")+lockSay()
	if _, say := lockNow(r); say != "" {
		col, tip = p.signal, say
	}
	return ui.Icon(c, lockIcon).Size(size, size).Shrink(0).TextColor(col).Tooltip(tip).Label(tip)
}

// anyLocked says whether a group shown has the lock.
func (u *UI) anyLocked() bool {
	for _, r := range u.snap.Routes {
		if r.Locked {
			return true
		}
	}
	return false
}

// setLocked turns group rid's lock on or off. The first lock is said
// before it is put on; the ones after it are a click.
func (u *UI) setLocked(rid string, on bool) {
	if on && !u.anyLocked() && u.lockAsk == "" {
		u.lockAsk = rid
		return
	}
	u.lockAsk = ""
	u.changeRouting(func(s *settings.Settings) { s.Routing.SetLocked(rid, on) }, func() { u.app.closeRoute(rid) })
}

// lockChip is the lock's choice in a group's detail, after 走节点 and
// 直接连接, as tall as they are: chosen while the group is locked.
func (u *UI) lockChip(c *ui.Context, p *pal, rid string) {
	if u.snap.Imported != nil || !routes.Lockable(rid) {
		return
	}
	r := u.route(rid)
	on := u.shownSettings().Routing.Locked(rid)
	sub, subCol := i18n.T("只走节点，不改走直连", "Node only, never direct"), p.mute
	tip := lockSay()
	if word, say := lockNow(r); on && word != "" {
		sub, subCol, tip = word, p.signal, say
	} else if on {
		sub = i18n.T("已锁定 · 走不了就断开", "Locked · cut if no node")
	}
	b := ui.ButtonBase(c.Key("lock")).Column().Gap(1).Shrink(0).Padding(9, 12).Radius(p.r(8)).AlignItems(ui.Start).
		Label(i18n.T("出口锁定", "Exit lock")).Tooltip(tip)
	switch {
	case on && p.brackets():
		picked(b.Border(1, p.hair), p, on)
	case on:
		b.Border(1.5, p.ink)
	default:
		b.Border(1, p.hair)
		if b.Hovered() {
			b.Background(p.hover)
		}
	}
	b.OnClick(func() { u.setLocked(rid, !on) }).Children(func() {
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, lockIcon).Size(12, 12).Shrink(0).TextColor(p.ink)
			ui.Text(c, i18n.T("出口锁定", "Exit lock")).FontSize(13.5).FontWeight(600).TextColor(p.ink).NoWrap()
		})
		ui.Text(c, sub).FontSize(11.5).TextColor(subCol).NoWrap()
	})
}

// lockDialog says what the first lock does before it is put on.
func (u *UI) lockDialog(c *ui.Context, p *pal) {
	if u.lockAsk == "" {
		return
	}
	open := true
	r := u.route(u.lockAsk)
	ui.Modal(c.Key("lock-ask"), &open, func() {
		ui.Column(c).Gap(10).Width(420).Children(func() {
			disp(ui.Text(c, i18n.T("锁定「", "Lock the exit of ")+r.Name+i18n.T("」的出口", "")), p).FontSize(18).FontWeight(700).TextColor(p.ink)
			for _, line := range []string{
				i18n.T("这个路由组的流量只走节点。下面这些时候，它的连接会断开，不会改走本机网络：", "This group's traffic only goes through a node. At these times its connections are cut rather than sent over this machine's network:"),
				i18n.T("· 关闭接管时：其他流量照常直连，只有它被拦下", "· With takeover off: everything else goes direct as usual, only this is refused"),
				i18n.T("· 节点全部连不上时", "· When no node can be reached"),
				i18n.T("· 在全局或直连模式下，它仍走自己的节点", "· In global or direct mode it still goes by its own node"),
				"· " + proxyOnlySay() + i18n.T("。要全部管住，请开着 TUN", ". To hold everything, keep TUN on"),
				i18n.T("关闭接管后，或者只开了 TUN 而 TUN 没有生效时，系统代理会指向 bowel 来守住它。退出 bowel 后不再拦截。", "With takeover off, or with only TUN on and TUN not working, the system proxy points at bowel to hold it. Once bowel quits, nothing is refused."),
			} {
				ui.Text(c, line).FontSize(12.5).TextColor(p.ink2)
			}
			ui.Row(c).Gap(8).Margin(4, 0, 0, 0).Children(func() {
				rid := u.lockAsk
				pill(c.Key("lock-yes"), p, i18n.T("锁定", "Lock"), true).OnClick(func() {
					u.lockAsk = ""
					u.changeRouting(func(s *settings.Settings) { s.Routing.SetLocked(rid, true) }, func() { u.app.closeRoute(rid) })
				})
				btn(c.Key("lock-no"), p, i18n.T("取消", "Cancel")).OnClick(func() { u.lockAsk = "" })
			})
		})
	})
	if !open {
		u.lockAsk = ""
	}
}
