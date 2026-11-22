package core

import (
	"errors"
	"net"
	"strconv"
	"testing"
)

// refuseUDP has listenUDP refuse the first n ports it is asked for, as
// Windows does for a port in a block reserved for UDP, and returns the
// ports it was asked for.
func refuseUDP(t *testing.T, n int) *[]int {
	t.Helper()
	var asked []int
	real := listenUDP
	listenUDP = func(network, addr string) (net.PacketConn, error) {
		_, p, err := net.SplitHostPort(addr)
		if err != nil {
			t.Fatal(err)
		}
		port, _ := strconv.Atoi(p)
		asked = append(asked, port)
		if len(asked) <= n {
			return nil, errors.New("bind: An attempt was made to access a socket in a way forbidden by its access permissions.")
		}
		return real(network, addr)
	}
	t.Cleanup(func() { listenUDP = real })
	return &asked
}

func TestFreePortTakesBoth(t *testing.T) {
	p, err := FreePort()
	if err != nil {
		t.Fatal(err)
	}
	addr := "127.0.0.1:" + strconv.Itoa(p)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("TCP on %d: %v", p, err)
	}
	l.Close()
	u, err := net.ListenPacket("udp", addr)
	if err != nil {
		t.Fatalf("UDP on %d: %v", p, err)
	}
	u.Close()
}

func TestFreePortSkipsPortsUDPRefuses(t *testing.T) {
	asked := refuseUDP(t, 2)
	p, err := FreePort()
	if err != nil {
		t.Fatal(err)
	}
	if len(*asked) != 3 {
		t.Fatalf("asked UDP for %v, want 3 ports", *asked)
	}
	a := *asked
	if a[0] == a[1] || a[1] == a[2] || a[0] == a[2] {
		t.Errorf("a refused port was offered again: %v", a)
	}
	if p != a[2] {
		t.Errorf("got %d, want the port UDP took, %d", p, a[2])
	}
}

func TestFreePortGivesUp(t *testing.T) {
	asked := refuseUDP(t, 1000)
	if p, err := FreePort(); err == nil {
		t.Fatalf("got %d with every UDP bind refused", p)
	}
	seen := map[int]bool{}
	for _, p := range *asked {
		if seen[p] {
			t.Fatalf("port %d offered twice: %v", p, *asked)
		}
		seen[p] = true
	}
	if len(*asked) != 20 {
		t.Errorf("tried %d ports, want 20", len(*asked))
	}
}
