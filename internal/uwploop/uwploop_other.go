//go:build !windows

package uwploop

import "errors"

// Supported is whether there are packaged apps kept from the loopback:
// Windows alone has them.
const Supported = false

var errUnsupported = errors.New("uwploop: not on this system")

func init() {
	containers = func() ([]string, error) { return nil, errUnsupported }
	exempted = func() ([]string, error) { return nil, errUnsupported }
	direct = func(string, bool) error { return errUnsupported }
	elevate = func(string) error { return errUnsupported }
	tool = func() string { return "" }
}
