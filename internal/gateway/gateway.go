// Package gateway makes this Mac the gateway of its LAN: other devices
// point their router and DNS at it, the kernel forwards their packets,
// TUN's routes hand them to the core, and the core's resolver answers on
// port 53.
//
// What the core does (TUN, the resolver) is the core package's; this one
// keeps the system around it: IPv4 forwarding, which it turns on and puts
// back as it found it; keeping the Mac awake; and what the interface shows
// about the network. Every system call goes through System, which tests
// replace.
package gateway

import (
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/tun"
)

// System is what the gateway asks of the operating system.
type System interface {
	// Forwarding reads whether the kernel forwards IPv4 packets.
	Forwarding() (bool, error)
	// SetForwarding turns IPv4 forwarding on or off; it needs Ready.
	SetForwarding(on bool) error
	// Ready says whether SetForwarding can work without asking anyone.
	Ready() bool
	// Prepare makes SetForwarding work from now on, asking for an
	// administrator's authorisation once; the steps given run as root
	// under the same authorisation, before it. prompt is the dialog's.
	Prepare(prompt string, before ...tun.Step) error
	// KeepAwake stops the system from sleeping until stop is called.
	KeepAwake() (stop func(), err error)
	// Network is what the Mac's LAN looks like now.
	Network() Network
	// AnswersDNS says whether a resolver answers on addr ("ip:port").
	AnswersDNS(addr string) bool
}

// Network is the Mac on its LAN, as the guide and the tips need it.
type Network struct {
	// IP is the Mac's IPv4 address on the LAN: what other devices set
	// as their gateway and DNS. "" when it is on none.
	IP string `json:"ip"`
	// Iface is the interface that has it, as en0; Router the LAN's router.
	Iface  string `json:"iface"`
	Router string `json:"router"`
	// WiFi: the interface is Wi-Fi, not a cable.
	WiFi bool `json:"wifi"`
	// DHCP: the address was leased, so it may change.
	DHCP bool `json:"dhcp"`
	// Firewall: the macOS firewall is on, which may block port 53.
	Firewall bool `json:"firewall"`
}

// Gateway turns forwarding on and off, and remembers what it was before
// in a file: after a crash, the next start can still put it back.
type Gateway struct {
	sys  System
	path string

	mu    sync.Mutex
	awake func() // stops keeping the Mac awake; nil when not
}

// New returns a gateway that keeps the forwarding it found in the JSON
// file at path while it is on.
func New(sys System, path string) *Gateway { return &Gateway{sys: sys, path: path} }

// System is the gateway's.
func (g *Gateway) System() System { return g.sys }

// before is the file's content.
type before struct {
	Forwarding bool      `json:"forwarding"`
	Since      time.Time `json:"since"`
}

// Enable turns forwarding on, first noting what it was, and keeps the Mac
// awake. Calling it again while on only mends what went off.
func (g *Gateway) Enable() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	cur, err := g.sys.Forwarding()
	if err != nil {
		return err
	}
	if _, ok := g.saved(); !ok {
		b, _ := json.Marshal(before{Forwarding: cur, Since: time.Now()})
		if err := os.WriteFile(g.path, b, 0o600); err != nil {
			return err
		}
	}
	if !cur {
		if !g.sys.Ready() {
			return ErrNotReady
		}
		if err := g.sys.SetForwarding(true); err != nil {
			return err
		}
	}
	if g.awake == nil {
		stop, err := g.sys.KeepAwake()
		if err != nil {
			return err
		}
		g.awake = stop
	}
	return nil
}

// Disable puts forwarding back as Enable found it and lets the Mac sleep.
// When Enable never ran (no file), forwarding is not Moho's and stays.
func (g *Gateway) Disable() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.awake != nil {
		g.awake()
		g.awake = nil
	}
	b, ok := g.saved()
	if !ok {
		return nil
	}
	if !b.Forwarding {
		cur, err := g.sys.Forwarding()
		if err != nil {
			return err
		}
		if cur {
			if !g.sys.Ready() {
				return ErrNotReady
			}
			if err := g.sys.SetForwarding(false); err != nil {
				return err
			}
		}
	}
	if err := os.Remove(g.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Pending says whether forwarding was turned on and not put back yet.
func (g *Gateway) Pending() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, ok := g.saved()
	return ok
}

// Awake says whether the gateway keeps the Mac awake now.
func (g *Gateway) Awake() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.awake != nil
}

func (g *Gateway) saved() (before, bool) {
	var b before
	data, err := os.ReadFile(g.path)
	if err != nil || json.Unmarshal(data, &b) != nil {
		return b, false
	}
	return b, true
}

// ErrNotReady is returned when forwarding must change and the one-time
// authorisation has not been given.
var ErrNotReady error = notReady{}

type notReady struct{}

func (notReady) Error() string {
	return i18n.T("网关模式需要先授权一次", "Gateway mode needs a one-time authorization first")
}

// Devices counts the clients among the source addresses: those that are
// not this Mac's own (locals) and not loopback.
func Devices(sources []string, locals []netip.Addr) int {
	seen := map[netip.Addr]bool{}
	for _, s := range sources {
		a, err := netip.ParseAddr(s)
		if err != nil {
			continue
		}
		a = a.Unmap()
		if a.IsLoopback() || a.IsUnspecified() || isLocal(a, locals) {
			continue
		}
		seen[a] = true
	}
	return len(seen)
}

func isLocal(a netip.Addr, locals []netip.Addr) bool {
	for _, l := range locals {
		if l == a {
			return true
		}
	}
	return false
}

// LocalAddrs are this Mac's addresses, on every interface (TUN's too: the
// Mac's own connections through TUN come from it).
func LocalAddrs() []netip.Addr {
	addrs, _ := net.InterfaceAddrs()
	out := make([]netip.Addr, 0, len(addrs))
	for _, ad := range addrs {
		if n, ok := ad.(*net.IPNet); ok {
			if a, ok := netip.AddrFromSlice(n.IP); ok {
				out = append(out, a.Unmap())
			}
		}
	}
	return out
}
