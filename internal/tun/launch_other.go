//go:build !windows

package tun

// Launch is the app run as Windows' launcher of TUN's core. Nothing runs
// it anywhere else.
func Launch([]string) int { return launchBadArgs }
