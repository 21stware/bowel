package dhcp

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"net"
	"net/netip"
	"slices"
	"time"
)

// Probe asks the LAN on iface who hands out addresses, as a device that
// just joined would, and returns the servers that offered one within wait.
// It takes no address: an offer that is not requested lapses by itself.
// self, the address of bowel's own server if it runs, is left out.
func Probe(ctx context.Context, iface string, self netip.Addr, wait time.Duration) ([]netip.Addr, error) {
	ifc, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, err
	}
	c, err := listen(ifc, clientPort)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	// A made-up device, so that no real one's lease is touched: a locally
	// administered address, new every time.
	mac := make(net.HardwareAddr, 6)
	_, _ = rand.Read(mac)
	mac[0] = mac[0]&^1 | 2
	var x [4]byte
	_, _ = rand.Read(x[:])
	q := &Message{Op: bootRequest, XID: binary.BigEndian.Uint32(x[:]), Flags: flagBroadcast, CHAddr: mac,
		Opts: map[byte][]byte{optType: {Discover}}}
	to := &net.UDPAddr{IP: net.IPv4bcast, Port: serverPort}

	var found []netip.Addr
	deadline := time.Now().Add(wait)
	buf := make([]byte, 1500)
	// Asked twice: a broadcast is easily lost on Wi-Fi.
	for _, at := range []time.Time{time.Now(), time.Now().Add(wait / 2)} {
		if d := time.Until(at); d > 0 {
			readOffers(ctx, c, buf, q, self, at, &found)
		}
		if ctx.Err() != nil {
			return found, ctx.Err()
		}
		if _, err := c.WriteTo(q.Marshal(), to); err != nil {
			return found, err
		}
	}
	readOffers(ctx, c, buf, q, self, deadline, &found)
	return found, ctx.Err()
}

// readOffers collects the servers that answer q until the time given.
func readOffers(ctx context.Context, c net.PacketConn, buf []byte, q *Message, self netip.Addr, until time.Time, found *[]netip.Addr) {
	for ctx.Err() == nil {
		// In short waits, so that a cancelled context ends it soon.
		step := time.Now().Add(200 * time.Millisecond)
		if step.After(until) {
			step = until
		}
		_ = c.SetReadDeadline(step)
		n, from, err := c.ReadFrom(buf)
		if err != nil {
			if !time.Now().Before(until) {
				return
			}
			continue
		}
		m, err := Parse(buf[:n])
		if err != nil || m.Op != bootReply || m.XID != q.XID || m.Type() != Offer {
			continue
		}
		srv := m.Server()
		if !srv.IsValid() { // a server that does not name itself: where it came from
			if u, ok := from.(*net.UDPAddr); ok {
				srv, _ = netip.AddrFromSlice(u.IP.To4())
			}
		}
		if srv.IsValid() && srv != self && !slices.Contains(*found, srv) {
			*found = append(*found, srv)
		}
	}
}
