package dhcp

import (
	"net"

	"golang.org/x/sys/unix"
)

// bindToInterface needs CAP_NET_RAW: root, as a gateway's daemon is.
func bindToInterface(fd int, ifc *net.Interface) error {
	return unix.BindToDevice(fd, ifc.Name)
}
