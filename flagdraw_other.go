//go:build !darwin

package main

// flagDrawn reports whether the emoji font draws flag as a flag; only macOS
// is known to draw some flags as something else (flagdraw_darwin.go).
func flagDrawn(flag string) bool { return true }
