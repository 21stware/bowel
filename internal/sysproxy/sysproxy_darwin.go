// Package sysproxy points the system's HTTP, HTTPS and SOCKS proxies at the
// core.
package sysproxy

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// DefaultBypass is macOS's own bypass list for a new service.
var DefaultBypass = []string{"*.local", "169.254/16"}

// services lists the enabled network services (Wi-Fi, Ethernet, …).
func services() ([]string, error) {
	out, err := exec.Command("networksetup", "-listallnetworkservices").Output()
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

func run(args ...string) error {
	if out, err := exec.Command("networksetup", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("networksetup %s: %s", args[0], strings.TrimSpace(string(out)))
	}
	return nil
}

// Enable sets every service's proxies to host:port, with the hosts and
// networks in bypass left out of them.
func Enable(host string, port int, bypass []string) error {
	list, err := services()
	if err != nil {
		return err
	}
	p := strconv.Itoa(port)
	for _, s := range list {
		for _, kind := range []string{"-setwebproxy", "-setsecurewebproxy", "-setsocksfirewallproxy"} {
			if err := run(kind, s, host, p); err != nil {
				return err
			}
		}
		domains := bypass
		if len(domains) == 0 {
			domains = []string{"Empty"} // networksetup's word for none
		}
		if err := run(append([]string{"-setproxybypassdomains", s}, domains...)...); err != nil {
			return err
		}
	}
	return nil
}

// Disable turns the proxies of every service off.
func Disable() error {
	list, err := services()
	if err != nil {
		return err
	}
	for _, s := range list {
		for _, kind := range []string{"-setwebproxystate", "-setsecurewebproxystate", "-setsocksfirewallproxystate"} {
			if err := run(kind, s, "off"); err != nil {
				return err
			}
		}
	}
	return nil
}

// kinds are networksetup's names of a service's three proxies, in the
// order of Service.endpoints.
var kinds = []string{"webproxy", "securewebproxy", "socksfirewallproxy"}

// Snapshot reads every enabled service's proxies and bypass list. It
// changes nothing.
func Snapshot() (Saved, error) {
	list, err := services()
	if err != nil {
		return Saved{}, err
	}
	var saved Saved
	for _, s := range list {
		sv := Service{Name: s}
		for i, e := range sv.endpoints() {
			out, err := exec.Command("networksetup", "-get"+kinds[i], s).Output()
			if err != nil {
				return Saved{}, fmt.Errorf("networksetup -get%s: %w", kinds[i], err)
			}
			*e = parseGet(string(out))
		}
		out, err := exec.Command("networksetup", "-getproxybypassdomains", s).Output()
		if err != nil {
			return Saved{}, fmt.Errorf("networksetup -getproxybypassdomains: %w", err)
		}
		sv.Bypass = parseBypass(string(out))
		saved.Services = append(saved.Services, sv)
	}
	return saved, nil
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
// goes on past a failure, and returns them all.
func Restore(saved Saved) error {
	list, err := services()
	if err != nil {
		return err
	}
	byName := map[string]Service{}
	for _, sv := range saved.Services {
		byName[sv.Name] = sv
	}
	var errs []error
	for _, s := range list {
		sv, ok := byName[s]
		if !ok {
			for _, kind := range kinds {
				errs = append(errs, run("-set"+kind+"state", s, "off"))
			}
			continue
		}
		for i, e := range sv.endpoints() {
			if e.Host != "" {
				errs = append(errs, run("-set"+kinds[i], s, e.Host, strconv.Itoa(e.Port)))
			} else {
				// networksetup has no word for no server: an empty one is
				// the nearest. Should it refuse, the state below still
				// turns the proxy off.
				_ = run("-set"+kinds[i], s, "", "0")
			}
			state := "off"
			if e.On && e.Host != "" {
				state = "on"
			}
			errs = append(errs, run("-set"+kinds[i]+"state", s, state))
		}
		domains := sv.Bypass
		if len(domains) == 0 {
			domains = []string{"Empty"}
		}
		errs = append(errs, run(append([]string{"-setproxybypassdomains", s}, domains...)...))
	}
	return errors.Join(errs...)
}
