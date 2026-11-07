//go:build !darwin && !windows

package sysproxy

// Check is not implemented outside macOS: it reports the proxy as off.
func Check(port int) (Status, error) { return Status{}, nil }
