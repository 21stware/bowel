package ping

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// ICMP times one echo to an IPv4 address with Windows' ping, which needs
// no privileges. iface, if set, is the adapter to leave through, given to
// ping as that adapter's own address; a TUN would otherwise answer in the
// server's place.
func ICMP(ctx context.Context, ip string, iface string, timeout time.Duration) (int, error) {
	if a := net.ParseIP(ip); a == nil || a.To4() == nil {
		return 0, errors.New("ping: not an IPv4 address")
	}
	src := ""
	if iface != "" {
		if src = ifaceIPv4(iface); src == "" {
			return 0, errors.New("ping: the adapter has no IPv4 address")
		}
	}
	sys, err := windows.GetSystemDirectory()
	if err != nil {
		return 0, err
	}
	cmd := exec.CommandContext(ctx, filepath.Join(sys, "PING.EXE"), pingArgs(ip, src, timeout)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	return parseTime(out)
}

func pingArgs(ip, src string, timeout time.Duration) []string {
	args := []string{"-n", "1", "-w", strconv.Itoa(max(1, int(timeout/time.Millisecond)))}
	if src != "" {
		args = append(args, "-S", src)
	}
	return append(args, ip)
}

// ifaceIPv4 is the adapter's first IPv4 address.
func ifaceIPv4(name string) string {
	ifc, err := net.InterfaceByName(name)
	if err != nil {
		return ""
	}
	addrs, _ := ifc.Addrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
			return n.IP.String()
		}
	}
	return ""
}
