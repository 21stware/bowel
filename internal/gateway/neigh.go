package gateway

import (
	"net/netip"
	"strings"
)

// Neighbours are the devices this computer has lately exchanged packets
// with on its LANs, by address, with their hardware addresses: the
// system's ARP table. As a gateway that is every device that uses it.
func Neighbours() map[netip.Addr]string { return neighbours() }

// parseARP reads `arp -an`: "? (192.168.1.31) at aa:bb:cc:dd:ee:ff on en0 …".
func parseARP(out string) map[netip.Addr]string {
	m := map[netip.Addr]string{}
	for _, line := range strings.Split(out, "\n") {
		open, close := strings.IndexByte(line, '('), strings.IndexByte(line, ')')
		_, rest, ok := strings.Cut(line, " at ")
		if open < 0 || close < open || !ok {
			continue
		}
		mac, _, _ := strings.Cut(rest, " ")
		a, err := netip.ParseAddr(line[open+1 : close])
		if err != nil || !strings.Contains(mac, ":") || strings.HasPrefix(mac, "ff:ff:ff") {
			continue
		}
		m[a] = normalMAC(mac)
	}
	return m
}

// parseProcARP reads Linux's /proc/net/arp: address, type, flags, hardware
// address, mask, device. Flags 0x0 is an entry that was never answered.
func parseProcARP(out string) map[netip.Addr]string {
	m := map[netip.Addr]string{}
	for i, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if i == 0 || len(f) < 4 || f[2] == "0x0" || f[3] == "00:00:00:00:00:00" {
			continue
		}
		if a, err := netip.ParseAddr(f[0]); err == nil {
			m[a] = normalMAC(f[3])
		}
	}
	return m
}

// normalMAC writes each byte with two digits, as macOS's arp does not.
func normalMAC(s string) string {
	parts := strings.Split(strings.ToLower(s), ":")
	for i, p := range parts {
		if len(p) == 1 {
			parts[i] = "0" + p
		}
	}
	return strings.Join(parts, ":")
}
