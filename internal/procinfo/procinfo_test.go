//go:build darwin

package procinfo

import (
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
)

// It skips when macOS hides the socket list from this process, as it does
// when the test binary's parent is ad-hoc signed; under the official go
// tool, or run from a shell, it runs.
func TestFindOwnSockets(t *testing.T) {
	if socks, err := list("tcp"); err != nil || len(socks) == 0 {
		t.Skipf("the socket list is hidden from this process (%d sockets, %v)", len(socks), err)
	}
	self, _ := os.Executable()
	self, _ = filepath.EvalSymlinks(self)

	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		if c, err := ln.Accept(); err == nil {
			defer c.Close()
			_, _ = c.Read(make([]byte, 1))
		}
	}()
	c, err := net.Dial("tcp4", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	src := c.LocalAddr().(*net.TCPAddr).AddrPort()

	f := New()
	if got := f.Find("tcp", src); got != self {
		t.Errorf("tcp %v: %q, want %q", src, got, self)
	}
	// an IPv4-mapped address names the same socket
	mapped := netip.AddrPortFrom(netip.AddrFrom16(src.Addr().As16()), src.Port())
	if got := f.Find("tcp", mapped); got != self {
		t.Errorf("tcp %v: %q", mapped, got)
	}

	u, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6loopback})
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	if got := f.Find("udp", u.LocalAddr().(*net.UDPAddr).AddrPort()); got != self {
		t.Errorf("udp6: %q", got)
	}

	if got := f.Find("tcp", netip.MustParseAddrPort("127.0.0.1:1")); got != "" {
		t.Errorf("no socket: %q", got)
	}
}

// What a hidden list looks like: the header and the trailer, no sockets.
func TestParseHidden(t *testing.T) {
	b := make([]byte, 48)
	b[0], b[24] = 24, 24
	socks, err := parse(b)
	if err != nil || len(socks) != 0 {
		t.Fatalf("%v %v", socks, err)
	}
}
