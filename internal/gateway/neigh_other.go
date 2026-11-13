//go:build !darwin && !linux

package gateway

import "net/netip"

func neighbours() map[netip.Addr]string { return nil }
