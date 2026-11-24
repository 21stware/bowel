//go:build !linux

package main

// reapOrphans has nothing to do: bowel runs in Linux containers only.
func reapOrphans(<-chan struct{}) {}
