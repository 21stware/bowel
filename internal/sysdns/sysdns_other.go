//go:build !darwin

package sysdns

import "context"

// Supported says whether this system's DNS servers are bowel's to set. On
// Linux the core points systemd-resolved at itself, and Windows is left
// for later (docs/platform-parity.md).
const Supported = false

func Snapshot(context.Context) (Saved, error) { return Saved{}, nil }

func Write(context.Context, []Service) error { return nil }

func DHCP(context.Context, string) []string { return nil }
