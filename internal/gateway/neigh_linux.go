package gateway

import (
	"net/netip"
	"os"
)

func neighbours() map[netip.Addr]string {
	b, err := os.ReadFile("/proc/net/arp")
	if err != nil {
		return nil
	}
	return parseProcARP(string(b))
}
