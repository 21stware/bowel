package sysdns

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Supported says whether this system's DNS servers are bowel's to set.
const Supported = true

// perCall bounds one networksetup: a stuck one must not hold up quitting
// or a change of settings.
const perCall = 5 * time.Second

func output(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, perCall)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = time.Second
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("%s %s: %s", name, args[0], firstLine(string(out), err))
	}
	return string(out), nil
}

func firstLine(out string, err error) string {
	if line, _, _ := strings.Cut(strings.TrimSpace(out), "\n"); line != "" {
		return line
	}
	return err.Error()
}

// Snapshot reads the DNS servers of every enabled network service. It
// changes nothing.
func Snapshot(ctx context.Context) (Saved, error) {
	out, err := output(ctx, "networksetup", "-listallnetworkservices")
	if err != nil {
		return Saved{}, err
	}
	devices := map[string]string{}
	if order, err := output(ctx, "networksetup", "-listnetworkserviceorder"); err == nil {
		devices = parseOrder(order)
	}
	var saved Saved
	for i, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		// The first line is a notice; disabled services start with '*'.
		if i == 0 || line == "" || strings.HasPrefix(line, "*") {
			continue
		}
		got, err := output(ctx, "networksetup", "-getdnsservers", line)
		if err != nil {
			return Saved{}, err
		}
		servers, err := parseGet(got)
		if err != nil {
			return Saved{}, err
		}
		saved.Services = append(saved.Services, Service{Name: line, Device: devices[line], Servers: servers})
	}
	return saved, nil
}

// Write sets the DNS servers of each service of steps. It goes on past a
// failure, and returns them all.
func Write(ctx context.Context, steps []Service) error {
	var errs []error
	for _, sv := range steps {
		_, err := output(ctx, "networksetup", setArgs(sv)...)
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// DHCP are the DNS servers the network hands out on iface, if it has a
// lease.
func DHCP(ctx context.Context, iface string) []string {
	if iface == "" {
		return nil
	}
	out, err := output(ctx, "/usr/sbin/ipconfig", "getpacket", iface)
	if err != nil {
		return nil
	}
	return parsePacket(out)
}
