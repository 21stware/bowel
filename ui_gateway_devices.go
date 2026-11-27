package main

// A device's own route, where the devices are listed (日志 → 设备): the
// row says it, and the device's detail chooses it.

import (
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/routes"
)

// devKey tells the devices' rows apart: the address, or the hardware
// address of a device whose route waits for it without one.
func devKey(d DeviceView) string {
	if d.IP != "" {
		return d.IP
	}
	return d.MAC
}

// deviceRouteWord is a device's route in a word or two, "" when it follows
// the rules as every device does.
func deviceRouteWord(d DeviceView) string {
	switch {
	case d.Bypass:
		return i18n.T("不经过网关", "Left out")
	case d.Route != "" && d.RouteName != "":
		return "→ " + d.RouteName
	case d.Route != "":
		return "→ " + d.Route
	}
	return ""
}

// actDevRouteTag is the route beside the device's name in its row.
func actDevRouteTag(c *ui.Context, p *pal, d DeviceView) {
	if w := deviceRouteWord(d); w != "" {
		col := p.ink2
		if d.Away {
			col = p.faint
		}
		p.lab(ui.Text(c, w)).FontSize(11.5).FontWeight(600).TextColor(col).NoWrap().Shrink(0)
	}
}

// deviceRouteChoices are what a device's route can be, in words and as
// set_device_route takes them: the rules, each route group in the list's
// order (the default one last), or left out of the gateway. A provider's
// own routing has none of bowel's groups to offer.
func deviceRouteChoices(all []RouteView, imported bool) (labels, ids []string) {
	labels, ids = []string{i18n.T("按规则分流（和其他设备一样）", "Follow the rules (as every device)")}, []string{deviceRules}
	if !imported {
		for _, last := range []bool{false, true} {
			for _, r := range all {
				if (r.ID == routes.Default) == last {
					labels, ids = append(labels, i18n.T("全部走 ", "All to ")+r.Name), append(ids, r.ID)
				}
			}
		}
	}
	return append(labels, i18n.T("不经过网关", "Leave out of the gateway")), append(ids, deviceBypass)
}

// actDevRoute chooses the device's route, in its detail.
func (u *UI) actDevRoute(c *ui.Context, p *pal, d DeviceView) {
	labels, ids := deviceRouteChoices(u.snap.Routes, u.snap.Imported != nil)
	at := 0
	switch {
	case d.Bypass:
		at = len(ids) - 1
	case d.Route != "":
		at = max(0, slices.Index(ids, d.Route))
	}
	cur := labels[at]
	who := d.IP
	if who == "" || d.MAC != "" {
		who = d.MAC
	}
	ui.Row(c.Key("dev-route-"+who)).Gap(10).AlignItems(ui.Center).Margin(0, 0, 6, 0).Children(func() {
		ui.Text(c, i18n.T("路由", "Route")).FontSize(11.5).FontWeight(600).TextColor(p.mute).Shrink(0)
		sel := ui.Select(c.Key("pick"), &cur, labels).Label(i18n.T("这台设备的路由", "This device's route")).Width(230)
		if u.pending("gateway:device") {
			sel.Opacity(.45)
		} else if sel.Changed() {
			id := ids[max(0, slices.Index(labels, cur))]
			u.do(func() error {
				ch, err := u.app.planDevice(who, id)
				if err != nil {
					return err
				}
				_, _, err = u.app.setDeviceRoute(ch)
				return err
			})
		}
	})
	if d.Note != "" {
		ui.Text(c, d.Note).FontSize(11.5).TextColor(p.mute).Margin(0, 0, 6, 0)
	} else if d.Route == "" && !d.Bypass {
		ui.Text(c, i18n.T("选一个路由组，这台设备的全部流量就都走它；拦截和锁定出口的路由组仍然对它生效。", "Pick a route group and all of this device's traffic goes there; Block and groups with the exit lock still hold for it.")).
			FontSize(11.5).TextColor(p.faint).Margin(0, 0, 6, 0)
	}
}
