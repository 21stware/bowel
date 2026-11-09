package devnet

import (
	"context"
	"os/exec"
	"time"
)

// osDetect reads what only macOS's own tools tell: the routes and the DNS
// the system splits by domain.
func osDetect(ctx context.Context) (routes []Route, resolvers []Resolver) {
	if out, err := run(ctx, "/usr/sbin/scutil", "--dns"); err == nil {
		resolvers = parseResolvers(out, proxyDNS)
	}
	if out, err := run(ctx, "/usr/sbin/netstat", "-rn"); err == nil {
		routes = parseRoutes(out)
	}
	return routes, resolvers
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(c, name, args...).Output()
	return string(out), err
}
