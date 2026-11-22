//go:build !unix

package owner

import "net"

// listenPrivate is plain on systems without a umask, where only the tests run.
func listenPrivate(path string) (net.Listener, error) {
	return net.Listen("unix", path)
}
