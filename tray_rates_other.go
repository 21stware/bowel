//go:build !darwin

package main

// systemRateFont is none: the rates stand on two lines only in the macOS
// menu bar (traySpeedStacks), and are drawn elsewhere, as in tests, in Go
// Medium (rateFace).
func systemRateFont() rateFont { return nil }
