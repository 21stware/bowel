//go:build !darwin

package tun

func signedApart(bundled, installed string) bool { return false }
