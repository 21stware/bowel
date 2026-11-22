//go:build unix

package owner

import (
	"net"
	"syscall"
)

// listenPrivate creates the socket with no access for other users. The core
// calls it while it starts, before anything else could create a file.
func listenPrivate(path string) (net.Listener, error) {
	old := syscall.Umask(0o177)
	defer syscall.Umask(old)
	return net.Listen("unix", path)
}
