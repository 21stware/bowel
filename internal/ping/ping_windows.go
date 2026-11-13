package ping

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/oboo/moho/internal/nowindow"
)

// ICMP times one echo to an IPv4 address with Windows' IP Helper API, in
// this process: no second process to start and no localized output to read.
// iface, if set, is the adapter to leave through, given to the API as the
// echo's source address; a TUN would otherwise answer in the server's place.
// Windows' ping is the fallback, for a system where the API cannot be used.
func ICMP(ctx context.Context, ip string, iface string, timeout time.Duration) (int, error) {
	a := net.ParseIP(ip)
	if a == nil || a.To4() == nil {
		return 0, errors.New("ping: not an IPv4 address")
	}
	src := ""
	if iface != "" {
		if src = ifaceIPv4(iface); src == "" {
			return 0, errors.New("ping: the adapter has no IPv4 address")
		}
	}
	ms, err := echo(ctx, a.To4(), src, timeout)
	if errors.Is(err, errNoIPHelper) {
		return pingExe(ctx, ip, src, timeout)
	}
	return ms, err
}

// pingExe times an echo with Windows' ping, which needs no privileges.
func pingExe(ctx context.Context, ip, src string, timeout time.Duration) (int, error) {
	sys, err := windows.GetSystemDirectory()
	if err != nil {
		return 0, err
	}
	cmd := nowindow.CommandContext(ctx, filepath.Join(sys, "PING.EXE"), pingArgs(ip, src, timeout)...)
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

// errNoIPHelper says the IP Helper API cannot be used here, rather than that
// the echo went unanswered: only then is Windows' ping worth starting.
var errNoIPHelper = errors.New("ping: IP Helper is not available")

var (
	iphlpapi            = windows.NewLazySystemDLL("iphlpapi.dll")
	icmpCreateFile      = iphlpapi.NewProc("IcmpCreateFile")
	icmpCloseHandle     = iphlpapi.NewProc("IcmpCloseHandle")
	icmpSendEcho2ExProc = iphlpapi.NewProc("IcmpSendEcho2Ex")
	echoPayload         = [32]byte{'a'}
	echoLookup          sync.Once
	echoUnavailable     error
)

// echo is one echo through IcmpSendEcho2Ex. src, if set, is the source
// address: the adapter a TUN would otherwise answer from.
func echo(ctx context.Context, ip net.IP, src string, timeout time.Duration) (int, error) {
	echoLookup.Do(func() {
		for _, p := range []*windows.LazyProc{icmpCreateFile, icmpCloseHandle, icmpSendEcho2ExProc} {
			if err := p.Find(); err != nil {
				echoUnavailable = errNoIPHelper
				return
			}
		}
	})
	if echoUnavailable != nil {
		return 0, echoUnavailable
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if d, ok := ctx.Deadline(); ok {
		if left := time.Until(d); left < timeout {
			timeout = left
		}
	}
	if timeout <= 0 {
		return 0, context.DeadlineExceeded
	}
	h, _, err := icmpCreateFile.Call()
	if h == 0 || h == uintptr(windows.InvalidHandle) {
		return 0, fmt.Errorf("%w: %v", errNoIPHelper, err)
	}
	defer icmpCloseHandle.Call(h)

	// ICMP_ECHO_REPLY, then the data sent, then the room the documentation
	// asks for: the eight bytes of the original datagram an ICMP error carries
	// and an IO_STATUS_BLOCK (sixteen bytes on 64-bit).
	reply := make([]byte, unsafe.Sizeof(icmpEchoReply{})+uintptr(len(echoPayload))+8+ioStatusBlock)
	n, _, e := icmpSendEcho2ExProc.Call(
		h, 0, 0, 0,
		uintptr(addr(net.ParseIP(src))), uintptr(addr(ip)),
		uintptr(unsafe.Pointer(&echoPayload[0])), uintptr(len(echoPayload)),
		0,
		uintptr(unsafe.Pointer(&reply[0])), uintptr(len(reply)),
		uintptr(max(1, int(timeout/time.Millisecond))),
	)
	if n == 0 {
		return 0, echoError(e)
	}
	r := (*icmpEchoReply)(unsafe.Pointer(&reply[0]))
	// Any status but zero is the echo not coming back: a host or network
	// unreachable reads the same way as a timeout, as it did in ping's output.
	if r.Status != 0 {
		return 0, errors.New("ping: no reply")
	}
	return millis(time.Duration(r.RoundTripTime) * time.Millisecond), nil
}

// ioStatusBlock is an IO_STATUS_BLOCK, the room Windows may use after the
// reply; sixteen bytes on 64-bit, and more than the least on 32-bit.
const ioStatusBlock = 16

// ipReqTimedOut is IP_REQ_TIMED_OUT: nothing came back before the timeout.
// Windows' own text for it is "Error due to lack of resources", which says
// nothing about a node that did not answer.
const ipReqTimedOut = 11010

// echoError says what a call that answered nothing means. A timeout is an
// answer, nobody replied, and only an API that cannot be used is worth
// starting a process for.
func echoError(err error) error {
	if errno, ok := err.(syscall.Errno); ok {
		switch errno {
		case ipReqTimedOut:
			return errors.New("ping: no reply")
		case windows.ERROR_NOT_SUPPORTED, windows.ERROR_CALL_NOT_IMPLEMENTED, windows.ERROR_INVALID_FUNCTION:
			return fmt.Errorf("%w: %v", errNoIPHelper, err)
		}
	}
	return fmt.Errorf("ping: %w", err)
}

// addr is an IPv4 address as the API takes it: IPAddr is in network byte
// order, which is a machine word read as it is stored. No address, or one
// that cannot be used, is 0: Windows chooses the source then.
func addr(a net.IP) uint32 {
	if b := a.To4(); b != nil {
		return binary.LittleEndian.Uint32(b)
	}
	return 0
}

// icmpEchoReply is Windows' ICMP_ECHO_REPLY, as the API fills it in.
type icmpEchoReply struct {
	Address       uint32
	Status        uint32
	RoundTripTime uint32
	DataSize      uint16
	Reserved      uint16
	Data          uintptr
	Options       ipOptionInformation
}

// ipOptionInformation is Windows' IP_OPTION_INFORMATION. OptionsData is a
// pointer, eight bytes on 64-bit and four on 32-bit; that is what makes the
// reply 40 bytes there rather than 32.
type ipOptionInformation struct {
	TTL, TOS, Flags, OptionsSize uint8
	OptionsData                  uintptr
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
