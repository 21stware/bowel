package dhcp

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"os"
	"slices"
	"sync"
	"time"
)

// Lease is an address a device holds, or held: an expired lease is kept,
// so that the device gets the same address when it comes back.
type Lease struct {
	MAC     string     `json:"mac"`
	IP      netip.Addr `json:"ip"`
	Name    string     `json:"name,omitempty"`
	Expires time.Time  `json:"expires"`
	First   time.Time  `json:"first"`
	Seen    time.Time  `json:"seen"`
}

// Active says whether the lease still holds at t.
func (l Lease) Active(t time.Time) bool { return t.Before(l.Expires) }

// Config is what a server hands out and where.
type Config struct {
	Iface string
	// Self is this computer on the LAN: the router and DNS of every lease.
	Self   netip.Addr
	Prefix netip.Prefix // the LAN
	// Avoid are addresses never handed out: the real router, at least.
	Avoid []netip.Addr
	// LeaseTime is how long a lease holds; DefaultLease when zero.
	LeaseTime time.Duration
	// Path is the file the leases are kept in; none when empty.
	Path string
	// InUse, if set, says whether some device already answers at an
	// address nobody leased: one with a fixed address, which must not be
	// given to another.
	InUse func(netip.Addr) bool
	// OnChange is called after a lease was given, renewed or given back.
	OnChange func()
}

// DefaultLease is short on purpose: when the router is given the job
// back, devices return to it within half of this.
const DefaultLease = 20 * time.Minute

const (
	offerHold = time.Minute         // an offer nobody requested
	keepFor   = 30 * 24 * time.Hour // an expired lease's address stays its device's
)

// Server hands out addresses on one LAN.
type Server struct {
	cfg Config
	now func() time.Time

	mu     sync.Mutex
	leases map[string]*Lease  // by MAC
	offers map[string]offered // by MAC
}

type offered struct {
	ip    netip.Addr
	until time.Time
}

// New returns a server for cfg, with the leases of an earlier run.
func New(cfg Config) (*Server, error) {
	cfg.Prefix = cfg.Prefix.Masked()
	if !cfg.Self.Is4() || !cfg.Prefix.Addr().Is4() || !cfg.Prefix.Contains(cfg.Self) {
		return nil, errors.New("dhcp: this computer's address is not on the LAN to serve")
	}
	if cfg.Prefix.Bits() < 16 || cfg.Prefix.Bits() > 30 {
		return nil, errors.New("dhcp: a LAN of this size is not served")
	}
	if cfg.LeaseTime <= 0 {
		cfg.LeaseTime = DefaultLease
	}
	s := &Server{cfg: cfg, now: time.Now, leases: map[string]*Lease{}, offers: map[string]offered{}}
	s.load()
	return s, nil
}

func (s *Server) load() {
	if s.cfg.Path == "" {
		return
	}
	b, err := os.ReadFile(s.cfg.Path)
	if err != nil {
		return
	}
	var list []Lease
	if json.Unmarshal(b, &list) != nil {
		return
	}
	now := s.now()
	for _, l := range list {
		// A lease from another LAN, or long gone, is of no use here.
		if s.cfg.Prefix.Contains(l.IP) && now.Sub(l.Expires) < keepFor {
			l := l
			s.leases[l.MAC] = &l
		}
	}
}

// save writes the leases; under s.mu.
func (s *Server) save() {
	if s.cfg.Path == "" {
		return
	}
	b, err := json.MarshalIndent(s.sorted(), "", "  ")
	if err != nil {
		return
	}
	tmp := s.cfg.Path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, s.cfg.Path)
	}
}

func (s *Server) sorted() []Lease {
	out := make([]Lease, 0, len(s.leases))
	for _, l := range s.leases {
		out = append(out, *l)
	}
	slices.SortFunc(out, func(a, b Lease) int { return a.IP.Compare(b.IP) })
	return out
}

// Leases are the leases known, by address, expired ones included.
func (s *Server) Leases() []Lease {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sorted()
}

// Pool is the range new devices get their addresses from: the upper part
// of the LAN, where routers rarely put fixed addresses (100–249 of a /24).
func (s *Server) Pool() (first, last netip.Addr) {
	base := s.cfg.Prefix.Addr().As4()
	n := uint32(1) << (32 - s.cfg.Prefix.Bits())
	b := uint32(base[0])<<24 | uint32(base[1])<<16 | uint32(base[2])<<8 | uint32(base[3])
	at := func(off uint32) netip.Addr {
		v := b + off
		return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
	}
	lo, hi := n*25/64, n-1-max(1, n*6/256)
	if lo < 1 {
		lo = 1
	}
	if hi < lo {
		hi = lo
	}
	return at(lo), at(hi)
}

// usable says whether ip may be the address of mac: on the LAN, not the
// network's own addresses, not kept away, and nobody else's.
func (s *Server) usable(ip netip.Addr, mac string) bool {
	p := s.cfg.Prefix
	if !ip.Is4() || !p.Contains(ip) || ip == p.Addr() || ip == s.cfg.Self || slices.Contains(s.cfg.Avoid, ip) {
		return false
	}
	if p.Bits() < 31 && ip == broadcastOf(p) {
		return false
	}
	now := s.now()
	for m, l := range s.leases {
		if m != mac && l.IP == ip && l.Active(now) {
			return false
		}
	}
	for m, o := range s.offers {
		if m != mac && o.ip == ip && now.Before(o.until) {
			return false
		}
	}
	return true
}

func broadcastOf(p netip.Prefix) netip.Addr {
	a := p.Addr().As4()
	for i := p.Bits(); i < 32; i++ {
		a[i/8] |= 1 << (7 - i%8)
	}
	return netip.AddrFrom4(a)
}

// pick chooses the address to offer mac: the one it had, the one it asks
// for, or the first of the pool that is free.
func (s *Server) pick(mac string, want netip.Addr) netip.Addr {
	if l := s.leases[mac]; l != nil && s.usable(l.IP, mac) {
		return l.IP
	}
	if want.IsValid() && s.usable(want, mac) {
		return want
	}
	held := map[netip.Addr]bool{} // an expired lease keeps its address while others are free
	for m, l := range s.leases {
		if m != mac {
			held[l.IP] = true
		}
	}
	first, last := s.Pool()
	for pass := 0; pass < 2; pass++ {
		for ip := first; ip.Compare(last) <= 0; ip = ip.Next() {
			if (pass == 0 && held[ip]) || !s.usable(ip, mac) {
				continue
			}
			if s.cfg.InUse != nil && s.cfg.InUse(ip) {
				continue
			}
			return ip
		}
	}
	return netip.Addr{}
}

// reply is an answer and where it goes.
type reply struct {
	msg *Message
	to  *net.UDPAddr
}

// handle answers one message of a client, or returns nil for none.
func (s *Server) handle(m *Message) *reply {
	if m.Op != bootRequest || len(m.CHAddr) != 6 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	mac, now := m.CHAddr.String(), s.now()
	changed := false
	defer func() {
		if changed {
			s.save()
			if s.cfg.OnChange != nil {
				go s.cfg.OnChange()
			}
		}
	}()

	switch m.Type() {
	case Discover:
		ip := s.pick(mac, m.addrOpt(optRequested))
		if !ip.IsValid() {
			return nil // the LAN is full
		}
		s.offers[mac] = offered{ip, now.Add(offerHold)}
		return s.answer(m, Offer, ip)

	case Request:
		if srv := m.Server(); srv.IsValid() && srv != s.cfg.Self {
			delete(s.offers, mac) // it took another server's offer
			return nil
		}
		ip := m.addrOpt(optRequested)
		if !ip.IsValid() {
			ip = m.CIAddr // renewing or rebinding
		}
		if !ip.IsValid() {
			return nil
		}
		// An address it holds from the router that handed them out before
		// is confirmed too, as long as nobody else has it: that is what
		// lets devices keep their addresses when this server takes over.
		if !s.usable(ip, mac) {
			return s.answer(m, Nak, netip.Addr{})
		}
		delete(s.offers, mac)
		l := s.leases[mac]
		if l == nil {
			l = &Lease{MAC: mac, First: now}
			s.leases[mac] = l
		}
		l.IP, l.Expires, l.Seen = ip, now.Add(s.cfg.LeaseTime), now
		if h := m.Hostname(); h != "" {
			l.Name = h
		}
		changed = true
		return s.answer(m, Ack, ip)

	case Release, Decline:
		delete(s.offers, mac)
		if l := s.leases[mac]; l != nil && l.Active(now) {
			l.Expires = now
			changed = true
		}
		if m.Type() == Decline {
			delete(s.leases, mac) // the address was someone's: don't offer it to this device again
			changed = true
		}
		return nil

	case Inform:
		if !m.CIAddr.IsValid() {
			return nil
		}
		return s.answer(m, Ack, netip.Addr{})
	}
	return nil
}

// answer builds the reply of type t to m, giving ip when it is valid.
func (s *Server) answer(m *Message, t byte, ip netip.Addr) *reply {
	out := &Message{Op: bootReply, XID: m.XID, Flags: m.Flags, GIAddr: m.GIAddr, CHAddr: m.CHAddr,
		Opts: map[byte][]byte{optType: {t}, optServer: addrBytes(s.cfg.Self)}}
	if t != Nak {
		out.YIAddr = ip
		if m.Type() == Inform {
			out.CIAddr = m.CIAddr
		} else {
			lease := uint32(s.cfg.LeaseTime / time.Second)
			out.Opts[optLease], out.Opts[optRenew], out.Opts[optRebind] = secs(lease), secs(lease/2), secs(lease*7/8)
		}
		mask := net.CIDRMask(s.cfg.Prefix.Bits(), 32)
		out.Opts[optMask], out.Opts[optRouter], out.Opts[optDNS] = mask, addrBytes(s.cfg.Self), addrBytes(s.cfg.Self)
	}
	to := &net.UDPAddr{IP: net.IPv4bcast, Port: clientPort}
	switch {
	case m.GIAddr.IsValid():
		to = &net.UDPAddr{IP: m.GIAddr.AsSlice(), Port: serverPort}
	case t == Nak:
	case m.CIAddr.IsValid() && !m.Broadcast():
		to = &net.UDPAddr{IP: m.CIAddr.AsSlice(), Port: clientPort}
	}
	return &reply{out, to}
}

// Listen opens the server's socket. It fails when another program has the
// port in a way that leaves none, or the system does not allow it.
func (s *Server) Listen() (net.PacketConn, error) {
	ifc, err := net.InterfaceByName(s.cfg.Iface)
	if err != nil {
		return nil, err
	}
	return listen(ifc, serverPort)
}

// Serve answers clients on c until ctx ends, and closes c.
func (s *Server) Serve(ctx context.Context, c net.PacketConn) error {
	stop := context.AfterFunc(ctx, func() { c.Close() })
	defer stop()
	defer c.Close()
	buf := make([]byte, 1500)
	for {
		n, _, err := c.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		m, err := Parse(buf[:n])
		if err != nil {
			continue
		}
		if r := s.handle(m); r != nil {
			_, _ = c.WriteTo(r.msg.Marshal(), r.to)
		}
	}
}
