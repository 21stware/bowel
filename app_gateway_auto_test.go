package main

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/oboo/moho/internal/dhcp"
	"github.com/oboo/moho/internal/gateway"
	"github.com/oboo/moho/internal/monitor"
	"github.com/oboo/moho/internal/settings"
)

// No test of this package sends a packet to the LAN it runs on: the probe
// and the server's socket are fakes unless a test says otherwise.
func init() {
	osDHCP.probe = func(context.Context, string, netip.Addr, time.Duration) ([]netip.Addr, error) {
		return nil, errors.New("no LAN in tests")
	}
	osDHCP.listen = func(*dhcp.Server) (net.PacketConn, error) { return nil, errors.New("no LAN in tests") }
}

// fakeLAN makes the automatic step see the servers given, and lets bowel's
// own server open a socket of the loopback's, which nothing reaches.
func fakeLAN(t *testing.T, others *[]netip.Addr) {
	saved := osDHCP
	osDHCP.supported = true
	osDHCP.probe = func(context.Context, string, netip.Addr, time.Duration) ([]netip.Addr, error) { return *others, nil }
	osDHCP.listen = func(*dhcp.Server) (net.PacketConn, error) { return net.ListenPacket("udp4", "127.0.0.1:0") }
	osDHCP.lan = func(gateway.Network) (netip.Addr, netip.Prefix, bool) {
		return netip.MustParseAddr("192.168.1.20"), netip.MustParsePrefix("192.168.1.0/24"), true
	}
	t.Cleanup(func() { osDHCP = saved })
}

// Automatic is refused while another server hands out addresses, turns on
// when none does, says so when one comes back, and ends with the gateway.
func TestGatewayAuto(t *testing.T) {
	installed, dialogs := true, 0
	fakeTun(t, &installed, &dialogs)
	var others []netip.Addr
	fakeLAN(t, &others)
	f := &gwFake{ready: true}
	a := gatewayApp(t, f)
	a.emitQueued = true // this bare App has nothing to make a snapshot of
	f.net = gateway.Network{IP: "192.168.1.20", Iface: "en0", Router: "192.168.1.1"}
	if _, err := a.settings.Update(func(s *settings.Settings) { s.Gateway, s.Tun, s.Paused = true, true, false }); err != nil {
		t.Fatal(err)
	}
	view := func() AutoView { return a.autoView(a.settings.Get(), true) }

	// Not while the gateway is off.
	if _, err := a.settings.Update(func(s *settings.Settings) { s.Paused = true }); err != nil {
		t.Fatal(err)
	}
	if err := a.setGatewayAuto(true); err == nil {
		t.Fatal("automatic with the gateway paused")
	}
	if _, err := a.settings.Update(func(s *settings.Settings) { s.Paused = false }); err != nil {
		t.Fatal(err)
	}

	// Not while the router answers.
	others = []netip.Addr{netip.MustParseAddr("192.168.1.1")}
	err := a.setGatewayAuto(true)
	if err == nil || !strings.Contains(err.Error(), "192.168.1.1") || a.settings.Get().GatewayAuto {
		t.Fatalf("with the router's DHCP on: %v", err)
	}
	if v := view(); v.Wanted || v.Status != "off" || len(v.Servers) != 1 || v.Servers[0] != "192.168.1.1" || v.Checked.IsZero() {
		t.Fatalf("view: %+v", v)
	}

	// Its DHCP is off: on.
	others = nil
	if err := a.setGatewayAuto(true); err != nil {
		t.Fatal(err)
	}
	if v := view(); !v.Wanted || v.Status != "on" || v.Pool == "" || !a.settings.Get().GatewayAuto {
		t.Fatalf("on: %+v", v)
	}

	// The router's comes back: an error to show, and still serving.
	others = []netip.Addr{netip.MustParseAddr("192.168.1.1")}
	if _, err := a.checkDHCP(); err != nil {
		t.Fatal(err)
	}
	if v := view(); v.Status != "error" || !strings.Contains(v.Detail, "192.168.1.1") {
		t.Fatalf("a second server: %+v", v)
	}

	// Pausing the takeover stops the server and keeps the choice.
	if _, err := a.settings.Update(func(s *settings.Settings) { s.Paused = true }); err != nil {
		t.Fatal(err)
	}
	a.syncGateway()
	if a.leases() != nil || !a.settings.Get().GatewayAuto {
		t.Fatal("paused: still serving, or the choice forgotten")
	}
	if _, err := a.settings.Update(func(s *settings.Settings) { s.Paused = false }); err != nil {
		t.Fatal(err)
	}
	a.syncGateway()
	a.auto.mu.Lock()
	serving := a.auto.srv != nil
	a.auto.mu.Unlock()
	if !serving {
		t.Fatal("resumed: not serving")
	}

	// Turning the gateway off ends it, and the next time starts at manual.
	if _, err := a.settings.Update(func(s *settings.Settings) { wantGateway(s, false) }); err != nil {
		t.Fatal(err)
	}
	a.syncGateway()
	a.auto.mu.Lock()
	serving = a.auto.srv != nil
	a.auto.mu.Unlock()
	if serving || a.settings.Get().GatewayAuto {
		t.Fatal("off: still serving, or still automatic")
	}
}

// Devices: one that has connections is there, by hand unless bowel leased
// its address; silent for long enough it is gone, and both are recorded.
func TestDevices(t *testing.T) {
	d := &devices{}
	now := time.Date(2026, 11, 13, 20, 0, 0, 0, time.UTC)
	tv, phone := netip.MustParseAddr("192.168.1.100"), netip.MustParseAddr("192.168.1.40")
	lease := dhcp.Lease{MAC: "02:00:00:00:00:01", IP: tv, Name: "tv", Expires: now.Add(20 * time.Minute), Seen: now}
	d.update(now, []netip.Addr{phone}, []dhcp.Lease{lease}, map[netip.Addr]string{phone: "02:00:00:00:00:02"})
	conns := []monitor.Conn{{Source: phone.String(), Status: "active", Down: 100, Up: 10}, {Source: phone.String(), Status: "closed", Down: 5}}
	v := d.view(conns)
	if len(v) != 2 || v[0].IP != phone.String() || v[0].Via != "manual" || !v[0].Online || v[0].MAC != "02:00:00:00:00:02" || v[0].Conns != 1 || v[0].Down != 105 ||
		v[1].IP != tv.String() || v[1].Via != "auto" || v[1].Name != "tv" || !v[1].Online {
		t.Fatalf("%+v", v)
	}
	if len(v[0].Events) != 1 || v[0].Events[0].Kind != "on" || v[0].Events[0].Note != "first" {
		t.Fatalf("events: %+v", v[0].Events)
	}
	// The phone goes quiet; the TV renews.
	later := now.Add(deviceGone + time.Minute)
	lease.Seen, lease.Expires = later.Add(-time.Minute), later.Add(19*time.Minute)
	d.update(later, nil, []dhcp.Lease{lease}, nil)
	v = d.view(nil)
	if v[0].IP != tv.String() || !v[0].Online || v[1].Online || len(v[1].Events) != 2 || v[1].Events[0].Kind != "off" || !v[1].Events[0].At.Equal(now) {
		t.Fatalf("after the phone left: %+v", v)
	}
	// And comes back: on again, not for the first time.
	d.update(later.Add(time.Minute), []netip.Addr{phone}, []dhcp.Lease{lease}, nil)
	if v = d.view(nil); !v[0].Online || !v[1].Online {
		t.Fatalf("back: %+v", v)
	}
	for _, x := range v {
		if x.IP == phone.String() && (len(x.Events) != 3 || x.Events[0].Kind != "on" || x.Events[0].Note != "") {
			t.Fatalf("the phone's events: %+v", x.Events)
		}
	}
}
