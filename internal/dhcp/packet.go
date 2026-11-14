// Package dhcp is the little of DHCP the gateway needs: finding out who
// hands out addresses on the LAN, and handing them out itself, with this
// computer as every device's router and DNS.
//
// It is IPv4 only, one subnet, no relay agents of its own, and it always
// answers a client that has no address yet by broadcast, which every
// client must accept (RFC 2131, 4.1).
package dhcp

import (
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Message types (option 53).
const (
	Discover byte = 1
	Offer    byte = 2
	Request  byte = 3
	Decline  byte = 4
	Ack      byte = 5
	Nak      byte = 6
	Release  byte = 7
	Inform   byte = 8
)

// The options used here.
const (
	optMask      byte = 1
	optRouter    byte = 3
	optDNS       byte = 6
	optHostname  byte = 12
	optRequested byte = 50
	optLease     byte = 51
	optType      byte = 53
	optServer    byte = 54
	optRenew     byte = 58
	optRebind    byte = 59
	optEnd       byte = 255
	optPad       byte = 0
)

const (
	serverPort    = 67
	clientPort    = 68
	headerLen     = 236
	minLen        = 300 // BOOTP's minimum, which some clients still insist on
	bootRequest   = 1
	bootReply     = 2
	flagBroadcast = 0x8000
)

var magic = [4]byte{99, 130, 83, 99}

// Message is a DHCP packet.
type Message struct {
	Op     byte
	XID    uint32
	Flags  uint16
	CIAddr netip.Addr // the client's address, when it has one
	YIAddr netip.Addr // the address it is given
	GIAddr netip.Addr // a relay agent's
	CHAddr net.HardwareAddr
	Opts   map[byte][]byte
}

// Type is the message type, 0 when there is none.
func (m *Message) Type() byte {
	if v := m.Opts[optType]; len(v) == 1 {
		return v[0]
	}
	return 0
}

// addrOpt is an option that holds one address.
func (m *Message) addrOpt(code byte) netip.Addr {
	if v := m.Opts[code]; len(v) >= 4 {
		return netip.AddrFrom4([4]byte(v[:4]))
	}
	return netip.Addr{}
}

// Server is the server a message names (option 54).
func (m *Message) Server() netip.Addr { return m.addrOpt(optServer) }

// maxHostname is the longest name kept of a client: a DNS label's length.
const maxHostname = 63

// Hostname is the name the client gives itself, if any, made safe to show:
// it is any device's to choose and ends up in terminals and windows, so
// only what prints is kept (no control characters, so no escape sequences,
// and none of the invisible ones that reorder text), and not much of it.
func (m *Message) Hostname() string {
	var b strings.Builder
	for _, r := range strings.ToValidUTF8(string(m.Opts[optHostname]), "") {
		if !unicode.IsPrint(r) {
			continue
		}
		if b.Len()+utf8.RuneLen(r) > maxHostname {
			break
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// Broadcast says whether the client asked to be answered by broadcast.
func (m *Message) Broadcast() bool { return m.Flags&flagBroadcast != 0 }

// Parse reads a packet.
func Parse(b []byte) (*Message, error) {
	if len(b) < headerLen+4 || [4]byte(b[headerLen:headerLen+4]) != magic {
		return nil, errors.New("dhcp: not a DHCP packet")
	}
	hlen := int(b[2])
	if b[1] != 1 || hlen != 6 { // Ethernet
		return nil, errors.New("dhcp: not an Ethernet client")
	}
	m := &Message{
		Op: b[0], XID: binary.BigEndian.Uint32(b[4:]), Flags: binary.BigEndian.Uint16(b[10:]),
		CIAddr: v4(b[12:16]), YIAddr: v4(b[16:20]), GIAddr: v4(b[24:28]),
		CHAddr: net.HardwareAddr(append([]byte(nil), b[28:28+hlen]...)),
		Opts:   map[byte][]byte{},
	}
	for o := b[headerLen+4:]; len(o) > 0; {
		code := o[0]
		if code == optEnd {
			break
		}
		if code == optPad {
			o = o[1:]
			continue
		}
		if len(o) < 2 || len(o) < 2+int(o[1]) {
			return nil, errors.New("dhcp: an option runs past the packet")
		}
		// A repeated option continues the one before (RFC 3396).
		m.Opts[code] = append(m.Opts[code], o[2:2+int(o[1])]...)
		o = o[2+int(o[1]):]
	}
	return m, nil
}

func v4(b []byte) netip.Addr {
	a := netip.AddrFrom4([4]byte(b))
	if a.IsUnspecified() {
		return netip.Addr{}
	}
	return a
}

func put4(b []byte, a netip.Addr) {
	if a.Is4() {
		v := a.As4()
		copy(b, v[:])
	}
}

// Marshal writes the packet, with the message type first among the options
// and the rest in the order of their codes.
func (m *Message) Marshal() []byte {
	b := make([]byte, headerLen+4, minLen)
	b[0], b[1], b[2] = m.Op, 1, 6
	binary.BigEndian.PutUint32(b[4:], m.XID)
	binary.BigEndian.PutUint16(b[10:], m.Flags)
	put4(b[12:], m.CIAddr)
	put4(b[16:], m.YIAddr)
	put4(b[24:], m.GIAddr)
	copy(b[28:44], m.CHAddr)
	copy(b[headerLen:], magic[:])
	add := func(code byte) {
		if v, ok := m.Opts[code]; ok && len(v) <= 255 {
			b = append(append(b, code, byte(len(v))), v...)
		}
	}
	add(optType)
	for code := 1; code < 255; code++ {
		if byte(code) != optType {
			add(byte(code))
		}
	}
	b = append(b, optEnd)
	for len(b) < minLen {
		b = append(b, optPad)
	}
	return b
}

func addrBytes(a netip.Addr) []byte { v := a.As4(); return v[:] }

func secs(n uint32) []byte { return binary.BigEndian.AppendUint32(nil, n) }
