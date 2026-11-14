//go:build !windows

package main

// taskbarLight is only read on Windows; elsewhere trayInk does not ask.
func taskbarLight() bool { return false }
