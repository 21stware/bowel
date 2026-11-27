package main

// A route for one device of the gateway (settings.GatewayDevice): all of a
// device's traffic to one route group, direct or blocked, or the device left
// out of the gateway altogether.
//
// The core tells a device by its address (core.DeviceRoute), which is all
// it sees of a forwarded connection. The rest is keeping that address the
// device's: bowel's DHCP keeps it for the device's hardware address and
// gives it to no other (dhcp.Reservation), and when the device turns up at
// another address anyway (the router's DHCP gave it one, or it was set by
// hand) its route follows it there. When its address is seen with another
// device, the route waits for it rather than take that one.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/dhcp"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

// Words of a device's route, as the command line and agents give it.
const (
	deviceRules  = "rules"  // no route of its own
	deviceBypass = "bypass" // left out of the gateway
)

// deviceRoutes are the devices' routes for the core, while the gateway is
// in effect: a device left out goes direct when it still comes through.
func (a *App) deviceRoutes(s settings.Settings) []core.DeviceRoute {
	if !a.gatewayOn(s) {
		return nil
	}
	var out []core.DeviceRoute
	for _, d := range s.GatewayDevices {
		if d.IP == "" {
			continue
		}
		r := core.DeviceRoute{IP: d.IP, Route: d.Route}
		if d.Bypass {
			r.Route = ""
		}
		out = append(out, r)
	}
	return out
}

// reservations are the addresses bowel's DHCP keeps for the devices with a
// route of their own.
func reservations(s settings.Settings) []dhcp.Reservation {
	var out []dhcp.Reservation
	for _, d := range s.GatewayDevices {
		out = append(out, dhcp.Reservation{MAC: d.MAC, IP: d.Addr(), Bypass: d.Bypass})
	}
	return out
}

// syncReservations hands the running DHCP server the reservations as the
// settings have them.
func (a *App) syncReservations() {
	a.auto.mu.Lock()
	srv := a.auto.srv
	a.auto.mu.Unlock()
	if srv != nil {
		srv.Reserve(reservations(a.settings.Get()))
	}
}

// followMu lets one change of the devices' addresses be saved at a time:
// the next look at the gateway does what is left.
var followMu sync.Mutex

// followDevices moves the devices' routes along with the devices, by what
// is known of the LAN now: the leases bowel gave and the ARP table.
func (a *App) followDevices(now time.Time, leases []dhcp.Lease, neigh map[netip.Addr]string) {
	s := a.settings.Get()
	if len(s.GatewayDevices) == 0 {
		return
	}
	at, who := whereDevices(now, leases, neigh)
	if _, changed := followed(s.GatewayDevices, at, who); !changed || !followMu.TryLock() {
		a.syncReservations()
		return
	}
	go func() {
		defer followMu.Unlock()
		var moves []string
		err := a.updateSettings(func(st *settings.Settings) {
			next, _ := followed(st.GatewayDevices, at, who)
			for i, d := range next {
				if old := st.GatewayDevices[i]; old.IP != d.IP {
					moves = append(moves, fmt.Sprintf("%s %s → %s", cmpOr(d.MAC, d.Name), cmpOr(old.IP, "—"), cmpOr(d.IP, "—")))
				}
			}
			st.GatewayDevices = next
		})
		if err != nil {
			log.Println("gateway: a device's route didn't follow it:", err)
		} else if len(moves) > 0 {
			log.Println("gateway: devices' routes follow them:", strings.Join(moves, ", "))
		}
		a.syncReservations()
		a.emit()
	}()
}

func cmpOr(s, or string) string {
	if s == "" {
		return or
	}
	return s
}

// whereDevices is where each hardware address is now (at) and whose each
// address is (who): an active lease of bowel's says so first, the ARP
// table otherwise. A hardware address the table has at several addresses
// says nothing of either: a Wi-Fi repeater that puts its own on every
// device behind it, or a router answering for others.
func whereDevices(now time.Time, leases []dhcp.Lease, neigh map[netip.Addr]string) (at map[string]netip.Addr, who map[netip.Addr]string) {
	at, who = map[string]netip.Addr{}, map[netip.Addr]string{}
	for ip, mac := range neigh {
		if ip.Is4() && mac != "" && !sharedMAC(neigh, mac) {
			at[mac], who[ip] = ip, mac
		}
	}
	for _, l := range leases {
		if l.Active(now) {
			at[l.MAC], who[l.IP] = l.IP, l.MAC
		}
	}
	return at, who
}

// followed is list with each device at its address now: a device known by
// its hardware address moves where that is seen, and waits (no address)
// while another device is seen at its address; one known by its address
// alone learns its hardware address from it.
func followed(list []settings.GatewayDevice, at map[string]netip.Addr, who map[netip.Addr]string) ([]settings.GatewayDevice, bool) {
	next := slices.Clone(list)
	changed := false
	held := func(ip string, but int) bool {
		return slices.ContainsFunc(next, func(o settings.GatewayDevice) bool { return o.IP == ip }) && next[but].IP != ip
	}
	for i, d := range next {
		if d.MAC == "" {
			if m := who[d.Addr()]; m != "" && !slices.ContainsFunc(next, func(o settings.GatewayDevice) bool { return o.MAC == m }) {
				next[i].MAC, changed = m, true
			}
			continue
		}
		cur, seen := at[d.MAC]
		switch {
		case seen && cur.String() != d.IP && !held(cur.String(), i):
			next[i].IP, changed = cur.String(), true
		case !seen && d.IP != "" && who[d.Addr()] != "" && who[d.Addr()] != d.MAC:
			next[i].IP, changed = "", true
		}
	}
	return next, changed
}

// withRoutes puts the devices' routes on the devices as the interface shows
// them, and adds those with a route that have not been seen.
func (a *App) withRoutes(views []DeviceView, s settings.Settings) []DeviceView {
	names := map[string]string{}
	for _, r := range a.routes() {
		names[r.ID] = routes.Title(r)
	}
	for _, d := range s.GatewayDevices {
		i := slices.IndexFunc(views, func(v DeviceView) bool { return sameDevice(d, v) })
		if i < 0 {
			views = append(views, DeviceView{IP: d.IP, MAC: d.MAC, Name: d.Name, Via: "manual", Events: []DeviceEvent{}})
			i = len(views) - 1
		}
		v := &views[i]
		v.Route, v.Bypass, v.Away = d.Route, d.Bypass, d.IP == ""
		v.RouteName = names[d.Route]
		if v.Name == "" {
			v.Name = d.Name
		}
	}
	for i := range views {
		views[i].Note = deviceNote(views[i], s)
	}
	return views
}

// sameDevice says whether v is the device d is the route of: the same
// hardware address, or the same address where one of the two has none.
func sameDevice(d settings.GatewayDevice, v DeviceView) bool {
	if d.MAC != "" && v.MAC != "" {
		return d.MAC == v.MAC
	}
	return d.IP != "" && d.IP == v.IP
}

// findDevice is the device s names, among those seen and those with a
// route: its address, its hardware address, or its name. An address on the
// LAN that no device has been seen at is taken too, for a device to come.
func (a *App) findDevice(s string) (settings.GatewayDevice, error) {
	s = strings.TrimSpace(s)
	st := a.settings.Get()
	views := a.withRoutes(a.devs.view(nil), st)
	mac := settings.CanonMAC(s)
	found := func(v DeviceView) (settings.GatewayDevice, error) {
		d := settings.GatewayDevice{IP: v.IP, MAC: v.MAC, Name: v.Name}
		// Not from a lease: the ARP table's, which a repeater may share.
		if v.Via != "auto" && d.IP != "" && (d.MAC == "" || sharedMAC(a.neighbours(), d.MAC)) {
			d.MAC = a.ownMAC(d.IP)
		}
		return d, nil
	}
	var named []DeviceView
	for _, v := range views {
		if v.IP != "" && v.IP == s || mac != "" && v.MAC == mac {
			return found(v)
		}
		if s != "" && strings.EqualFold(v.Name, s) {
			named = append(named, v)
		}
	}
	switch len(named) {
	case 0:
	case 1:
		return found(named[0])
	default:
		return settings.GatewayDevice{}, fmt.Errorf(i18n.T("有多台设备叫 %s，请用它的地址", "Several devices are called %s; use the address"), s)
	}
	ip, err := netip.ParseAddr(s)
	if err != nil || !ip.Is4() {
		return settings.GatewayDevice{}, fmt.Errorf(i18n.T("没有这台设备: %s（用 bowel gateway devices 看看有哪些）", "No such device: %s (bowel gateway devices lists them)"), s)
	}
	n := a.gw.System().Network()
	if self, lan, ok := osDHCP.lan(n); ok {
		switch {
		case !lan.Contains(ip) || ip == lan.Addr() || ip == self || ip.String() == n.Router:
			return settings.GatewayDevice{}, fmt.Errorf(i18n.T("%s 不是这个局域网（%s）里的设备地址", "%s isn't a device's address on this LAN (%s)"), s, lan)
		}
	}
	return settings.GatewayDevice{IP: ip.String(), MAC: a.ownMAC(ip.String())}, nil
}

// deviceChange is a device's route as asked for: the device and what it is
// to have, nil for no route of its own.
type deviceChange struct {
	dev  settings.GatewayDevice
	want *settings.GatewayDevice
}

// planDevice reads a device and its route as given: a route group's id or
// name, bypass, or rules.
func (a *App) planDevice(who, route string) (deviceChange, error) {
	d, err := a.findDevice(who)
	if err != nil {
		return deviceChange{}, err
	}
	ch := deviceChange{dev: d}
	switch r := strings.TrimSpace(route); strings.ToLower(r) {
	case deviceRules, "none", "":
		return ch, nil
	case deviceBypass:
		w := d
		w.Bypass = true
		ch.want = &w
	default:
		if a.importedNow() != nil {
			return deviceChange{}, errors.New(i18n.T("现在用的是 Provider 自带的分流，设备的路由只能指向 bowel 自己的路由组", "A provider's own routing is in use: a device's route can only name bowel's own route groups"))
		}
		rt, err := a.findRoute(r)
		if err != nil {
			return deviceChange{}, err
		}
		w := d
		w.Route = rt.ID
		ch.want = &w
	}
	return ch, nil
}

// apply puts the change into list: the device's entry replaced, or gone.
func (ch deviceChange) apply(list []settings.GatewayDevice) []settings.GatewayDevice {
	i := slices.IndexFunc(list, func(o settings.GatewayDevice) bool {
		return ch.dev.MAC != "" && o.MAC == ch.dev.MAC || ch.dev.IP != "" && o.IP == ch.dev.IP
	})
	if ch.want == nil {
		if i >= 0 {
			list = slices.Delete(slices.Clone(list), i, i+1)
		}
		return list
	}
	w := *ch.want
	if i >= 0 {
		list = slices.Clone(list)
		if w.MAC == "" {
			w.MAC = list[i].MAC
		}
		if w.IP == "" {
			w.IP = list[i].IP
		}
		list[i] = w
		return list
	}
	return append(slices.Clone(list), w)
}

// describe is the change in words, for the dialog that asks the user.
func (ch deviceChange) describe(a *App) string {
	who := ch.dev.IP
	if ch.dev.Name != "" {
		who = ch.dev.Name + " (" + cmpOr(ch.dev.IP, ch.dev.MAC) + ")"
	}
	switch {
	case ch.want == nil:
		return fmt.Sprintf(i18n.T("%s：按规则分流，和其他设备一样", "%s: follows the rules, as every device does"), who)
	case ch.want.Bypass:
		return fmt.Sprintf(i18n.T("%s：不经过网关", "%s: left out of the gateway"), who)
	}
	return fmt.Sprintf(i18n.T("%s：全部流量走「%s」", "%s: all its traffic to %s"), who, routes.Title(a.route(ch.want.Route)))
}

// setDeviceRoute saves a device's route and applies it: the core's rules,
// the DHCP server's reservations, and the device's open connections closed,
// so that what it opens next goes the new way. It returns the device as the
// interface shows it and how many connections were closed.
func (a *App) setDeviceRoute(ch deviceChange) (DeviceView, int, error) {
	done := a.begin("gateway:device")
	defer done()
	if err := a.updateSettings(func(s *settings.Settings) { s.GatewayDevices = ch.apply(s.GatewayDevices) }); err != nil {
		return DeviceView{}, 0, err
	}
	a.syncReservations()
	closed := 0
	if ch.dev.IP != "" {
		closed = a.closeFrom(ch.dev.IP)
	}
	a.emit()
	d := settings.GatewayDevice{IP: ch.dev.IP, MAC: ch.dev.MAC}
	for _, v := range a.withRoutes(a.devs.view(a.mon.Conns()), a.settings.Get()) {
		if sameDevice(d, v) {
			return v, closed, nil
		}
	}
	return DeviceView{IP: ch.dev.IP, MAC: ch.dev.MAC, Name: ch.dev.Name, Via: "manual", Events: []DeviceEvent{}}, closed, nil
}

// closeFrom closes the connections that come from ip, and says how many.
func (a *App) closeFrom(ip string) int {
	c, err := a.core.Client()
	if err != nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	snap, err := c.Connections(ctx)
	if err != nil {
		return 0
	}
	var ids []string
	for _, cn := range snap.Connections {
		if src, err := netip.ParseAddr(cn.Metadata.SourceIP); err == nil && src.Unmap().String() == ip {
			ids = append(ids, cn.ID)
		}
	}
	if len(ids) > 0 && a.closeConns(ids) == nil {
		a.mon.Wake()
		return len(ids)
	}
	return 0
}

// deviceNote is what the user or an agent should know of a device's route
// with settings s, "" when nothing: when a device left out learns of it,
// and that one pointed here by hand can only be sent direct.
func deviceNote(v DeviceView, s settings.Settings) string {
	if v.Route == "" && !v.Bypass {
		return ""
	}
	switch {
	case !s.Gateway:
		return i18n.T("网关没有打开：打开后才生效", "The gateway is off: this takes effect once it is on")
	case v.Away:
		return i18n.T("它的地址现在是另一台设备的：它再出现时路由会跟过去", "Its address is another device's now: the route follows it when it shows up again")
	case v.Bypass && s.GatewayAuto && v.Via == "auto":
		return i18n.T("bowel 给它的网关和 DNS 是路由器，它不经过这里（改了之后最晚下次续租生效，10 分钟内；重连 Wi-Fi 立刻生效）", "bowel gives it the router as its gateway and DNS, so it doesn't come through here (from its next renewal after a change, within 10 minutes; at once if its Wi-Fi reconnects)")
	case v.Bypass:
		return i18n.T("它是手动指向这里的，bowel 没法让它离开：它的流量现在直连。要它完全不经过这里，把它的网关和 DNS 改回路由器", "It was pointed here by hand, which bowel can't undo: its traffic goes direct now. To take it out altogether, set its router and DNS back to the router")
	case v.Route != "" && s.Mode != "rule":
		return i18n.T("设备的路由是规则：现在不是规则模式，要切回规则模式才生效", "A device's route is a rule: it takes effect in rule mode, and the mode is another now")
	}
	return ""
}

// sharedMAC says whether the ARP table has mac at more than one address.
func sharedMAC(neigh map[netip.Addr]string, mac string) bool {
	n := 0
	for ip, m := range neigh {
		if m == mac && ip.Is4() {
			n++
		}
	}
	return n > 1
}

// ownMAC is the hardware address the ARP table has at ip, "" when none or
// when it has that one at other addresses too.
func (a *App) ownMAC(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ""
	}
	neigh := a.neighbours()
	if m := neigh[addr]; m != "" && !sharedMAC(neigh, m) {
		return m
	}
	return ""
}
