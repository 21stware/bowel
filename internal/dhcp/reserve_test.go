package dhcp

import (
	"net/netip"
	"testing"
)

func request(s *Server, n byte, want netip.Addr) *reply {
	return s.handle(msg(Request, mac(n), map[byte][]byte{optRequested: addrBytes(want), optServer: addrBytes(self)}))
}

// A device with an address kept for it is offered that one, and no other
// device is given it, even when it asks for it.
func TestReservation(t *testing.T) {
	s, _ := testServer(t, "")
	kept := ip("192.168.1.50")
	s.Reserve([]Reservation{{MAC: mac(1).String(), IP: kept}})

	if r := s.handle(msg(Discover, mac(1), nil)); r == nil || r.msg.YIAddr != kept {
		t.Fatalf("offered %+v, want the kept address", r)
	}
	if r := request(s, 1, kept); r == nil || r.msg.Type() != Ack || r.msg.YIAddr != kept {
		t.Fatalf("not confirmed: %+v", r)
	}
	// Another device asks for it: refused, and not offered it either.
	if r := request(s, 2, kept); r == nil || r.msg.Type() != Nak {
		t.Fatalf("another device was confirmed the kept address: %+v", r)
	}
	if r := s.handle(msg(Discover, mac(2), map[byte][]byte{optRequested: addrBytes(kept)})); r == nil || r.msg.YIAddr == kept {
		t.Fatalf("another device was offered the kept address: %+v", r)
	}
}

// A device that holds another address than the one kept for it is refused
// it, so that it asks again and gets its own; one whose kept address is
// someone else's by now keeps the one it has.
func TestReservationMoves(t *testing.T) {
	s, _ := testServer(t, "")
	if r := request(s, 1, ip("192.168.1.31")); r == nil || r.msg.Type() != Ack {
		t.Fatalf("not confirmed before the reservation: %+v", r)
	}
	s.Reserve([]Reservation{{MAC: mac(1).String(), IP: ip("192.168.1.50")}})
	if r := request(s, 1, ip("192.168.1.31")); r == nil || r.msg.Type() != Nak {
		t.Fatalf("confirmed its old address: %+v", r)
	}
	if r := s.handle(msg(Discover, mac(1), nil)); r == nil || r.msg.YIAddr != ip("192.168.1.50") {
		t.Fatalf("not offered its own: %+v", r)
	}

	// A device with a fixed address of its own answers at the kept one.
	s.cfg.InUse = func(a netip.Addr, m string) bool { return a == ip("192.168.1.50") && m != "x" }
	if r := request(s, 1, ip("192.168.1.31")); r == nil || r.msg.Type() != Ack {
		t.Fatalf("refused while its kept address is taken: %+v", r)
	}
}

// A reservation that knows no hardware address keeps its address out of
// what new devices are offered, and still lets the device that has it
// renew it.
func TestReservationWithoutMAC(t *testing.T) {
	s, _ := testServer(t, "")
	first, _ := s.Pool()
	s.Reserve([]Reservation{{IP: first}})
	if r := s.handle(msg(Discover, mac(3), nil)); r == nil || r.msg.YIAddr == first {
		t.Fatalf("offered the kept address: %+v", r)
	}
	if r := request(s, 4, first); r == nil || r.msg.Type() != Ack {
		t.Fatalf("its holder couldn't renew it: %+v", r)
	}
}

// A device left out of the gateway gets the LAN's router as its router and
// DNS; the others keep this computer.
func TestReservationBypass(t *testing.T) {
	s, _ := testServer(t, "")
	s.cfg.Router = router
	s.Reserve([]Reservation{{MAC: mac(1).String(), IP: ip("192.168.1.50"), Bypass: true}})
	r := s.handle(msg(Discover, mac(1), nil))
	if r == nil || r.msg.addrOpt(optRouter) != router || r.msg.addrOpt(optDNS) != router || r.msg.Server() != self {
		t.Fatalf("left out: %+v", r)
	}
	if a := request(s, 1, ip("192.168.1.50")); a == nil || a.msg.Type() != Ack || a.msg.addrOpt(optRouter) != router {
		t.Fatalf("ack: %+v", a)
	}
	if o := s.handle(msg(Discover, mac(2), nil)); o == nil || o.msg.addrOpt(optRouter) != self || o.msg.addrOpt(optDNS) != self {
		t.Fatalf("another device: %+v", o)
	}
	// Back in: this computer again.
	s.Reserve(nil)
	if a := request(s, 1, ip("192.168.1.50")); a == nil || a.msg.Type() != Ack || a.msg.addrOpt(optRouter) != self {
		t.Fatalf("back in: %+v", a)
	}
}
