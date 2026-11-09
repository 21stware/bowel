package devnet

import (
	"encoding/json"
	"net/netip"
	"slices"
	"strings"
)

// TailscaleOutbound is the name of the direct outbound bound to Tailscale.
const TailscaleOutbound = "Tailscale-net"

// The bounds on a very large tailnet: machines named in hosts and rules,
// and those of them in the system proxy's bypass list, which is short.
const (
	maxPeers      = 256
	maxShortNames = 64
)

// Direct is the core's own direct target.
const Direct = "DIRECT"

// Options are the user's choices.
type Options struct {
	Disabled []string // scenario ids
	Extra    []string // domain suffixes and networks
	// Proxied are the user's rules that send something to a proxy, as the
	// core writes them. What they name has to reach the core.
	Proxied []string
}

// Rule is one rule without its place in the list.
type Rule struct {
	Text   string `json:"text"` // the core's, without the target
	Target string `json:"target"`
}

// Outbound is a direct connection that leaves by one interface.
type Outbound struct {
	Name  string `json:"name"`
	Iface string `json:"iface"`
}

// Policy sends a domain's questions to a server.
type Policy struct {
	Pattern string `json:"pattern"`
	Server  string `json:"server"`
}

// Overlay is what the core's config and the system proxy take from Compile.
// Each list is what to add: the user's own entries win over them.
type Overlay struct {
	Bypass       []string            `json:"bypass"`
	NoProxy      []string            `json:"noProxy"`
	FakeIPFilter []string            `json:"fakeIpFilter"`
	Policy       []Policy            `json:"policy,omitempty"`
	SystemDNS    []string            `json:"systemDns,omitempty"` // patterns the system's own DNS answers
	Hosts        map[string][]string `json:"hosts,omitempty"`
	Rules        []Rule              `json:"rules"`
	Outbounds    []Outbound          `json:"outbounds,omitempty"`
	TunExclude   []string            `json:"tunExclude"`
	SniffSkip    []string            `json:"sniffSkip"`

	domains  []string
	prefixes []netip.Prefix
}

// Empty says nothing is to be added.
func (o Overlay) Empty() bool { return len(o.Rules) == 0 }

// Fingerprint changes when the config made from o would.
func (o Overlay) Fingerprint() string {
	b, _ := json.Marshal(o)
	return string(b)
}

// MergeBypass is user with o's entries after it.
func (o Overlay) MergeBypass(user []string) []string {
	return merge(user, o.Bypass)
}

// Merge is first, then those of more it lacks, keeping the order.
func Merge(first, more []string) []string { return merge(first, more) }

func merge(first, more []string) []string {
	out := slices.Clone(first)
	seen := map[string]bool{}
	for _, s := range out {
		seen[s] = true
	}
	for _, s := range more {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// Covers says whether a rule of the core's is about something o handles:
// a user's more specific rule of that kind goes before o's own.
func (o Overlay) Covers(text string) bool {
	kind, rest, _ := strings.Cut(text, ",")
	value, _, _ := strings.Cut(rest, ",")
	switch kind {
	case "DOMAIN", "DOMAIN-SUFFIX":
		value = strings.ToLower(value)
		for _, d := range o.domains {
			if value == d || strings.HasSuffix(value, "."+d) {
				return true
			}
		}
	case "IP-CIDR", "IP-CIDR6":
		p, err := netip.ParsePrefix(value)
		if err != nil {
			return false
		}
		for _, c := range o.prefixes {
			if c.Bits() <= p.Bits() && c.Contains(p.Addr()) {
				return true
			}
		}
	}
	return false
}

// Keeps says whether a connection the core matched by payload, leaving by
// chains, goes the same way in every mode.
func (o Overlay) Keeps(payload string, chains []string) bool {
	for _, b := range o.Outbounds {
		if slices.Contains(chains, b.Name) {
			return true
		}
	}
	if payload == "" {
		return false
	}
	for _, r := range o.Rules {
		_, rest, _ := strings.Cut(r.Text, ",")
		value, _, _ := strings.Cut(rest, ",")
		if value == payload {
			return true
		}
	}
	return false
}

// Match is the rule of o that host, a name or an address, comes under.
func (o Overlay) Match(host string) (Rule, bool) {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	ip, ipErr := netip.ParseAddr(host)
	for _, r := range o.Rules {
		kind, rest, _ := strings.Cut(r.Text, ",")
		value, _, _ := strings.Cut(rest, ",")
		switch kind {
		case "DOMAIN":
			if host == value {
				return r, true
			}
		case "DOMAIN-SUFFIX":
			if host == value || strings.HasSuffix(host, "."+value) {
				return r, true
			}
		case "IP-CIDR", "IP-CIDR6":
			if p, err := netip.ParsePrefix(value); ipErr == nil && err == nil && p.Contains(ip) {
				return r, true
			}
		}
	}
	return Rule{}, false
}

// builder gathers entries, the first of any name winning.
type builder struct {
	o         Overlay
	domains   map[string]bool
	prefixes  map[netip.Prefix]bool
	domainRs  []Rule
	exactRs   []Rule
	cidrRs    []cidrRule
	shortName int
	// shortToCore keeps machines' short names out of the bypass list.
	shortToCore bool
	// What the user's rules want from the core.
	wantDomains  []string
	wantPrefixes []netip.Prefix
}

type cidrRule struct {
	p      netip.Prefix
	target string
}

// Compile makes the overlay of env and the catalog.
func Compile(opts Options, env Env) Overlay {
	b := &builder{domains: map[string]bool{}, prefixes: map[netip.Prefix]bool{}, o: Overlay{Hosts: map[string][]string{}}}
	off := func(id string) bool { return slices.Contains(opts.Disabled, id) }
	b.want(opts.Proxied)

	// What is measured first: a network it names goes by its interface,
	// not by the catalog's DIRECT.
	if !off(Mesh) {
		b.tailscale(env)
	}
	if !off(VPN) {
		b.tunnels(env)
		b.resolvers(env)
	}
	for _, s := range scenarios {
		if off(s.ID) {
			continue
		}
		for _, l := range s.Literal {
			b.o.Bypass = append(b.o.Bypass, l)
			b.o.NoProxy = append(b.o.NoProxy, l)
		}
		for _, d := range s.Domains {
			if b.domain(d, Direct) && s.Unlisted {
				b.o.SystemDNS = append(b.o.SystemDNS, "+."+d)
			}
		}
		for _, c := range s.CIDRs {
			b.cidr(netip.MustParsePrefix(c), Direct, slices.Contains(s.Outside, c))
		}
	}
	for _, e := range opts.Extra {
		if p, d, ok := parseExtra(e); ok {
			if d != "" {
				b.domain(d, Direct)
			} else {
				b.cidr(p, Direct, true)
			}
		}
	}
	return b.finish()
}

func (b *builder) tailscale(env Env) {
	ts := env.Tailscale
	if ts.Iface == "" {
		return
	}
	b.o.Outbounds = append(b.o.Outbounds, Outbound{Name: TailscaleOutbound, Iface: ts.Iface})
	names := []string{ts.Suffix, "ts.net", "beta.tailscale.net"}
	if ts.Shadowed {
		// The system's own route would lose this traffic; the core's
		// outbound, bound to the interface, does not.
		b.wantDomains = append(b.wantDomains, names...)
		b.wantPrefixes = append(b.wantPrefixes, cgnat)
		b.shortToCore = true
	}
	for _, d := range names {
		if b.domain(d, TailscaleOutbound) && ts.MagicDNS {
			b.o.Policy = append(b.o.Policy, Policy{"+." + cleanDomain(d), magicResolver + "#" + TailscaleOutbound})
		}
	}
	for _, c := range meshCIDRs {
		b.cidr(netip.MustParsePrefix(c), TailscaleOutbound, true)
	}
	for _, p := range ts.Subnets {
		b.cidr(p, TailscaleOutbound, false)
	}
	for _, p := range ts.Peers[:min(len(ts.Peers), maxPeers)] {
		ips := hostIPs(p.IPs)
		b.o.Hosts[strings.ToLower(p.Name)] = ips
		// MagicDNS answers with both families and the core tries both, and
		// it knows the machines that joined since Detect last looked.
		if p.FQDN != "" && !ts.MagicDNS {
			b.o.Hosts[strings.ToLower(p.FQDN)] = ips
		}
		b.short(p.Name, TailscaleOutbound)
	}
}

// hostIPs is what a hosts entry may hold. The core picks one of an entry's
// addresses at random and tries no other, so a machine with both families
// would fail half the time for a service that listens on IPv4 only.
func hostIPs(addrs []netip.Addr) []string {
	var v4, v6 []string
	for _, ip := range addrs {
		if ip.Is4() {
			v4 = append(v4, ip.String())
		} else {
			v6 = append(v6, ip.String())
		}
	}
	if len(v4) > 0 {
		return v4
	}
	return v6
}

func tunnelOutbound(iface string) string { return "VPN-" + iface }

func (b *builder) tunnels(env Env) {
	for _, t := range env.VPNs {
		b.o.Outbounds = append(b.o.Outbounds, Outbound{Name: tunnelOutbound(t.Iface), Iface: t.Iface})
		for _, p := range t.Prefixes {
			b.cidr(p, tunnelOutbound(t.Iface), false)
		}
	}
}

// outboundFor is what reaches a resolver's server: the interface the
// system sends it to, when that is a tunnel we have an outbound for.
func (b *builder) outboundFor(env Env, r Resolver) string {
	if env.Tailscale.Iface != "" && (r.Iface == env.Tailscale.Iface || slices.Contains(r.Servers, magicResolver)) {
		return TailscaleOutbound
	}
	if r.Iface != "" && slices.ContainsFunc(b.o.Outbounds, func(o Outbound) bool { return o.Iface == r.Iface }) {
		return tunnelOutbound(r.Iface)
	}
	return ""
}

func (b *builder) resolvers(env Env) {
	for _, r := range env.Resolvers {
		out := b.outboundFor(env, r)
		target := Direct
		if out != "" {
			target = out
		}
		if !b.domain(r.Domain, target) {
			continue
		}
		for _, s := range r.Servers {
			server := s
			if out != "" {
				server += "#" + out
			}
			b.o.Policy = append(b.o.Policy, Policy{"+." + r.Domain, server})
		}
	}
}

// want notes what the user's rules send to a proxy.
func (b *builder) want(rules []string) {
	for _, text := range rules {
		kind, rest, _ := strings.Cut(text, ",")
		value, _, _ := strings.Cut(rest, ",")
		switch kind {
		case "DOMAIN", "DOMAIN-SUFFIX":
			if d := cleanDomain(value); d != "" {
				b.wantDomains = append(b.wantDomains, d)
			}
		case "IP-CIDR", "IP-CIDR6":
			if p, err := netip.ParsePrefix(value); err == nil {
				b.wantPrefixes = append(b.wantPrefixes, p.Masked())
			}
		}
	}
}

// wantsUnder says whether a rule of the user's sends d, or a name under d,
// to a proxy: then the system must not keep d from the core.
func (b *builder) wantsUnder(d string) bool {
	return slices.ContainsFunc(b.wantDomains, func(w string) bool { return w == d || strings.HasSuffix(w, "."+d) })
}

// domain adds a suffix and says whether it was new.
func (b *builder) domain(d, target string) bool {
	d = cleanDomain(d)
	if d == "" || b.domains[d] {
		return false
	}
	b.domains[d] = true
	b.o.domains = append(b.o.domains, d)
	b.domainRs = append(b.domainRs, Rule{"DOMAIN-SUFFIX," + d, target})
	if !b.wantsUnder(d) {
		b.o.Bypass = append(b.o.Bypass, "*."+d)
		b.o.NoProxy = append(b.o.NoProxy, "."+d)
	}
	b.o.FakeIPFilter = append(b.o.FakeIPFilter, "+."+d)
	return true
}

// short adds a single-label name of a machine.
func (b *builder) short(name, target string) {
	name = cleanDomain(name)
	if name == "" || strings.Contains(name, ".") || b.domains[name] {
		return
	}
	b.domains[name] = true
	b.o.domains = append(b.o.domains, name)
	b.exactRs = append(b.exactRs, Rule{"DOMAIN," + name, target})
	b.o.FakeIPFilter = append(b.o.FakeIPFilter, name)
	if !b.shortToCore && b.shortName < maxShortNames {
		b.shortName++
		b.o.Bypass = append(b.o.Bypass, name)
		b.o.NoProxy = append(b.o.NoProxy, name)
	}
}

func (b *builder) cidr(p netip.Prefix, target string, outside bool) {
	p = p.Masked()
	if b.prefixes[p] {
		return
	}
	b.prefixes[p] = true
	b.o.prefixes = append(b.o.prefixes, p)
	b.cidrRs = append(b.cidrRs, cidrRule{p, target})
	if !slices.ContainsFunc(b.wantPrefixes, p.Overlaps) {
		b.o.NoProxy = append(b.o.NoProxy, p.String())
		if p.Addr().Is4() {
			b.o.Bypass = append(b.o.Bypass, p.String())
		}
		if outside {
			b.o.TunExclude = append(b.o.TunExclude, p.String())
		}
	}
	if !p.Addr().IsLoopback() {
		b.o.SniffSkip = append(b.o.SniffSkip, p.String())
	}
}

func (b *builder) finish() Overlay {
	o := b.o
	// The longest first, so that a narrower one is not hidden by a wider.
	slices.SortStableFunc(b.domainRs, func(x, y Rule) int { return len(y.Text) - len(x.Text) })
	slices.SortStableFunc(b.cidrRs, func(x, y cidrRule) int { return y.p.Bits() - x.p.Bits() })
	o.Rules = append(o.Rules, b.exactRs...)
	o.Rules = append(o.Rules, b.domainRs...)
	for _, r := range b.cidrRs {
		kind := "IP-CIDR,"
		if r.p.Addr().Is6() {
			kind = "IP-CIDR6,"
		}
		o.Rules = append(o.Rules, Rule{kind + r.p.String() + ",no-resolve", r.target})
	}
	o.Bypass, o.NoProxy = merge(nil, o.Bypass), merge(nil, o.NoProxy)
	return o
}
