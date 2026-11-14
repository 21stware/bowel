package main

import (
	"context"
	"errors"
	"maps"
	"net"
	"net/netip"
	"slices"
	"strings"
	"sync"
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

// lanFake is the LAN as the automatic step sees it in a test: the other
// servers that answer, how often it was asked, and where this computer is.
type lanFake struct {
	mu      sync.Mutex
	others  []netip.Addr
	probes  int
	self    netip.Addr
	lan     netip.Prefix
	neigh   map[netip.Addr]string
	conn    net.PacketConn // the server's socket, on the loopback
	conns   []*lanTestConn // all servers to join before restoring the fakes
	arpWait time.Duration  // how long reading the ARP table takes
	arps    int            // how often it was read
	// probing, if set, is called while a check listens, with the address
	// the check leaves out, and may add to the servers that answer.
	probing func(skip netip.Addr) []netip.Addr
}

func (l *lanFake) arpReads() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.arps
}

func (l *lanFake) set(fn func(*lanFake)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fn(l)
}

func (l *lanFake) asked() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.probes
}

// lanTestConn accepts test requests on loopback, but never lets the DHCP
// server's broadcast replies escape to the host's LAN.
type lanTestConn struct {
	net.PacketConn
	done chan struct{}
	once sync.Once
}

func (c *lanTestConn) ReadFrom(p []byte) (int, net.Addr, error) {
	n, addr, err := c.PacketConn.ReadFrom(p)
	if err != nil {
		// Serve can no longer handle a packet or call the fake operations.
		c.once.Do(func() { close(c.done) })
	}
	return n, addr, err
}

func (c *lanTestConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	if to, ok := addr.(*net.UDPAddr); !ok || !to.IP.IsLoopback() {
		return 0, errors.New("no LAN in tests")
	}
	return c.PacketConn.WriteTo(p, addr)
}

// fakeLAN puts a fake LAN (192.168.1.0/24 behind 192.168.1.1) in place of
// the real one. The server's socket is one of the loopback's, which only
// the test reaches.
func fakeLAN(t *testing.T) *lanFake {
	l := &lanFake{self: netip.MustParseAddr("192.168.1.20"), lan: netip.MustParsePrefix("192.168.1.0/24"),
		neigh: map[netip.Addr]string{netip.MustParseAddr("192.168.1.1"): "a4:00:00:00:00:01"}}
	saved, retry := osDHCP, dhcpRetry
	dhcpRetry = 0
	osDHCP.supported = true
	osDHCP.probe = func(_ context.Context, _ string, skip netip.Addr, _ time.Duration) ([]netip.Addr, error) {
		l.mu.Lock()
		l.probes++
		found, during := slices.Clone(l.others), l.probing
		l.mu.Unlock()
		if during != nil {
			found = append(found, during(skip)...)
		}
		return slices.DeleteFunc(found, func(x netip.Addr) bool { return x == skip }), nil
	}
	osDHCP.listen = func(*dhcp.Server) (net.PacketConn, error) {
		c, err := net.ListenPacket("udp4", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		conn := &lanTestConn{PacketConn: c, done: make(chan struct{})}
		l.set(func(l *lanFake) {
			l.conn = conn
			l.conns = append(l.conns, conn)
		})
		return conn, nil
	}
	osDHCP.lan = func(gateway.Network) (netip.Addr, netip.Prefix, bool) {
		l.mu.Lock()
		defer l.mu.Unlock()
		return l.self, l.lan, true
	}
	osDHCP.neighbours = func() map[netip.Addr]string {
		l.mu.Lock()
		l.arps++
		wait, m := l.arpWait, maps.Clone(l.neigh)
		l.mu.Unlock()
		time.Sleep(wait)
		return m
	}
	t.Cleanup(func() {
		l.mu.Lock()
		conns := slices.Clone(l.conns)
		l.mu.Unlock()
		for _, c := range conns {
			c.Close()
			<-c.done
		}
		osDHCP, dhcpRetry = saved, retry
	})
	return l
}

// autoApp is an App with the gateway on, on the fake LAN.
func autoApp(t *testing.T) (*App, *lanFake, *gwFake) {
	installed, dialogs := true, 0
	fakeTun(t, &installed, &dialogs)
	l := fakeLAN(t)
	f := &gwFake{ready: true}
	a := gatewayApp(t, f)
	a.emitQueued = true // this bare App has nothing to make a snapshot of
	f.net = gateway.Network{IP: "192.168.1.20", Iface: "en0", Router: "192.168.1.1"}
	if _, err := a.settings.Update(func(s *settings.Settings) { s.Gateway, s.Tun, s.Paused = true, true, false }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.stopDHCP)
	return a, l, f
}

func (a *App) serving() bool {
	a.auto.mu.Lock()
	defer a.auto.mu.Unlock()
	return a.auto.srv != nil
}

// settle waits for a check that syncDHCP started to end, and for what it
// leads to.
func settle(t *testing.T, a *App, want bool) {
	t.Helper()
	for deadline := time.Now().Add(3 * time.Second); ; time.Sleep(2 * time.Millisecond) {
		a.auto.mu.Lock()
		busy := a.auto.starting || a.auto.checking
		a.auto.mu.Unlock()
		if !busy && a.serving() == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("serving = %v, want %v (busy %v)", a.serving(), want, busy)
		}
	}
}

func setPaused(t *testing.T, a *App, paused bool) {
	t.Helper()
	if _, err := a.settings.Update(func(s *settings.Settings) { s.Paused = paused }); err != nil {
		t.Fatal(err)
	}
	a.syncGateway()
}

// Automatic is refused while another server hands out addresses, turns on
// when none does, says so when one comes back, and ends with the gateway.
func TestGatewayAuto(t *testing.T) {
	a, l, _ := autoApp(t)
	view := func() AutoView { return a.autoView(a.settings.Get(), true) }
	router := netip.MustParseAddr("192.168.1.1")

	// Not while the gateway is off.
	setPaused(t, a, true)
	if err := a.setGatewayAuto(true); err == nil {
		t.Fatal("automatic with the gateway paused")
	}
	setPaused(t, a, false)

	// Not while the router answers.
	l.set(func(l *lanFake) { l.others = []netip.Addr{router} })
	err := a.setGatewayAuto(true)
	if err == nil || !strings.Contains(err.Error(), "192.168.1.1") || a.settings.Get().GatewayAuto {
		t.Fatalf("with the router's DHCP on: %v", err)
	}
	if v := view(); v.Wanted || v.Status != "off" || len(v.Servers) != 1 || v.Servers[0] != "192.168.1.1" || v.Checked.IsZero() {
		t.Fatalf("view: %+v", v)
	}

	// Its DHCP is off: on, at once, and the LAN is remembered.
	l.set(func(l *lanFake) { l.others = nil })
	if err := a.setGatewayAuto(true); err != nil {
		t.Fatal(err)
	}
	s := a.settings.Get()
	if v := view(); !v.Wanted || v.Status != "on" || v.Pool == "" || !s.GatewayAuto || s.GatewayAutoLAN != "192.168.1.0/24 192.168.1.1 a4:00:00:00:00:01" {
		t.Fatalf("on: %+v, LAN %q", v, s.GatewayAutoLAN)
	}

	// The router's comes back: an error to show, and still serving.
	l.set(func(l *lanFake) { l.others = []netip.Addr{router} })
	if _, err := a.checkDHCP(); err != nil {
		t.Fatal(err)
	}
	if v := view(); v.Status != "error" || !strings.Contains(v.Detail, "192.168.1.1") || !a.serving() {
		t.Fatalf("a second server: %+v", v)
	}

	// Pausing the takeover stops the server and keeps the choice.
	setPaused(t, a, true)
	if a.serving() || !a.settings.Get().GatewayAuto {
		t.Fatal("paused: still serving, or the choice forgotten")
	}

	// Turning the gateway off ends it, and the next time starts at manual.
	if _, err := a.settings.Update(func(s *settings.Settings) { wantGateway(s, false) }); err != nil {
		t.Fatal(err)
	}
	a.syncGateway()
	if s := a.settings.Get(); a.serving() || s.GatewayAuto || s.GatewayAutoLAN != "" {
		t.Fatalf("off: serving %v, settings %+v", a.serving(), s)
	}
}

// Before the server starts again (a launch, a resume), the LAN is asked
// once more: with the router's DHCP back it stays out, and goes on by
// itself when that is off.
func TestGatewayAutoAsksBeforeEveryStart(t *testing.T) {
	a, l, _ := autoApp(t)
	if err := a.setGatewayAuto(true); err != nil {
		t.Fatal(err)
	}
	setPaused(t, a, true)
	before := l.asked()
	setPaused(t, a, false)
	settle(t, a, true)
	if l.asked() != before+1 {
		t.Fatalf("asked %d times before serving again, want 1", l.asked()-before)
	}

	setPaused(t, a, true)
	l.set(func(l *lanFake) { l.others = []netip.Addr{netip.MustParseAddr("192.168.1.1")} })
	setPaused(t, a, false)
	settle(t, a, false)
	if v := a.autoView(a.settings.Get(), true); v.Status != "error" || !strings.Contains(v.Detail, "192.168.1.1") {
		t.Fatalf("with the router's DHCP back: %+v", v)
	}
	// The regular look tries again, and finds it gone.
	l.set(func(l *lanFake) { l.others = nil })
	a.tendAuto()
	settle(t, a, true)
}

// On another LAN nothing is served and nothing is even asked: a laptop
// carried to an office offers itself to nobody there. Back home it serves.
func TestGatewayAutoStaysOffOtherLANs(t *testing.T) {
	a, l, f := autoApp(t)
	if err := a.setGatewayAuto(true); err != nil {
		t.Fatal(err)
	}
	home := f.net
	office := func(l *lanFake) {
		l.self, l.lan = netip.MustParseAddr("10.0.0.37"), netip.MustParsePrefix("10.0.0.0/24")
		l.neigh = map[netip.Addr]string{netip.MustParseAddr("10.0.0.1"): "b8:00:00:00:00:01"}
	}
	l.set(office)
	f.net = gateway.Network{IP: "10.0.0.37", Iface: "en0", Router: "10.0.0.1"}
	a.auto.neighAt = time.Time{} // the ARP table is another one too
	before := l.asked()
	a.syncGateway()
	settle(t, a, false)
	a.tendAuto()
	settle(t, a, false)
	if l.asked() != before {
		t.Fatalf("asked the other LAN %d times", l.asked()-before)
	}
	if v := a.autoView(a.settings.Get(), true); v.Status != "error" || v.Detail == "" || !a.settings.Get().GatewayAuto {
		t.Fatalf("on the other LAN: %+v", v)
	}

	// A LAN with the same numbers and another router is another LAN.
	l.set(func(l *lanFake) {
		l.self, l.lan = netip.MustParseAddr("192.168.1.20"), netip.MustParsePrefix("192.168.1.0/24")
		l.neigh = map[netip.Addr]string{netip.MustParseAddr("192.168.1.1"): "c0:00:00:00:00:09"}
	})
	f.net = home
	a.auto.neighAt = time.Time{}
	a.syncGateway()
	settle(t, a, false)
	if l.asked() != before {
		t.Fatal("asked a LAN that only looks like home")
	}

	// Home again.
	l.set(func(l *lanFake) {
		l.neigh = map[netip.Addr]string{netip.MustParseAddr("192.168.1.1"): "a4:00:00:00:00:01"}
	})
	a.auto.neighAt = time.Time{}
	a.syncGateway()
	settle(t, a, true)
}

// Automatic is not for a computer whose own address is leased: with the
// router's DHCP off nobody renews it. It is refused before the LAN is even
// asked, and a server that runs stops when the address turns into a leased
// one, with the reason to show; with a fixed address again it goes on.
func TestGatewayAutoNeedsFixedAddress(t *testing.T) {
	a, l, f := autoApp(t)
	f.net.DHCP = true
	err := a.setGatewayAuto(true)
	if err == nil || !strings.Contains(err.Error(), "DHCP") || !strings.Contains(err.Error(), "固定地址") {
		t.Fatalf("with a leased address: %v", err)
	}
	if a.settings.Get().GatewayAuto || a.serving() || l.asked() != 0 {
		t.Fatalf("turned on, or the LAN was asked (%d times), with a leased address", l.asked())
	}

	f.net.DHCP = false
	if err := a.setGatewayAuto(true); err != nil {
		t.Fatal(err)
	}
	// The address becomes a leased one while it serves.
	f.net.DHCP = true
	before := l.asked()
	a.tendAuto()
	settle(t, a, false)
	v := a.autoView(a.settings.Get(), true)
	if v.Status != "error" || v.Detail != errLeased().Error() || !a.settings.Get().GatewayAuto {
		t.Fatalf("serving with a leased address: %+v", v)
	}
	// It does not start again either, and asks the LAN nothing meanwhile.
	a.tendAuto()
	a.syncGateway()
	settle(t, a, false)
	if l.asked() != before {
		t.Fatalf("asked the LAN %d times with a leased address", l.asked()-before)
	}
	// Fixed again: on by itself, after asking the LAN as before every start.
	f.net.DHCP = false
	a.tendAuto()
	settle(t, a, true)
	if v := a.autoView(a.settings.Get(), true); v.Status != "on" || v.Detail != "" {
		t.Fatalf("with a fixed address again: %+v", v)
	}
}

// A check that begins before bowel's own server runs and ends after it
// started hears that server too: it is not reported as another one. An
// answer from this computer while bowel does not serve is another
// program's server, and is reported.
func TestCheckLeavesOutOwnServer(t *testing.T) {
	a, l, _ := autoApp(t)
	self := netip.MustParseAddr("192.168.1.20")

	// Another program on this computer hands out addresses.
	l.set(func(l *lanFake) { l.others = []netip.Addr{self} })
	if others, err := a.checkDHCP(); err != nil || len(others) != 1 || others[0] != self {
		t.Fatalf("a server on this computer that is not bowel's: %v, %v", others, err)
	}
	if err := a.setGatewayAuto(true); err == nil || !strings.Contains(err.Error(), self.String()) {
		t.Fatalf("turned on beside it: %v", err)
	}
	l.set(func(l *lanFake) { l.others = nil })

	// bowel's own starts while a check listens, and answers it: the check
	// began after the one that cleared the LAN ended, before the server ran.
	if err := a.setGatewayAuto(true); err != nil {
		t.Fatal(err)
	}
	setPaused(t, a, true)
	if a.serving() {
		t.Fatal("serving while paused")
	}
	l.set(func(l *lanFake) {
		l.probing = func(netip.Addr) []netip.Addr {
			if _, err := a.settings.Update(func(s *settings.Settings) { s.Paused = false }); err != nil {
				t.Error(err)
			}
			a.auto.mu.Lock()
			a.auto.clear, a.auto.clearAt = "en0|192.168.1.20|192.168.1.0/24", time.Now()
			a.auto.clearLAN = a.settings.Get().GatewayAutoLAN
			a.auto.mu.Unlock()
			a.syncDHCP()
			return []netip.Addr{self}
		}
	})
	others, err := a.checkDHCP()
	if !a.serving() {
		t.Fatal("the server did not start during the check")
	}
	if err != nil || len(others) != 0 {
		t.Fatalf("bowel's own server reported as another: %v, %v", others, err)
	}
	if v := a.autoView(a.settings.Get(), true); v.Status != "on" || len(v.Servers) != 0 {
		t.Fatalf("view: %+v", v)
	}
	// While it serves, a check leaves this computer out from the start.
	l.set(func(l *lanFake) {
		l.probing = func(skip netip.Addr) []netip.Addr {
			if skip != self {
				t.Errorf("a check while serving leaves out %v", skip)
			}
			return []netip.Addr{self}
		}
	})
	if others, err := a.checkDHCP(); err != nil || len(others) != 0 {
		t.Fatalf("while serving: %v, %v", others, err)
	}
}

// Without a router's address the LAN can't be told from another, and the
// reason given is that one, not that this is another LAN.
func TestGatewayAutoNeedsRouter(t *testing.T) {
	a, l, f := autoApp(t)
	home := f.net
	f.net.Router = ""
	err := a.setGatewayAuto(true)
	if err == nil || err.Error() != errNoRouter().Error() || a.settings.Get().GatewayAuto || l.asked() != 0 {
		t.Fatalf("without a router: %v (asked %d times)", err, l.asked())
	}
	f.net = home
	if err := a.setGatewayAuto(true); err != nil {
		t.Fatal(err)
	}
	// The router is not known when it is to start again.
	setPaused(t, a, true)
	f.net.Router = ""
	before := l.asked()
	setPaused(t, a, false)
	settle(t, a, false)
	if v := a.autoView(a.settings.Get(), true); v.Status != "error" || v.Detail != errNoRouter().Error() || l.asked() != before {
		t.Fatalf("starting without a router: %+v (asked %d times)", v, l.asked()-before)
	}
	f.net = home
	a.tendAuto()
	settle(t, a, true)
}

// The server does not hold its lock while the ARP table is read: its leases
// are there at once however long the system takes, and what the server
// asks under its lock (who answers at an address) reads nothing.
func TestGatewayAutoSlowARP(t *testing.T) {
	a, l, _ := autoApp(t)
	if err := a.setGatewayAuto(true); err != nil {
		t.Fatal(err)
	}
	tv, other := netip.MustParseAddr("192.168.1.100"), "02:00:00:00:00:09"
	l.set(func(l *lanFake) { l.neigh[tv] = other })
	a.auto.neighMu.Lock()
	a.auto.neighAt = time.Time{}
	a.auto.neighMu.Unlock()

	// Under the server's lock: the table as last read, and no reading.
	reads := l.arpReads()
	if a.addressInUse(tv, "02:00:00:00:00:01") || l.arpReads() != reads {
		t.Fatalf("addressInUse read the table (%d times) or knew what was not read yet", l.arpReads()-reads)
	}

	// A device asks for an address while the table takes its time.
	l.set(func(l *lanFake) { l.arpWait = 2 * time.Second })
	l.mu.Lock()
	to := l.conn.LocalAddr()
	l.mu.Unlock()
	from, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer from.Close()
	m := &dhcp.Message{Op: 1, XID: 1, CHAddr: net.HardwareAddr{2, 0, 0, 0, 0, 1}, Opts: map[byte][]byte{53: {dhcp.Discover}}}
	if _, err := from.WriteTo(m.Marshal(), to); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(3 * time.Second); l.arpReads() == reads; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the server did not have the table read for a DISCOVER")
		}
	}
	start := time.Now()
	a.leases()
	a.autoView(a.settings.Get(), true)
	if d := time.Since(start); d > time.Second {
		t.Fatalf("the leases waited %v for the ARP table", d)
	}
	// Once read, the server knows who answers there.
	l.set(func(l *lanFake) { l.arpWait = 0 })
	a.neighbours()
	if !a.addressInUse(tv, "02:00:00:00:00:01") || a.addressInUse(tv, other) {
		t.Fatal("addressInUse does not follow the table that was read")
	}
}

// The snapshot and the server do not wait for each other: the server asks
// who answers at an address with its lock held, and the view asks the
// server for its leases.
func TestGatewayAutoViewAndServerDoNotDeadlock(t *testing.T) {
	a, l, _ := autoApp(t)
	if err := a.setGatewayAuto(true); err != nil {
		t.Fatal(err)
	}
	l.set(func(l *lanFake) { l.arpWait = 200 * time.Microsecond })
	l.mu.Lock()
	to := l.conn.LocalAddr()
	l.mu.Unlock()
	from, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer from.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 400 {
			a.auto.neighMu.Lock()
			a.auto.neighAt = time.Time{} // every packet reads the table again
			a.auto.neighMu.Unlock()
			m := &dhcp.Message{Op: 1, XID: uint32(i), CHAddr: net.HardwareAddr{2, 0, 0, 0, byte(i >> 8), byte(i)}, Opts: map[byte][]byte{53: {dhcp.Discover}}}
			_, _ = from.WriteTo(m.Marshal(), to)
			a.autoView(a.settings.Get(), true)
		}
	}()
	for i := 0; i < 400; i++ {
		a.autoView(a.settings.Get(), true)
		a.leases()
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the view and the server wait for each other")
	}
	// And the server still answers.
	if v := a.autoView(a.settings.Get(), true); v.Status != "on" {
		t.Fatalf("after it: %+v", v)
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
