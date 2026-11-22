package core

import (
	"errors"
	"net"
	"strconv"
	"testing"
)

// refuseUDP has listenUDP refuse the ports refused says no to, as Windows
// does for a port in a block reserved for UDP, and returns the ports it
// was asked for. refused sees each port and the ports asked for before it.
func refuseUDP(t *testing.T, refused func(port int, before []int) bool) *[]int {
	t.Helper()
	var asked []int
	real := listenUDP
	listenUDP = func(network, addr string) (net.PacketConn, error) {
		_, p, err := net.SplitHostPort(addr)
		if err != nil {
			t.Fatal(err)
		}
		port, _ := strconv.Atoi(p)
		no := refused(port, asked)
		asked = append(asked, port)
		if no {
			return nil, errors.New("bind: An attempt was made to access a socket in a way forbidden by its access permissions.")
		}
		return real(network, addr)
	}
	t.Cleanup(func() { listenUDP = real })
	return &asked
}

// firstN refuses the first n ports asked for.
func firstN(n int) func(int, []int) bool {
	return func(_ int, before []int) bool { return len(before) < n }
}

// Each port FreePort gives binds for TCP and UDP. On a Windows machine with
// ports reserved for UDP, enough calls land in such a block.
func TestFreePortTakesBoth(t *testing.T) {
	for range 200 {
		p, err := FreePort()
		if err != nil {
			t.Fatal(err)
		}
		addr := "127.0.0.1:" + strconv.Itoa(p)
		l, err := net.Listen("tcp", addr)
		if err != nil {
			t.Fatalf("TCP on %d: %v", p, err)
		}
		u, err := net.ListenPacket("udp", addr)
		l.Close()
		if err != nil {
			t.Fatalf("UDP on %d: %v", p, err)
		}
		u.Close()
	}
}

func TestFreePortSkipsPortsUDPRefuses(t *testing.T) {
	asked := refuseUDP(t, firstN(2))
	p, err := FreePort()
	if err != nil {
		t.Fatal(err)
	}
	// A random try can also land on a port the system refuses for real.
	a := *asked
	if len(a) < 3 {
		t.Fatalf("asked UDP for %v, want at least 3 ports", a)
	}
	if p != a[len(a)-1] {
		t.Errorf("got %d, want the port UDP took, %d", p, a[len(a)-1])
	}
}

// Windows hands its ports out in order, so after a port in a reserved block
// it offers the next one, in the block too. A block longer than the tries
// must still leave FreePort a port outside it.
func TestFreePortLeavesABlock(t *testing.T) {
	const block = 1000
	asked := refuseUDP(t, func(p int, before []int) bool {
		first := p
		if len(before) > 0 {
			first = before[0]
		}
		return p >= first && p < first+block
	})
	p, err := FreePort()
	if err != nil {
		t.Fatalf("no port after asking UDP for %d: %v", len(*asked), err)
	}
	if first := (*asked)[0]; p >= first && p < first+block {
		t.Errorf("got %d, in the refused block from %d", p, first)
	}
}

func TestFreePortGivesUp(t *testing.T) {
	asked := refuseUDP(t, func(int, []int) bool { return true })
	if p, err := FreePort(); err == nil {
		t.Fatalf("got %d with every UDP bind refused", p)
	}
	if n := len(*asked); n == 0 || n > freePortTries {
		t.Errorf("asked UDP for %d ports, want 1 to %d", n, freePortTries)
	}
}
