package core

import (
	"net/netip"
	"strings"

	"github.com/oboo/moho/internal/routes"
)

// DeviceRoute is a device of the gateway's LAN all of whose traffic goes
// one way. The core tells it by its address: what it sees of a forwarded
// connection is the device's own source address.
type DeviceRoute struct {
	IP string // IPv4
	// Route is the id of the route group its traffic goes to; "" is
	// direct, for a device left out of the gateway that still comes
	// through it.
	Route string
}

// A device's route takes all of its traffic, before the route groups'
// rules, with what holds for every device left above it: the developer
// networks (their rules come first in any case), Block, whose rules stay
// in force whatever else would take a connection, and the groups with the
// exit lock, whose traffic leaves by their node or not at all, from a
// device as from this computer. A device that is blocked has nothing above
// it: it gets nothing through the gateway at all.
//
// Held groups at the top of the list are above the devices' rules as they
// are; the lines of those further down are repeated for each device, for
// its address alone, so that the order stays as it was for everyone else.

// heldFor says whether route id's rules hold for a device with a route of
// its own: Block, and a group with the exit lock.
func heldFor(all []routes.Route, id string) bool {
	return id == routes.Reject || isLocked(all, id)
}

// heldLine is a line of a held group's part of the list, below the first
// group that is not held, with the target it was given and whether a UDP
// hold went after it.
type heldLine struct {
	line   routes.Line
	target string
	hold   bool
}

// deviceRules are the lines of the devices: top goes before every other
// rule, section where the first group that is not held starts.
func deviceRules(in Input, held []heldLine, guard bool) (top, section []string) {
	for _, d := range in.Devices {
		ip, err := netip.ParseAddr(d.IP)
		if err != nil || !ip.Is4() {
			continue
		}
		target, locked := "DIRECT", false
		if d.Route != "" {
			var ok bool
			if target, ok = exitOf(in.Routes, d.Route, guard); !ok {
				continue // its group is gone: it follows the rules again
			}
			locked = isLocked(in.Routes, d.Route)
		}
		src := "SRC-IP-CIDR," + ip.String() + "/32"
		if target == "REJECT" {
			top = append(top, src+",REJECT")
			continue
		}
		for _, h := range held {
			// A program is one of this computer's: no device's traffic is.
			if strings.HasPrefix(h.line.Text, "PROCESS-") {
				continue
			}
			sub, ok := logicSafe(h.line.Text)
			if !ok {
				continue
			}
			t := h.target
			if !heldFor(in.Routes, h.line.Route) {
				// An exception of a held group's service: the traffic is not
				// that group's, so it is the device's.
				t = target
			}
			if t == target {
				continue // it goes there anyway, and the device's own hold holds it
			}
			section = append(section, "AND,(("+src+"),("+sub+")),"+t)
			if h.hold {
				section = append(section, "AND,(("+src+"),(NETWORK,UDP),("+sub+")),REJECT")
			}
		}
		section = append(section, src+","+target)
		if locked {
			if hold, ok := udpHold(src); ok {
				section = append(section, hold)
			}
		}
	}
	return top, section
}
