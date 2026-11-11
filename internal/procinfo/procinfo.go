// Package procinfo names the process behind a local socket.
//
// mihomo does this itself. On macOS 27 the kernel's socket list
// (net.inet.tcp.pcblist_n) holds only the asker's own sockets unless the
// asker is signed under an identifier with a dot in it and its parent is
// not an app; the core is started detached (core.startCore) and named
// (core.BinName) so that it is both, and bowel, opened like any app, is
// both by itself. The monitor still asks here for the connections the
// core does not look up, those through a route group's own port, and for
// any it could not name: a core given by MOHO_CORE under another
// signature, or the copy TUN runs on that an earlier version made.
package procinfo

import (
	"net/netip"
	"sync"
	"time"
)

// Finder looks sockets up in a snapshot of the kernel's list, taken again
// when older than maxAge, so that the lookups of one poll share one read.
type Finder struct {
	mu     sync.Mutex
	maxAge time.Duration
	at     map[string]time.Time
	socks  map[string]map[netip.AddrPort]int32 // network -> local address -> pid
	paths  map[int32]string
}

// New returns a Finder whose snapshots live for 250 ms.
func New() *Finder {
	return &Finder{
		maxAge: 250 * time.Millisecond,
		at:     map[string]time.Time{},
		socks:  map[string]map[netip.AddrPort]int32{},
		paths:  map[int32]string{},
	}
}

// Find returns the executable path of the process that owns the local
// socket at src ("tcp" or "udp"), or "" if it can't tell.
func (f *Finder) Find(network string, src netip.AddrPort) string {
	if network != "tcp" && network != "udp" || !src.IsValid() {
		return ""
	}
	src = netip.AddrPortFrom(src.Addr().Unmap(), src.Port())
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	if now.Sub(f.at[network]) > f.maxAge {
		socks, err := list(network)
		if err != nil {
			return ""
		}
		f.socks[network], f.at[network] = socks, now
	}
	socks := f.socks[network]
	pid, ok := socks[src]
	if !ok && network == "udp" {
		// a UDP socket bound to the wildcard address
		pid, ok = socks[netip.AddrPortFrom(unspecified(src.Addr()), src.Port())]
	}
	if !ok || pid <= 0 {
		return ""
	}
	if p, ok := f.paths[pid]; ok {
		return p
	}
	p := pidPath(pid)
	if len(f.paths) > 2000 { // pids get reused; keep the cache small
		clear(f.paths)
	}
	f.paths[pid] = p
	return p
}

func unspecified(a netip.Addr) netip.Addr {
	if a.Is4() {
		return netip.IPv4Unspecified()
	}
	return netip.IPv6Unspecified()
}
