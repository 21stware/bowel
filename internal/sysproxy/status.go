package sysproxy

import "strings"

// Status is the system proxy as the OS has it now.
type Status struct {
	On   bool   // any of HTTP, HTTPS, SOCKS enabled
	Host string // the HTTP proxy host (or HTTPS/SOCKS if HTTP is off)
	Port int
	Ours bool // HTTP and HTTPS enabled and pointing at 127.0.0.1/localhost:port
}

type endpoint struct {
	on   bool
	host string
	port int
}

func (e endpoint) local(port int) bool {
	h := strings.ToLower(e.host)
	return e.on && e.port == port && (h == "127.0.0.1" || h == "localhost" || h == "::1")
}

// status sums up the three proxies: the first one on, and whether HTTP and
// HTTPS both point at port.
func status(http, https, socks endpoint, port int) Status {
	var s Status
	for _, e := range []endpoint{http, https, socks} {
		if e.on {
			s.On, s.Host, s.Port = true, e.host, e.port
			break
		}
	}
	s.Ours = http.local(port) && https.local(port)
	return s
}
