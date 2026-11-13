//go:build darwin || linux

package dhcp

import (
	"context"
	"fmt"
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

// Supported says whether this system can probe for and serve DHCP.
const Supported = true

// listen opens a UDP socket on port for the interface alone: what it sends
// to the broadcast address leaves by that interface, whatever the routes
// say (TUN's would take it), and it hears only what arrives there. Others
// may hold the port too (the system's own DHCP client holds 68).
func listen(ifc *net.Interface, port int) (net.PacketConn, error) {
	var serr error
	lc := net.ListenConfig{Control: func(_, _ string, c syscall.RawConn) error {
		return c.Control(func(fd uintptr) {
			f := int(fd)
			for _, opt := range []int{unix.SO_REUSEADDR, unix.SO_REUSEPORT, unix.SO_BROADCAST} {
				if err := unix.SetsockoptInt(f, unix.SOL_SOCKET, opt, 1); err != nil {
					serr = err
					return
				}
			}
			serr = bindToInterface(f, ifc)
		})
	}}
	c, err := lc.ListenPacket(context.Background(), "udp4", fmt.Sprintf(":%d", port))
	if err == nil && serr != nil {
		c.Close()
		err = serr
	}
	if err != nil {
		return nil, fmt.Errorf("dhcp: port %d on %s: %w", port, ifc.Name, err)
	}
	return c, nil
}
