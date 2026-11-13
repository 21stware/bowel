package dhcp

import (
	"net"
	"net/netip"
	"path/filepath"
	"testing"
	"time"
)

var (
	self   = netip.MustParseAddr("192.168.1.20")
	router = netip.MustParseAddr("192.168.1.1")
	lan    = netip.MustParsePrefix("192.168.1.0/24")
)

func testServer(t *testing.T, path string) (*Server, *time.Time) {
	t.Helper()
	s, err := New(Config{Iface: "x", Self: self, Prefix: lan, Avoid: []netip.Addr{router}, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	return s, &now
}

func mac(n byte) net.HardwareAddr { return net.HardwareAddr{2, 0, 0, 0, 0, n} }

func msg(t byte, m net.HardwareAddr, opts map[byte][]byte) *Message {
	if opts == nil {
		opts = map[byte][]byte{}
	}
	opts[optType] = []byte{t}
	return &Message{Op: bootRequest, XID: 7, CHAddr: m, Opts: opts}
}

func ip(s string) netip.Addr { return netip.MustParseAddr(s) }

// What is written is what is read.
func TestRoundTrip(t *testing.T) {
	in := &Message{Op: bootRequest, XID: 0xdeadbeef, Flags: flagBroadcast, CIAddr: ip("192.168.1.31"), CHAddr: mac(9),
		Opts: map[byte][]byte{optType: {Request}, optRequested: addrBytes(ip("192.168.1.31")), optHostname: []byte("tv"), optServer: addrBytes(self)}}
	b := in.Marshal()
	if len(b) < minLen {
		t.Fatalf("%d bytes: shorter than BOOTP's minimum", len(b))
	}
	out, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if out.XID != in.XID || !out.Broadcast() || out.CIAddr != in.CIAddr || out.CHAddr.String() != in.CHAddr.String() ||
		out.Type() != Request || out.Hostname() != "tv" || out.Server() != self || out.addrOpt(optRequested) != ip("192.168.1.31") {
		t.Fatalf("%+v", out)
	}
	if _, err := Parse(b[:100]); err == nil {
		t.Fatal("a cut packet parsed")
	}
	b[headerLen] = 0
	if _, err := Parse(b); err == nil {
		t.Fatal("a packet without the cookie parsed")
	}
}

// A new device is offered an address of the pool, with this computer as its
// router and DNS, and has it once it asks.
func TestDiscoverRequest(t *testing.T) {
	s, _ := testServer(t, "")
	r := s.handle(msg(Discover, mac(1), nil))
	if r == nil || r.msg.Type() != Offer {
		t.Fatalf("no offer: %+v", r)
	}
	got := r.msg.YIAddr
	first, last := s.Pool()
	if first != ip("192.168.1.100") || last != ip("192.168.1.249") || got != first {
		t.Fatalf("offered %v, pool %v–%v", got, first, last)
	}
	if r.msg.addrOpt(optRouter) != self || r.msg.addrOpt(optDNS) != self || r.msg.Server() != self ||
		net.IP(r.msg.Opts[optMask]).String() != "255.255.255.0" || !r.to.IP.Equal(net.IPv4bcast) || r.to.Port != clientPort {
		t.Fatalf("offer: %+v to %v", r.msg, r.to)
	}
	// A second device meanwhile gets another address.
	if r2 := s.handle(msg(Discover, mac(2), nil)); r2 == nil || r2.msg.YIAddr == got {
		t.Fatalf("the same address offered twice: %+v", r2)
	}
	a := s.handle(msg(Request, mac(1), map[byte][]byte{optRequested: addrBytes(got), optServer: addrBytes(self), optHostname: []byte("tv")}))
	if a == nil || a.msg.Type() != Ack || a.msg.YIAddr != got {
		t.Fatalf("no ack: %+v", a)
	}
	ls := s.Leases()
	if len(ls) != 1 || ls[0].IP != got || ls[0].Name != "tv" || !ls[0].Active(s.now()) {
		t.Fatalf("leases: %+v", ls)
	}
}

// A device that took the router's offer is left alone.
func TestRequestForAnotherServer(t *testing.T) {
	s, _ := testServer(t, "")
	s.handle(msg(Discover, mac(1), nil))
	if r := s.handle(msg(Request, mac(1), map[byte][]byte{optRequested: addrBytes(ip("192.168.1.50")), optServer: addrBytes(router)})); r != nil {
		t.Fatalf("answered a request to the router: %+v", r.msg)
	}
	if len(s.Leases()) != 0 || len(s.offers) != 0 {
		t.Fatal("something kept of it")
	}
}

// Taking over: a device that has an address from the router keeps it, when
// it rebinds (by broadcast, its address in ciaddr) and when it starts with
// the address it remembers. The answer to the first goes to the device.
func TestTakeoverKeepsAddresses(t *testing.T) {
	s, _ := testServer(t, "")
	m := msg(Request, mac(1), nil)
	m.CIAddr = ip("192.168.1.31")
	r := s.handle(m)
	if r == nil || r.msg.Type() != Ack || r.msg.YIAddr != ip("192.168.1.31") || r.msg.addrOpt(optRouter) != self {
		t.Fatalf("rebinding: %+v", r)
	}
	if !r.to.IP.Equal(net.IP{192, 168, 1, 31}) {
		t.Fatalf("answered to %v", r.to)
	}
	r = s.handle(msg(Request, mac(2), map[byte][]byte{optRequested: addrBytes(ip("192.168.1.32"))}))
	if r == nil || r.msg.Type() != Ack || r.msg.YIAddr != ip("192.168.1.32") {
		t.Fatalf("starting with a remembered address: %+v", r)
	}
	// Another device can't have an address that is taken, nor the router's,
	// this computer's, or one of another LAN.
	for _, a := range []string{"192.168.1.31", "192.168.1.1", "192.168.1.20", "192.168.1.255", "10.0.0.5"} {
		r = s.handle(msg(Request, mac(3), map[byte][]byte{optRequested: addrBytes(ip(a))}))
		if r == nil || r.msg.Type() != Nak || !r.to.IP.Equal(net.IPv4bcast) {
			t.Fatalf("%s: %+v", a, r)
		}
	}
	// And is not offered them either.
	if o := s.handle(msg(Discover, mac(3), map[byte][]byte{optRequested: addrBytes(ip("192.168.1.31"))})); o == nil || o.msg.YIAddr == ip("192.168.1.31") {
		t.Fatalf("offered a taken address: %+v", o)
	}
}

// A lease ends, the device comes back later and gets its address again,
// also after a restart; a released address is free for others at once.
func TestLeasesLastAndReturn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "leases.json")
	s, now := testServer(t, path)
	req := func(s *Server, n byte, a string) *reply {
		return s.handle(msg(Request, mac(n), map[byte][]byte{optRequested: addrBytes(ip(a))}))
	}
	if r := req(s, 1, "192.168.1.100"); r.msg.Type() != Ack {
		t.Fatal("no lease")
	}
	*now = now.Add(DefaultLease + time.Second)
	if s.Leases()[0].Active(s.now()) {
		t.Fatal("the lease did not end")
	}
	// Its address is the last to go to a new device.
	if o := s.handle(msg(Discover, mac(2), nil)); o.msg.YIAddr != ip("192.168.1.101") {
		t.Fatalf("a new device got %v", o.msg.YIAddr)
	}
	s2, err := New(Config{Iface: "x", Self: self, Prefix: lan, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	s2.now = s.now
	if o := s2.handle(msg(Discover, mac(1), nil)); o == nil || o.msg.YIAddr != ip("192.168.1.100") {
		t.Fatalf("after a restart the device was offered %+v", o)
	}
	if r := req(s2, 1, "192.168.1.100"); r.msg.Type() != Ack {
		t.Fatal("not renewed")
	}
	if r := req(s2, 5, "192.168.1.100"); r.msg.Type() != Nak {
		t.Fatal("a held address given away")
	}
	s2.handle(msg(Release, mac(1), nil))
	if r := req(s2, 5, "192.168.1.100"); r.msg.Type() != Ack {
		t.Fatal("a released address still held")
	}
}

// An address some device answers at without a lease is passed over.
func TestInUseIsSkipped(t *testing.T) {
	s, _ := testServer(t, "")
	s.cfg.InUse = func(a netip.Addr) bool { return a == ip("192.168.1.100") || a == ip("192.168.1.101") }
	if o := s.handle(msg(Discover, mac(1), nil)); o.msg.YIAddr != ip("192.168.1.102") {
		t.Fatalf("offered %v", o.msg.YIAddr)
	}
}

func TestNewRefuses(t *testing.T) {
	if _, err := New(Config{Self: ip("10.0.0.2"), Prefix: lan}); err == nil {
		t.Fatal("served a LAN this computer is not on")
	}
	if _, err := New(Config{Self: ip("10.0.0.2"), Prefix: netip.MustParsePrefix("10.0.0.0/8")}); err == nil {
		t.Fatal("served a /8")
	}
}
