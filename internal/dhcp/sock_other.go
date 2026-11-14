//go:build !darwin && !linux

package dhcp

import (
	"errors"
	"net"
)

// Supported says whether this system can probe for and serve DHCP.
const Supported = false

func listen(*net.Interface, int) (net.PacketConn, error) {
	return nil, errors.New("dhcp: not supported on this system")
}
