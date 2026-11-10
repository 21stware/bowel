//go:build !darwin

package devnet

import "syscall"

// bindToIface does nothing: only macOS runs bowel's TUN, so elsewhere the
// system's route is the one traffic takes.
func bindToIface(string) func(network, address string, c syscall.RawConn) error { return nil }
