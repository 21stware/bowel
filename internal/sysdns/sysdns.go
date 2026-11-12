// Package sysdns points the system's DNS servers at addresses that route
// into TUN while TUN is on, and puts them back afterwards.
//
// On macOS the resolver a network hands out is usually the router, an
// address of the LAN. Queries to it leave by the physical interface and
// never reach the TUN device, so the core's dns-hijack does not see them:
// the system gets real addresses, connections arrive as bare IPs, and no
// rule that names a domain matches (#117).
package sysdns

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"
)

// Placeholder is what a service's DNS servers are set to while TUN is on.
// Any address that routes into TUN would do, since dns-hijack answers in
// its place. These are public resolvers so that, should they ever be left
// behind, names still resolve.
var Placeholder = []string{"223.5.5.5", "119.29.29.29"}

// Service is one network service's DNS servers as the user set them; none
// means the service uses what its network hands out.
type Service struct {
	Name string `json:"name"`
	// Device is the service's interface (en0), where known.
	Device  string   `json:"device,omitempty"`
	Servers []string `json:"servers"`
}

// Saved is the DNS servers of every enabled service.
type Saved struct {
	Services []Service `json:"services"`
}

func (s Saved) of(name string) (Service, bool) {
	for _, sv := range s.Services {
		if sv.Name == name {
			return sv, true
		}
	}
	return Service{}, false
}

// Held says whether sv has the placeholder.
func Held(sv Service) bool { return slices.Equal(sv.Servers, Placeholder) }

// HeldAll says whether every service of cur has the placeholder, and there
// is at least one.
func HeldAll(cur Saved) bool {
	return len(cur.Services) > 0 && !slices.ContainsFunc(cur.Services, func(sv Service) bool { return !Held(sv) })
}

// Keep is what to remember before the placeholder is written: for each
// service of cur what it has now, except that a service which already has
// the placeholder keeps what old remembers of it (the first copy), or none.
// What old remembers of services cur does not list stays, for when they
// come back.
func Keep(old, cur Saved) Saved {
	var out Saved
	for _, sv := range cur.Services {
		if Held(sv) {
			was, _ := old.of(sv.Name)
			sv.Servers = was.Servers
			if Held(sv) {
				sv.Servers = nil
			}
		}
		out.Services = append(out.Services, sv)
	}
	for _, sv := range old.Services {
		if _, ok := cur.of(sv.Name); !ok {
			out.Services = append(out.Services, sv)
		}
	}
	return out
}

// TakeSteps are the services of cur to give the placeholder.
func TakeSteps(cur Saved) []Service {
	var steps []Service
	for _, sv := range cur.Services {
		if !Held(sv) {
			steps = append(steps, Service{Name: sv.Name, Servers: Placeholder})
		}
	}
	return steps
}

// RestoreSteps are the writes that put saved back: only services that
// still have the placeholder are touched, since anything else was set by
// the user or another program after bowel. One that saved does not know
// goes back to what its network hands out.
func RestoreSteps(saved, cur Saved) []Service {
	var steps []Service
	for _, sv := range cur.Services {
		if !Held(sv) {
			continue
		}
		was, _ := saved.of(sv.Name)
		if Held(was) {
			was.Servers = nil
		}
		steps = append(steps, Service{Name: sv.Name, Servers: was.Servers})
	}
	return steps
}

// Local are the servers the network's own names are to be asked of while
// the placeholder is set: what the user had set on the service of iface,
// or else dhcp, what the network hands out there.
func Local(saved Saved, iface string, dhcp []string) []string {
	if iface != "" {
		for _, sv := range saved.Services {
			if sv.Device == iface && len(sv.Servers) > 0 && !Held(sv) {
				return Usable(sv.Servers)
			}
		}
	}
	return Usable(dhcp)
}

// Usable are the servers of list the core can be given as they are: plain
// addresses, without a zone, and not a TUN's fake resolver.
func Usable(list []string) []string {
	var out []string
	for _, s := range list {
		ip, err := netip.ParseAddr(s)
		if err != nil || ip.Zone() != "" || ip.IsLoopback() || fake.Contains(ip) || slices.Contains(Placeholder, s) || slices.Contains(out, s) {
			continue
		}
		out = append(out, s)
	}
	return out
}

var fake = netip.MustParsePrefix("198.18.0.0/15")

// parseGet reads `networksetup -getdnsservers`: one address a line, or a
// sentence saying there are none.
func parseGet(out string) ([]string, error) {
	var list []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, err := netip.ParseAddr(line); err != nil {
			if len(list) == 0 && strings.HasPrefix(line, "There aren't any DNS Servers") {
				return nil, nil
			}
			return nil, fmt.Errorf("networksetup -getdnsservers: %s", line)
		}
		list = append(list, line)
	}
	return list, nil
}

// parseOrder reads `networksetup -listnetworkserviceorder`: the device of
// each service, by its name.
func parseOrder(out string) map[string]string {
	devices := map[string]string{}
	name := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "(Hardware Port:"):
			if _, dev, ok := strings.Cut(line, "Device:"); ok && name != "" {
				if dev = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(dev), ")")); dev != "" {
					devices[name] = dev
				}
			}
			name = ""
		case strings.HasPrefix(line, "("):
			// "(1) Wi-Fi", or "(*) Wi-Fi" for a disabled one.
			if _, n, ok := strings.Cut(line, ") "); ok {
				name = strings.TrimSpace(n)
			}
		}
	}
	return devices
}

// parsePacket reads the DNS servers out of `ipconfig getpacket`:
// "domain_name_server (ip_mult): {192.168.1.1, 192.168.1.2}".
func parsePacket(out string) []string {
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "domain_name_server") {
			continue
		}
		_, v, _ := strings.Cut(line, ":")
		v = strings.Trim(strings.TrimSpace(v), "{}")
		var list []string
		for _, s := range strings.Split(v, ",") {
			if s = strings.TrimSpace(s); s != "" {
				list = append(list, s)
			}
		}
		return list
	}
	return nil
}

// setArgs is the networksetup call that writes sv.
func setArgs(sv Service) []string {
	args := []string{"-setdnsservers", sv.Name}
	if len(sv.Servers) == 0 {
		return append(args, "Empty") // networksetup's word for none
	}
	return append(args, sv.Servers...)
}
