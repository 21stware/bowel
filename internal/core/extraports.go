package core

import (
	"net"
	"sync"
)

// anyAddr is the address an extra port open to the LAN listens on: every
// address of both families, as the mixed port's ":port" does, where the
// system has IPv6 sockets, and every IPv4 one where it has none (a kernel
// started with IPv6 off), on which "::" would leave the port closed. A
// listener's address is an IP, so it can't be ":port": the core reads ""
// as 0.0.0.0, IPv4 alone.
var anyAddr = sync.OnceValue(func() string {
	l, err := net.Listen("tcp", "[::]:0")
	if err != nil {
		return "0.0.0.0"
	}
	l.Close()
	return "::"
})
