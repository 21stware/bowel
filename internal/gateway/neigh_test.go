package gateway

import (
	"net/netip"
	"testing"
)

func TestParseARP(t *testing.T) {
	m := parseARP(`? (192.168.1.1) at a4:2b:b0:1:2:3 on en0 ifscope [ethernet]
? (192.168.1.31) at 2:0:0:0:0:1f on en0 ifscope [ethernet]
? (192.168.1.77) at (incomplete) on en0 ifscope [ethernet]
? (192.168.1.255) at ff:ff:ff:ff:ff:ff on en0 ifscope [ethernet]
mdns.mcast.net (224.0.0.251) at 1:0:5e:0:0:fb on en0 ifscope permanent [ethernet]
`)
	if len(m) != 3 || m[netip.MustParseAddr("192.168.1.1")] != "a4:2b:b0:01:02:03" || m[netip.MustParseAddr("192.168.1.31")] != "02:00:00:00:00:1f" {
		t.Fatalf("%v", m)
	}
}

func TestParseProcARP(t *testing.T) {
	m := parseProcARP(`IP address       HW type     Flags       HW address            Mask     Device
172.31.77.10     0x1         0x2         02:42:ac:1f:4d:0a     *        eth0
172.31.77.99     0x1         0x0         00:00:00:00:00:00     *        eth0
`)
	if len(m) != 1 || m[netip.MustParseAddr("172.31.77.10")] != "02:42:ac:1f:4d:0a" {
		t.Fatalf("%v", m)
	}
}
