//go:build !linux

package tun

import "context"

// NeedsRule is always false: only Linux leaves a polkit rule.
func NeedsRule(string) bool { return false }

// LinkDNS is never known: only Linux has systemd-resolved to ask.
func LinkDNS(context.Context) (pointed, known bool) { return false, false }
