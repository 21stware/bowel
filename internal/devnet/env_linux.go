package devnet

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/oboo/moho/internal/netif"
)

// osDetect reads the routes of every table with iproute2, and the DNS
// the system splits by domain from systemd-resolved, where it runs.
// Without it the system has one set of servers, and nothing is split:
// those of /etc/resolv.conf, which nothing points at the core under TUN.
func osDetect(ctx context.Context) (routes []Route, resolvers []Resolver, defaultDNS []string) {
	routes = fromSystem(netif.Routes(ctx))
	for _, bin := range []string{"resolvectl", "/usr/bin/resolvectl", "/bin/resolvectl"} {
		path, err := exec.LookPath(bin)
		if err != nil {
			continue
		}
		dns, err1 := run(ctx, path, "dns")
		domains, err2 := run(ctx, path, "domain")
		if err1 == nil && err2 == nil {
			return routes, parseResolved(dns, domains, proxyDNS, isTunnel), nil
		}
		break // resolvectl is there, systemd-resolved is not running
	}
	if b, err := os.ReadFile("/etc/resolv.conf"); err == nil {
		defaultDNS = parseResolvConf(string(b))
	}
	return routes, resolvers, defaultDNS
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(c, name, args...).Output()
	return string(out), err
}
