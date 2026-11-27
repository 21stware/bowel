package main

import (
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oboo/moho/internal/dhcp"
	"github.com/oboo/moho/internal/gateway"
	"github.com/oboo/moho/internal/mcp"
	"github.com/oboo/moho/internal/settings"
)

var (
	tvMAC  = "02:00:00:00:00:0a"
	camMAC = "02:00:00:00:00:0b"
)

func addr(s string) netip.Addr { return netip.MustParseAddr(s) }

// A device's route goes with the device: to where its hardware address is
// seen now, a lease of bowel's first; it waits while another device has
// its address; one known by its address alone learns its hardware address.
func TestDevicesFollowed(t *testing.T) {
	list := []settings.GatewayDevice{
		{IP: "192.168.1.50", MAC: tvMAC, Route: "media"},
		{IP: "192.168.1.60", MAC: camMAC, Bypass: true},
		{IP: "192.168.1.70", Route: "direct"},
	}
	now := time.Now()
	leases := []dhcp.Lease{{MAC: tvMAC, IP: addr("192.168.1.51"), Expires: now.Add(time.Minute)}}
	neigh := map[netip.Addr]string{
		addr("192.168.1.50"): tvMAC,               // an old entry: the lease says otherwise
		addr("192.168.1.60"): "02:00:00:00:00:99", // another device has the camera's address now
		addr("192.168.1.70"): "02:00:00:00:00:0c", // the one with an address alone
		addr("192.168.1.80"): "02:00:00:00:00:0d", // nobody's
	}
	at, who := whereDevices(now, leases, neigh)
	got, changed := followed(list, at, who)
	want := []settings.GatewayDevice{
		{IP: "192.168.1.51", MAC: tvMAC, Route: "media"},
		{IP: "", MAC: camMAC, Bypass: true},
		{IP: "192.168.1.70", MAC: "02:00:00:00:00:0c", Route: "direct"},
	}
	if !changed || !slices.Equal(got, want) {
		t.Fatalf("followed: %v %+v", changed, got)
	}
	st := settings.Defaults.Clone()
	st.GatewayDevices = got
	if err := st.Validate(); err != nil {
		t.Fatalf("what follows isn't valid: %v", err)
	}
	// The camera comes back at another address.
	neigh[addr("192.168.1.61")] = camMAC
	at, who = whereDevices(now, leases, neigh)
	if got, _ = followed(got, at, who); got[1].IP != "192.168.1.61" {
		t.Fatalf("the camera came back: %+v", got[1])
	}
	// Nothing changes when all is where it was.
	if _, changed := followed(got, at, who); changed {
		t.Fatal("a change with nothing moved")
	}
	// A device does not take an address another device's route has.
	at[tvMAC] = addr("192.168.1.70")
	if again, _ := followed(got, at, who); again[0].IP != "192.168.1.51" {
		t.Fatalf("moved onto another route's address: %+v", again[0])
	}
}

// A hardware address the ARP table has at several addresses (a repeater's,
// put on every device behind it) moves no route and is learnt by none.
func TestDevicesFollowSharedMAC(t *testing.T) {
	repeater := "02:00:00:00:00:ee"
	list := []settings.GatewayDevice{{IP: "192.168.1.50", MAC: repeater, Route: "media"}, {IP: "192.168.1.70", Route: "direct"}, {IP: "192.168.1.80", MAC: tvMAC, Route: "media"}}
	neigh := map[netip.Addr]string{addr("192.168.1.51"): repeater, addr("192.168.1.70"): repeater, addr("192.168.1.80"): repeater}
	at, who := whereDevices(time.Now(), nil, neigh)
	if got, changed := followed(list, at, who); changed {
		t.Fatalf("a shared hardware address moved a route: %+v", got)
	}
}

// The core is given the devices only while the gateway is in effect; one
// left out goes direct, one waiting for its address is not given.
func TestDeviceRoutesWithTheGateway(t *testing.T) {
	installed, dialogs := true, 0
	fakeTun(t, &installed, &dialogs)
	a := gatewayApp(t, &gwFake{ready: true})
	if _, err := a.settings.Update(func(s *settings.Settings) {
		s.Gateway, s.Tun, s.Paused = true, true, false
		s.GatewayDevices = []settings.GatewayDevice{{IP: "192.168.1.50", Route: "media"}, {IP: "192.168.1.60", Bypass: true}, {MAC: camMAC, Route: "reject"}}
	}); err != nil {
		t.Fatal(err)
	}
	got := a.deviceRoutes(a.settings.Get())
	if len(got) != 2 || got[0].IP != "192.168.1.50" || got[0].Route != "media" || got[1].IP != "192.168.1.60" || got[1].Route != "" {
		t.Fatalf("with the gateway: %+v", got)
	}
	setPaused(t, a, true)
	if got := a.deviceRoutes(a.settings.Get()); got != nil {
		t.Fatalf("paused: %+v", got)
	}
}

// The DHCP server keeps the devices' addresses from its start, and is
// told of a change.
func TestDeviceReservations(t *testing.T) {
	a, _, _ := autoApp(t)
	if _, err := a.settings.Update(func(s *settings.Settings) {
		s.GatewayDevices = []settings.GatewayDevice{{IP: "192.168.1.50", MAC: tvMAC, Route: "media"}}
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.setGatewayAuto(true); err != nil {
		t.Fatal(err)
	}
	srv := a.auto.srv
	if r := srv.Reservations(); len(r) != 1 || r[0].MAC != tvMAC || r[0].IP != addr("192.168.1.50") || r[0].Bypass {
		t.Fatalf("at the start: %+v", r)
	}
	if _, err := a.settings.Update(func(s *settings.Settings) { s.GatewayDevices[0].Route, s.GatewayDevices[0].Bypass = "", true }); err != nil {
		t.Fatal(err)
	}
	a.syncReservations()
	if r := srv.Reservations(); len(r) != 1 || !r[0].Bypass {
		t.Fatalf("after a change: %+v", r)
	}
}

// The devices list shows each device's route, and the devices with a route
// that have not come through yet.
func TestDevicesWithRoutes(t *testing.T) {
	a, _, _ := mcpApp(t)
	s := a.settings.Get()
	s.GatewayDevices = []settings.GatewayDevice{{IP: "192.168.1.50", MAC: tvMAC, Route: "media"}, {IP: "192.168.1.90", Name: "printer", Bypass: true}, {MAC: camMAC, Route: "reject"}}
	views := a.withRoutes([]DeviceView{{IP: "192.168.1.50", MAC: tvMAC, Via: "auto", Online: true}, {IP: "192.168.1.51"}}, s)
	if len(views) != 4 {
		t.Fatalf("%d devices: %+v", len(views), views)
	}
	if v := views[0]; v.Route != "media" || v.RouteName == "" || v.Bypass || v.Away {
		t.Errorf("the TV: %+v", v)
	}
	if v := views[1]; v.Route != "" || v.Bypass {
		t.Errorf("a device without a route: %+v", v)
	}
	if v := views[2]; v.IP != "192.168.1.90" || v.Name != "printer" || !v.Bypass || v.Online {
		t.Errorf("the printer, not seen yet: %+v", v)
	}
	if v := views[3]; !v.Away || v.MAC != camMAC || v.Route != "reject" {
		t.Errorf("the camera, away: %+v", v)
	}
}

// set_device_route follows the policy, takes a device by its address, its
// hardware address or its name, and refuses what is not a device of the
// LAN or not a route.
func TestSetDeviceRouteTool(t *testing.T) {
	saved := osGateway
	osGateway = func() gateway.System {
		return &gwFake{ready: true, net: gateway.Network{IP: "192.168.1.20", Iface: "en0", Router: "192.168.1.1"}}
	}
	t.Cleanup(func() { osGateway = saved })
	l := fakeLAN(t)
	l.set(func(l *lanFake) { l.neigh[addr("192.168.1.50")] = tvMAC })
	a, asks, answer := mcpApp(t)
	srv := a.mcpServer()
	s := &mcp.Session{}
	set := func(device, route string) (map[string]any, error) {
		return callTool(t, srv, s, "set_device_route", map[string]any{"device": device, "route": route})
	}

	setPolicy(t, a, "deny")
	if _, err := set("192.168.1.50", "media"); err == nil || len(a.settings.Get().GatewayDevices) != 0 {
		t.Fatalf("deny let it through: %v", err)
	}
	setPolicy(t, a, "ask")
	*answer = [2]bool{true, false}
	out, err := set("192.168.1.50", "流媒体")
	if err != nil || *asks != 1 {
		t.Fatalf("asked %d times: %v", *asks, err)
	}
	if d := a.settings.Get().GatewayDevices; len(d) != 1 || d[0] != (settings.GatewayDevice{IP: "192.168.1.50", MAC: tvMAC, Route: "media"}) {
		t.Fatalf("saved: %+v", d)
	}
	if dev := out["device"].(map[string]any); dev["route"] != "media" || dev["mac"] != tvMAC {
		t.Fatalf("answer: %v", out)
	}
	// By its hardware address, left out; then back to the rules.
	setPolicy(t, a, "allow")
	if _, err := set(strings.ToUpper(tvMAC), "bypass"); err != nil {
		t.Fatal(err)
	}
	if d := a.settings.Get().GatewayDevices; len(d) != 1 || !d[0].Bypass || d[0].Route != "" {
		t.Fatalf("left out: %+v", d)
	}
	if _, err := set("192.168.1.50", "rules"); err != nil || len(a.settings.Get().GatewayDevices) != 0 {
		t.Fatalf("back to the rules: %v %+v", err, a.settings.Get().GatewayDevices)
	}
	for _, bad := range [][2]string{{"10.0.0.5", "media"}, {"192.168.1.1", "media"}, {"192.168.1.20", "direct"}, {"tv", "media"}, {"192.168.1.50", "nowhere"}} {
		if _, err := set(bad[0], bad[1]); err == nil {
			t.Errorf("%v was taken", bad)
		}
	}
	if d := a.settings.Get().GatewayDevices; len(d) != 0 {
		t.Fatalf("a refused change was saved: %+v", d)
	}
}
