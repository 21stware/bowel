//go:build !darwin && !linux && !windows

package procinfo

func running() []string { return nil }
