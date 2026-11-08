package ping

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

var pingTime = regexp.MustCompile(`time=([0-9.]+) ms`)

// ICMP times one echo to an IPv4 address with the system's ping, which
// needs no privileges. iface, if set, is the interface to leave through;
// a TUN would otherwise answer in the server's place.
func ICMP(ctx context.Context, ip string, iface string, timeout time.Duration) (int, error) {
	if a := net.ParseIP(ip); a == nil || a.To4() == nil {
		return 0, errors.New("ping: not an IPv4 address")
	}
	args := []string{"-n", "-c", "1", "-t", strconv.Itoa(max(1, int(timeout/time.Second)))}
	if iface != "" {
		args = append(args, "-b", iface)
	}
	out, err := exec.CommandContext(ctx, "/sbin/ping", append(args, ip)...).Output()
	if err != nil {
		return 0, err
	}
	return parseTime(out)
}

func parseTime(out []byte) (int, error) {
	m := pingTime.FindSubmatch(out)
	if m == nil {
		return 0, errors.New("ping: no reply")
	}
	v, err := strconv.ParseFloat(string(m[1]), 64)
	if err != nil {
		return 0, err
	}
	return millis(time.Duration(v * float64(time.Millisecond))), nil
}
