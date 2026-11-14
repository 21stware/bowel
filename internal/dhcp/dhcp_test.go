package dhcp

import (
	"net"
	"net/netip"
	"path/filepath"
	"strings"
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
	// or one of another LAN.
	for _, a := range []string{"192.168.1.31", "192.168.1.1", "192.168.1.255", "10.0.0.5"} {
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
	// .100 and .101 have devices of their own; .50 is where device 7 sits.
	s.cfg.InUse = func(a netip.Addr, m string) bool {
		return a == ip("192.168.1.100") || a == ip("192.168.1.101") || (a == ip("192.168.1.50") && m != mac(7).String())
	}
	if o := s.handle(msg(Discover, mac(1), nil)); o.msg.YIAddr != ip("192.168.1.102") {
		t.Fatalf("offered %v", o.msg.YIAddr)
	}
	// Asking for a taken address by name gets another, and no lease on it.
	if o := s.handle(msg(Discover, mac(2), map[byte][]byte{optRequested: addrBytes(ip("192.168.1.50"))})); o.msg.YIAddr == ip("192.168.1.50") {
		t.Fatal("offered the address another device answers at")
	}
	if r := s.handle(msg(Request, mac(2), map[byte][]byte{optRequested: addrBytes(ip("192.168.1.50"))})); r.msg.Type() != Nak {
		t.Fatalf("leased the address another device answers at: %+v", r.msg)
	}
	// The device that sits there keeps it.
	if r := s.handle(msg(Request, mac(7), map[byte][]byte{optRequested: addrBytes(ip("192.168.1.50"))})); r.msg.Type() != Ack {
		t.Fatalf("the address's own device was refused: %+v", r.msg)
	}
}

// An address a device declines goes to nobody for a while, known holder or
// not; and offers nobody took do not pile up.
func TestDeclineAndOffers(t *testing.T) {
	s, now := testServer(t, "")
	o := s.handle(msg(Discover, mac(1), nil))
	bad := o.msg.YIAddr
	s.handle(msg(Request, mac(1), map[byte][]byte{optRequested: addrBytes(bad), optServer: addrBytes(self)}))
	s.handle(msg(Decline, mac(1), map[byte][]byte{optRequested: addrBytes(bad), optServer: addrBytes(self)}))
	if len(s.Leases()) != 0 {
		t.Fatalf("the declined lease is kept: %+v", s.Leases())
	}
	for n := byte(1); n <= 3; n++ {
		if o := s.handle(msg(Discover, mac(n), nil)); o == nil || o.msg.YIAddr == bad {
			t.Fatalf("device %d was offered the declined address", n)
		}
	}
	if r := s.handle(msg(Request, mac(4), map[byte][]byte{optRequested: addrBytes(bad)})); r.msg.Type() != Nak {
		t.Fatal("the declined address was leased")
	}
	if len(s.offers) != 3 {
		t.Fatalf("%d offers held", len(s.offers))
	}
	// Later the offers are gone and the address is back.
	*now = now.Add(declineHold + time.Second)
	if o := s.handle(msg(Discover, mac(9), nil)); o == nil || o.msg.YIAddr != bad {
		t.Fatalf("after the hold a new device got %+v", o)
	}
	if len(s.offers) != 1 || len(s.declined) != 0 {
		t.Fatalf("held: %d offers, %d declined", len(s.offers), len(s.declined))
	}
}

// A request for this computer's own address gets no answer at all: it is
// this computer's own DHCP client renewing with the router, and a refusal,
// which is broadcast, would take the address from under the server.
func TestOwnAddressIsLeftAlone(t *testing.T) {
	s, _ := testServer(t, "")
	own := msg(Request, mac(8), nil)
	own.CIAddr = self // renewing
	for _, m := range []*Message{
		own,
		msg(Request, mac(8), map[byte][]byte{optRequested: addrBytes(self)}),
		msg(Request, mac(8), map[byte][]byte{optRequested: addrBytes(self), optServer: addrBytes(self)}),
	} {
		if r := s.handle(m); r != nil {
			t.Fatalf("answered a request for this computer's address: %+v to %v", r.msg, r.to)
		}
	}
	if len(s.Leases()) != 0 {
		t.Fatalf("leased this computer's address: %+v", s.Leases())
	}
	// And it is offered to nobody who asks for it by name.
	if o := s.handle(msg(Discover, mac(8), map[byte][]byte{optRequested: addrBytes(self)})); o == nil || o.msg.YIAddr == self {
		t.Fatalf("offered this computer's address: %+v", o)
	}
}

// One device can't keep another from its address by declining it: a
// DECLINE counts only for the address the server gave to the device that
// sends it.
func TestDeclineOfAnothersAddress(t *testing.T) {
	s, now := testServer(t, "")
	req := func(n byte, a netip.Addr) *reply {
		return s.handle(msg(Request, mac(n), map[byte][]byte{optRequested: addrBytes(a)}))
	}
	a := s.handle(msg(Discover, mac(1), nil)).msg.YIAddr
	if r := req(1, a); r.msg.Type() != Ack {
		t.Fatal("no lease")
	}
	// B, which holds a lease of its own, declines A's address.
	b := s.handle(msg(Discover, mac(2), nil)).msg.YIAddr
	if r := req(2, b); r.msg.Type() != Ack || b == a {
		t.Fatalf("B's lease: %+v", r.msg)
	}
	s.handle(msg(Decline, mac(2), map[byte][]byte{optRequested: addrBytes(a), optServer: addrBytes(self)}))
	if len(s.declined) != 0 {
		t.Fatalf("an address declined by a device that never had it is kept away: %v", s.declined)
	}
	// A renews, by unicast with its address in ciaddr and by broadcast.
	*now = now.Add(DefaultLease / 2)
	renew := msg(Request, mac(1), nil)
	renew.CIAddr = a
	if r := s.handle(renew); r == nil || r.msg.Type() != Ack || r.msg.YIAddr != a {
		t.Fatalf("A's renewal after B's decline: %+v", r)
	}
	if r := req(1, a); r.msg.Type() != Ack {
		t.Fatalf("A's request after B's decline: %+v", r.msg)
	}
	// B gave up its own lease by declining, but its address is not kept
	// away either, and B is not forgotten: it gets the address back.
	if ls := s.Leases(); len(ls) != 2 || ls[1].IP != b || ls[1].Active(s.now()) {
		t.Fatalf("leases after B's decline: %+v", ls)
	}
	if o := s.handle(msg(Discover, mac(2), nil)); o == nil || o.msg.YIAddr != b {
		t.Fatalf("B was offered %+v", o)
	}

	// Even if an address is kept away, the device that holds it by a lease
	// keeps renewing it.
	s.declined[a] = now.Add(declineHold)
	if r := s.handle(renew); r == nil || r.msg.Type() != Ack {
		t.Fatalf("the holder of a declined address was refused: %+v", r)
	}
	if r := req(3, a); r.msg.Type() != Nak {
		t.Fatal("another device got it")
	}
}

// An address a device was offered and found taken, before it ever held
// it, is kept away from everyone for the hold, the device itself included.
func TestDeclineOfOfferedAddress(t *testing.T) {
	s, now := testServer(t, "")
	bad := s.handle(msg(Discover, mac(2), nil)).msg.YIAddr
	s.handle(msg(Decline, mac(2), map[byte][]byte{optRequested: addrBytes(bad), optServer: addrBytes(self)}))
	if _, ok := s.declined[bad]; !ok {
		t.Fatal("the declined offer is not kept away")
	}
	*now = now.Add(declineHold - time.Minute)
	for n := byte(1); n <= 3; n++ {
		if o := s.handle(msg(Discover, mac(n), map[byte][]byte{optRequested: addrBytes(bad)})); o == nil || o.msg.YIAddr == bad {
			t.Fatalf("device %d was offered the declined address: %+v", n, o)
		}
		if r := s.handle(msg(Request, mac(n), map[byte][]byte{optRequested: addrBytes(bad)})); r.msg.Type() != Nak {
			t.Fatalf("device %d got the declined address", n)
		}
	}
	*now = now.Add(2 * time.Minute)
	if r := s.handle(msg(Request, mac(4), map[byte][]byte{optRequested: addrBytes(bad)})); r.msg.Type() != Ack {
		t.Fatal("the address is still away after the hold")
	}
}

// Leases that ended long ago are forgotten while the server runs, not only
// when it starts, and the file follows.
func TestOldLeasesAreForgotten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "leases.json")
	s, now := testServer(t, path)
	req := func(n byte, a string) {
		t.Helper()
		if r := s.handle(msg(Request, mac(n), map[byte][]byte{optRequested: addrBytes(ip(a))})); r.msg.Type() != Ack {
			t.Fatalf("%s: %+v", a, r.msg)
		}
	}
	req(1, "192.168.1.100")
	*now = now.Add(keepFor - time.Hour)
	req(2, "192.168.1.101")
	if len(s.Leases()) != 2 {
		t.Fatalf("forgotten too soon: %+v", s.Leases())
	}
	// A message that changes nothing else is enough.
	*now = now.Add(DefaultLease + 2*time.Hour)
	s.handle(msg(Inform, mac(9), nil))
	if ls := s.Leases(); len(ls) != 1 || ls[0].IP != ip("192.168.1.101") {
		t.Fatalf("kept: %+v", ls)
	}
	s2, err := New(Config{Iface: "x", Self: self, Prefix: lan, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if ls := s2.Leases(); len(ls) != 1 || ls[0].IP != ip("192.168.1.101") {
		t.Fatalf("in the file: %+v", ls)
	}
}

// A device's name is shown in terminals: nothing of it that does not print
// gets through, and no more than a label's length.
func TestHostnameIsCleaned(t *testing.T) {
	for in, want := range map[string]string{
		"tv":                            "tv",
		"客厅的电视":                         "客厅的电视",
		"\x1b]0;owned\x07\x1b[2Jtv\r\n": "]0;owned[2Jtv",
		"a\x00b\tc\x7f\u009bd":          "abcd",
		"x\u202egnp.exe":                "xgnp.exe",
		"bad\xff\xfeutf":                "badutf",
		"  spaced  ":                    "spaced",
		strings.Repeat("a", 300):        strings.Repeat("a", 63),
		strings.Repeat("电", 40):         strings.Repeat("电", 21),
	} {
		m := msg(Request, mac(1), map[byte][]byte{optHostname: []byte(in)})
		if got := m.Hostname(); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	// And so it is in the lease.
	s, _ := testServer(t, "")
	s.handle(msg(Request, mac(1), map[byte][]byte{optRequested: addrBytes(ip("192.168.1.100")), optHostname: []byte("\x1b[31mtv\x1b[0m")}))
	if ls := s.Leases(); len(ls) != 1 || ls[0].Name != "[31mtv[0m" {
		t.Fatalf("%+q", ls)
	}
}

// Refresh runs before a message that may give an address, and not under the
// server's lock: Leases does not wait for it.
func TestRefreshIsOutsideTheLock(t *testing.T) {
	s, _ := testServer(t, "")
	calls, in, out := 0, make(chan struct{}), make(chan struct{})
	s.cfg.Refresh = func() {
		calls++
		if calls == 1 {
			close(in)
			<-out
		}
	}
	done := make(chan *reply)
	go func() { done <- s.handle(msg(Discover, mac(1), nil)) }()
	<-in
	got := make(chan int)
	go func() { got <- len(s.Leases()) }()
	select {
	case <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("Leases waits for Refresh")
	}
	close(out)
	if r := <-done; r == nil || r.msg.Type() != Offer {
		t.Fatalf("%+v", r)
	}
	s.handle(msg(Release, mac(1), nil))
	s.handle(msg(Inform, mac(1), nil))
	if calls != 1 {
		t.Fatalf("Refresh ran %d times, want 1: only DISCOVER and REQUEST need it", calls)
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
