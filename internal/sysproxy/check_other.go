//go:build !darwin

package sysproxy

// Status is the system proxy as the OS has it now.
type Status struct {
	On   bool   // any of HTTP, HTTPS, SOCKS enabled
	Host string // the HTTP proxy host (or HTTPS/SOCKS if HTTP is off)
	Port int
	Ours bool // HTTP and HTTPS enabled and pointing at 127.0.0.1/localhost:port
}

// Check is not implemented outside macOS: it reports the proxy as off.
func Check(port int) (Status, error) { return Status{}, nil }
