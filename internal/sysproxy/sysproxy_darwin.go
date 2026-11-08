// Package sysproxy points the system's HTTP, HTTPS and SOCKS proxies at the
// core.
package sysproxy

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultBypass is macOS's own bypass list for a new service.
var DefaultBypass = []string{"*.local", "169.254/16"}

const (
	// perCall bounds one networksetup or scutil: a stuck one must not hold
	// up quitting or a change of settings.
	perCall = 5 * time.Second
	// readLimit bounds what Restore reads first. Reading saves work, so
	// that it is worth waiting for only briefly.
	readLimit = 1500 * time.Millisecond
)

// command is exec.CommandContext, bounded by perCall. Call cancel when done.
func command(ctx context.Context, name string, args ...string) (*exec.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, perCall)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = time.Second
	return cmd, cancel
}

// services lists the enabled network services (Wi-Fi, Ethernet, …).
func services(ctx context.Context) ([]string, error) {
	cmd, cancel := command(ctx, "networksetup", "-listallnetworkservices")
	defer cancel()
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var list []string
	for i, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		// The first line is a notice; disabled services start with '*'.
		if i == 0 || line == "" || strings.HasPrefix(line, "*") {
			continue
		}
		list = append(list, line)
	}
	return list, nil
}

func run(ctx context.Context, args ...string) error {
	cmd, cancel := command(ctx, "networksetup", args...)
	defer cancel()
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("networksetup %s: %s", args[0], strings.TrimSpace(string(out)))
	}
	return nil
}

// Enable sets every service's proxies to host:port, with the hosts and
// networks in bypass left out of them.
func Enable(host string, port int, bypass []string) error {
	ctx := context.Background()
	list, err := services(ctx)
	if err != nil {
		return err
	}
	p := strconv.Itoa(port)
	for _, s := range list {
		for _, kind := range []string{"-setwebproxy", "-setsecurewebproxy", "-setsocksfirewallproxy"} {
			if err := run(ctx, kind, s, host, p); err != nil {
				return err
			}
		}
		domains := bypass
		if len(domains) == 0 {
			domains = []string{"Empty"} // networksetup's word for none
		}
		if err := run(ctx, append([]string{"-setproxybypassdomains", s}, domains...)...); err != nil {
			return err
		}
	}
	return nil
}

// Disable turns the proxies of every service off. It goes on past a
// failure, and returns them all.
func Disable() error {
	ctx := context.Background()
	list, err := services(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, s := range list {
		for _, kind := range []string{"-setwebproxystate", "-setsecurewebproxystate", "-setsocksfirewallproxystate"} {
			errs = append(errs, run(ctx, kind, s, "off"))
		}
	}
	return errors.Join(errs...)
}

// kinds are networksetup's names of a service's three proxies, in the
// order of Service.endpoints.
var kinds = []string{"webproxy", "securewebproxy", "socksfirewallproxy"}

// Snapshot reads every enabled service's proxies and bypass list. It
// changes nothing.
func Snapshot() (Saved, error) {
	ctx := context.Background()
	list, err := services(ctx)
	if err != nil {
		return Saved{}, err
	}
	var saved Saved
	for _, s := range list {
		sv, err := readService(ctx, s)
		if err != nil {
			return Saved{}, err
		}
		saved.Services = append(saved.Services, sv)
	}
	return saved, nil
}

// readService reads one service's proxies and bypass list.
func readService(ctx context.Context, s string) (Service, error) {
	get := func(what string) (string, error) {
		cmd, cancel := command(ctx, "networksetup", "-get"+what, s)
		defer cancel()
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("networksetup -get%s: %w", what, err)
		}
		return string(out), nil
	}
	sv := Service{Name: s}
	for i, e := range sv.endpoints() {
		out, err := get(kinds[i])
		if err != nil {
			return Service{}, err
		}
		*e = parseGet(out)
	}
	out, err := get("proxybypassdomains")
	if err != nil {
		return Service{}, err
	}
	sv.Bypass = parseBypass(out)
	return sv, nil
}

// parseGet reads networksetup -getwebproxy and its kin:
//
//	Enabled: Yes
//	Server: 127.0.0.1
//	Port: 7899
//	Authenticated Proxy Enabled: 0
func parseGet(out string) Endpoint {
	var e Endpoint
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "Enabled":
			e.On = v == "Yes"
		case "Server":
			e.Host = v
		case "Port":
			e.Port, _ = strconv.Atoi(v)
		}
	}
	return e
}

// parseBypass reads networksetup -getproxybypassdomains: one entry a line,
// or a sentence when there are none. Entries have no spaces; sentences do.
func parseBypass(out string) []string {
	var list []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.ContainsAny(line, " \t") {
			list = append(list, line)
		}
	}
	return list
}

// Restore puts every service's proxies and bypass list back as saved has
// them. A service saved has not heard of gets its proxies turned off. It
// reads first and writes only what differs, as each write is a process of
// its own and a Mac can have many services. It goes on past a failure, and
// returns them all; when ctx ends it stops there.
func Restore(ctx context.Context, saved Saved) error {
	list, err := services(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, args := range restoreSteps(list, saved, readAll(ctx, list)) {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		errs = append(errs, run(ctx, args...))
	}
	return errors.Join(errs...)
}

// readAll reads the services side by side, as reading does not need the
// lock writing does. A service missing from the result could not be read.
func readAll(ctx context.Context, list []string) map[string]Service {
	ctx, cancel := context.WithTimeout(ctx, readLimit)
	defer cancel()
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		cur = map[string]Service{}
	)
	for _, s := range list {
		wg.Go(func() {
			if sv, err := readService(ctx, s); err == nil {
				mu.Lock()
				cur[s] = sv
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	return cur
}

// restoreSteps lists the networksetup calls that take the services from
// cur to saved. Turning proxies off comes first, as that is what gives the
// network back should time run out. A service missing from cur gets every
// call. A proxy saved with no server is only turned off: networksetup has
// no word for no server, and a stale one does nothing while it is off (and
// Saved.Forget drops it from the next snapshot).
func restoreSteps(list []string, saved Saved, cur map[string]Service) [][]string {
	byName := map[string]Service{}
	for _, sv := range saved.Services {
		byName[sv.Name] = sv
	}
	var off, on, bypass [][]string
	for _, s := range list {
		sv, known := byName[s]
		c, read := cur[s]
		wanted, had := sv.endpoints(), c.endpoints()
		for i, kind := range kinds {
			want, have := *wanted[i], *had[i]
			// Setting a server also turns its proxy on.
			set := []string{"-set" + kind, s, want.Host, strconv.Itoa(want.Port)}
			state := []string{"-set" + kind + "state", s, "off"}
			switch {
			case want.On && want.Host != "":
				if !read || have != want {
					on = append(on, set)
				}
			case want.Host != "":
				if !read || have.On || have.Host != want.Host || have.Port != want.Port {
					off = append(off, set, state)
				}
			default:
				if !read || have.On {
					off = append(off, state)
				}
			}
		}
		if known && (!read || !slices.Equal(c.Bypass, sv.Bypass)) {
			domains := sv.Bypass
			if len(domains) == 0 {
				domains = []string{"Empty"}
			}
			bypass = append(bypass, append([]string{"-setproxybypassdomains", s}, domains...))
		}
	}
	return slices.Concat(off, on, bypass)
}
