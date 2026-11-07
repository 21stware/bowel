package sysproxy

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Check reads the system proxy with scutil --proxy and tells whether it
// points at the core's mixed port. It changes nothing.
func Check(port int) (Status, error) {
	out, err := exec.Command("scutil", "--proxy").Output()
	if err != nil {
		return Status{}, fmt.Errorf("scutil --proxy: %w", err)
	}
	return parseScutil(string(out), port), nil
}

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

// parseScutil reads the output of scutil --proxy, e.g.
//
//	<dictionary> {
//	  HTTPEnable : 1
//	  HTTPPort : 7890
//	  HTTPProxy : 127.0.0.1
//	  …
//	}
//
// port is the core's mixed port, used to decide Ours.
func parseScutil(out string, port int) Status {
	kv := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), " : ")
		if !ok {
			continue
		}
		kv[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	get := func(prefix string) endpoint {
		p, _ := strconv.Atoi(kv[prefix+"Port"])
		return endpoint{on: kv[prefix+"Enable"] == "1", host: kv[prefix+"Proxy"], port: p}
	}
	http, https, socks := get("HTTP"), get("HTTPS"), get("SOCKS")
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
