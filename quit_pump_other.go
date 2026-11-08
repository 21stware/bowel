//go:build !darwin

package main

// pumpUntil waits for done.
func pumpUntil(done <-chan struct{}) { <-done }
