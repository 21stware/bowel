//go:build !darwin

package procinfo

import (
	"errors"
	"net/netip"
)

// Elsewhere the core names processes itself.

func list(string) (map[netip.AddrPort]int32, error) { return nil, errors.ErrUnsupported }

func pidPath(int32) string { return "" }
