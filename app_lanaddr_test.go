package main

import (
	"net"
	"testing"
)

func ifaceAddrs(t *testing.T, cidrs ...string) []net.Addr {
	t.Helper()
	var out []net.Addr
	for _, c := range cidrs {
		ip, n, err := net.ParseCIDR(c)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, &net.IPNet{IP: ip, Mask: n.Mask})
	}
	return out
}

// An adapter that is down can still hold an address: Windows keeps a
// static one on an adapter whose cable is out, and lists that adapter
// first when its metric is the lowest. Nothing answers there, so it is
// not the address other devices are told to connect to (#124). The
// addresses are a Windows 11 machine's, as Go read them.
func TestLanIPSkipsAnInterfaceThatIsDown(t *testing.T) {
	const up = net.FlagUp | net.FlagBroadcast | net.FlagMulticast | net.FlagRunning
	unplugged := ifaceAddrs(t, "fe80::e997:9899:5b37:d6fe/64", "169.254.88.193/16", "192.168.50.100/24")
	ethernet := ifaceAddrs(t, "fe80::aaa5:25b3:85d:294d/64", "172.30.1.2/24")
	loopback := ifaceAddrs(t, "::1/128", "127.0.0.1/8")

	if got := lanIP(0, unplugged); got != "" {
		t.Errorf("an adapter that is down gave %q", got)
	}
	if got := lanIP(up, ethernet); got != "172.30.1.2" {
		t.Errorf("the adapter in use gave %q, want 172.30.1.2", got)
	}
	if got := lanIP(up, unplugged); got != "192.168.50.100" {
		t.Errorf("the same adapter plugged in gave %q, want 192.168.50.100", got)
	}
	if got := lanIP(net.FlagUp|net.FlagLoopback, loopback); got != "" {
		t.Errorf("loopback gave %q", got)
	}
}
