package main

// The devices that use the gateway: who they are, whether they are there
// now, and when each came and went. What is known of a device comes from
// three places: the connections it has through the core (its address),
// the lease bowel gave it (its name, and that it came by itself), and the
// system's ARP table (its hardware address).

import (
	"encoding/json"
	"net/netip"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/oboo/moho/internal/dhcp"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/monitor"
)

const (
	// deviceGone is how long a device may be silent before it counts as
	// offline: longer than half a lease, at which a device that only sits
	// there renews.
	deviceGone   = dhcp.DefaultLease/2 + 2*time.Minute
	deviceEvents = 30                  // kept for each device
	deviceKeep   = 30 * 24 * time.Hour // a device not seen for this long is forgotten
)

type deviceRec struct {
	IP     string        `json:"ip"`
	MAC    string        `json:"mac,omitempty"`
	Name   string        `json:"name,omitempty"`
	Via    string        `json:"via"`
	Seen   time.Time     `json:"seen"`
	Online bool          `json:"online"`
	Events []DeviceEvent `json:"events"`
}

// devices is the record, kept in a file so that the history outlasts a
// restart.
type devices struct {
	path string

	mu     sync.Mutex
	byIP   map[string]*deviceRec
	loaded bool
	dirty  bool
	saved  time.Time
}

func (d *devices) load() {
	if d.loaded {
		return
	}
	d.loaded, d.byIP = true, map[string]*deviceRec{}
	b, err := os.ReadFile(d.path)
	if err != nil {
		return
	}
	var list []*deviceRec
	if json.Unmarshal(b, &list) == nil {
		for _, r := range list {
			d.byIP[r.IP] = r
		}
	}
}

// update takes what is true at now: the addresses with open connections
// (sources, this computer's own left out by the caller), the leases, and
// the neighbours' hardware addresses.
func (d *devices) update(now time.Time, sources []netip.Addr, leases []dhcp.Lease, neigh map[netip.Addr]string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.load()
	rec := func(ip netip.Addr) *deviceRec {
		r := d.byIP[ip.String()]
		if r == nil {
			r = &deviceRec{IP: ip.String(), Via: "manual"}
			d.byIP[r.IP] = r
			d.dirty = true
		}
		return r
	}
	leased := map[string]bool{}
	for _, l := range leases {
		if !l.Active(now) {
			continue
		}
		leased[l.IP.String()] = true
		r := rec(l.IP)
		if r.MAC != "" && r.MAC != l.MAC { // the address went to another device
			r.Name, r.Events, r.Online = "", nil, false
		}
		if r.Name != l.Name || r.MAC != l.MAC || r.Via != "auto" {
			r.Name, r.MAC, r.Via, d.dirty = l.Name, l.MAC, "auto", true
		}
		if l.Seen.After(r.Seen) {
			r.Seen = l.Seen
		}
	}
	for _, ip := range sources {
		r := rec(ip)
		r.Seen = now
		if !leased[r.IP] && r.Via != "manual" {
			r.Via, d.dirty = "manual", true
		}
	}
	for ip, r := range d.byIP {
		if a, err := netip.ParseAddr(ip); err == nil && r.MAC == "" && neigh[a] != "" {
			r.MAC, d.dirty = neigh[a], true
		}
		on := !r.Seen.IsZero() && now.Sub(r.Seen) < deviceGone
		if on != r.Online {
			ev := DeviceEvent{At: now, Kind: "off"}
			if on {
				ev.Kind = "on"
				if len(r.Events) == 0 {
					ev.Note = "first"
				}
			} else {
				ev.At = r.Seen // it left when it was last heard, not when that was noticed
			}
			r.Online, d.dirty = on, true
			r.Events = append([]DeviceEvent{ev}, r.Events...)
			if len(r.Events) > deviceEvents {
				r.Events = r.Events[:deviceEvents]
			}
		}
		if !on && now.Sub(r.Seen) > deviceKeep {
			delete(d.byIP, ip)
			d.dirty = true
		}
	}
	// Seen moves all the time: the file follows a change of substance, and
	// otherwise every few minutes.
	if d.dirty || now.Sub(d.saved) > 5*time.Minute {
		d.save(now)
	}
}

func (d *devices) save(now time.Time) {
	list := make([]*deviceRec, 0, len(d.byIP))
	for _, r := range d.byIP {
		list = append(list, r)
	}
	slices.SortFunc(list, func(a, b *deviceRec) int { return compareIP(a.IP, b.IP) })
	if b, err := json.MarshalIndent(list, "", "  "); err == nil && d.path != "" {
		tmp := d.path + ".tmp"
		if os.WriteFile(tmp, b, 0o600) == nil {
			_ = os.Rename(tmp, d.path)
		}
	}
	d.dirty, d.saved = false, now
}

func compareIP(a, b string) int {
	x, _ := netip.ParseAddr(a)
	y, _ := netip.ParseAddr(b)
	return x.Compare(y)
}

// view is the devices for the interface: those online first, each with
// what its connections carry.
func (d *devices) view(conns []monitor.Conn) []DeviceView {
	type sum struct {
		n        int
		down, up int64
	}
	by := map[string]*sum{}
	for _, c := range conns {
		s := by[c.Source]
		if s == nil {
			s = &sum{}
			by[c.Source] = s
		}
		if c.Status == "active" {
			s.n++
		}
		s.down, s.up = s.down+c.Down, s.up+c.Up
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.load()
	out := make([]DeviceView, 0, len(d.byIP))
	for _, r := range d.byIP {
		v := DeviceView{IP: r.IP, MAC: r.MAC, Name: r.Name, Via: r.Via, Online: r.Online, Seen: r.Seen, Events: slices.Clone(r.Events)}
		if v.Events == nil {
			v.Events = []DeviceEvent{}
		}
		if s := by[r.IP]; s != nil {
			v.Conns, v.Down, v.Up = s.n, s.down, s.up
		}
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b DeviceView) int {
		if a.Online != b.Online {
			if a.Online {
				return -1
			}
			return 1
		}
		return compareIP(a.IP, b.IP)
	})
	return out
}

// deviceTitle is what a device is called where it has no name.
func deviceTitle(v DeviceView) string {
	if v.Name != "" {
		return v.Name
	}
	return i18n.T("未命名设备", "Unnamed device")
}
