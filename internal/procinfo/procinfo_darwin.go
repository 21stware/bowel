package procinfo

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// The list is a header (struct xinpgen), then for each socket a run of
// records, each starting with its length and kind (xnu bsd/netinet/
// in_pcblist.c, get_pcblist_n): the protocol block, the socket, two
// buffers, statistics and, for TCP, the TCP block. Records are walked by
// their own lengths, so layouts that grow do not break the walk, as fixed
// strides would.
const (
	kindSocket = 0x001 // XSO_SOCKET: struct xsocket_n
	kindInpcb  = 0x010 // XSO_INPCB: struct xinpcb_n

	// offsets in xinpcb_n
	inpLport = 18 // inp_lport, network order
	inpVflag = 44 // inp_vflag: 0x1 IPv4, 0x2 IPv6
	inpLaddr = 64 // inp_dependladdr: in6_addr, or the IPv4 address at +12

	// offset in xsocket_n
	soLastPid = 68 // so_last_pid
)

func list(network string) (map[netip.AddrPort]int32, error) {
	name := "net.inet.tcp.pcblist_n"
	if network == "udp" {
		name = "net.inet.udp.pcblist_n"
	}
	b, err := unix.SysctlRaw(name)
	if err != nil {
		return nil, err
	}
	return parse(b)
}

func parse(b []byte) (map[netip.AddrPort]int32, error) {
	le := binary.LittleEndian
	if len(b) < 8 {
		return nil, errors.New("procinfo: short list")
	}
	out := map[netip.AddrPort]int32{}
	i := rup8(int(le.Uint32(b)))
	var addr netip.AddrPort // of the last protocol block, until its socket
	for i+8 <= len(b) {
		n, kind := int(le.Uint32(b[i:])), le.Uint32(b[i+4:])
		if n < 8 || i+n > len(b) {
			break // the trailing xinpgen, or garbage
		}
		r := b[i : i+n]
		switch kind {
		case kindInpcb:
			addr = netip.AddrPort{}
			if n < inpLaddr+16 {
				break
			}
			port := binary.BigEndian.Uint16(r[inpLport:])
			switch v := r[inpVflag]; {
			case v&0x1 != 0:
				a, _ := netip.AddrFromSlice(r[inpLaddr+12 : inpLaddr+16])
				addr = netip.AddrPortFrom(a, port)
			case v&0x2 != 0:
				a, _ := netip.AddrFromSlice(r[inpLaddr : inpLaddr+16])
				addr = netip.AddrPortFrom(a.Unmap(), port)
			}
		case kindSocket:
			if addr.IsValid() && n >= soLastPid+4 {
				if pid := int32(le.Uint32(r[soLastPid:])); pid > 0 {
					out[addr] = pid
				}
			}
			addr = netip.AddrPort{}
		}
		i += rup8(n)
	}
	return out, nil
}

func rup8(n int) int { return (n + 7) &^ 7 }

// running asks the kernel for this user's processes (kern.proc.uid), then
// each one's path.
func running() []string {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.uid", os.Getuid())
	if err != nil {
		return nil
	}
	var out []string
	for _, p := range procs {
		if path := pidPath(p.Proc.P_pid); path != "" {
			out = append(out, path)
		}
	}
	return out
}

// pidPath asks proc_pidpath (proc_info with PROC_PIDPATHINFO).
func pidPath(pid int32) string {
	const (
		callPidInfo  = 2
		pidPathInfo  = 11
		pathInfoSize = 4 * 1024 // PROC_PIDPATHINFO_MAXSIZE
	)
	buf := make([]byte, pathInfoSize)
	_, _, errno := syscall.Syscall6(syscall.SYS_PROC_INFO, callPidInfo, uintptr(pid), pidPathInfo, 0,
		uintptr(unsafe.Pointer(&buf[0])), pathInfoSize)
	if errno != 0 {
		return ""
	}
	return unix.ByteSliceToString(buf)
}
