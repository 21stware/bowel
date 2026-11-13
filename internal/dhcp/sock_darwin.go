package dhcp

import (
	"net"

	"golang.org/x/sys/unix"
)

func bindToInterface(fd int, ifc *net.Interface) error {
	return unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_BOUND_IF, ifc.Index)
}
