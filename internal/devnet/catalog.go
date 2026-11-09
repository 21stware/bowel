// Package devnet keeps what developers' machines already know how to reach
// away from the proxy: this Mac, the LAN, containers, Tailscale, VPNs. See
// docs/dev-network.md for the cases and why each is handled as it is.
//
// It has a catalog of what is true everywhere, reads what is true of this
// machine (Detect), and compiles the two into an Overlay that the core's
// config and the system proxy's bypass list are merged with. Nothing here is
// saved in the user's settings, so turning it off leaves no trace.
package devnet

import (
	"net/netip"
	"strings"

	"github.com/oboo/moho/internal/i18n"
)

// IDs of the scenarios that also take what Detect finds.
const (
	Mesh = "mesh"
	VPN  = "vpn"
)

// Scenario is one kind of traffic that has no business in a proxy.
type Scenario struct {
	ID             string
	zh, en         string
	descZh, descEn string
	// Domains are suffixes: the name and everything under it.
	Domains []string
	CIDRs   []string
	// Outside are the CIDRs that never enter TUN, so that the OS's own
	// routes and DNS keep working for them.
	Outside []string
	// Literal are extra entries for the system proxy's bypass list.
	Literal []string
	// Unlisted: these names exist only in the local network's own DNS, so
	// the core's public resolvers must not be the ones asked.
	Unlisted bool
}

func (s Scenario) Name() string { return i18n.T(s.zh, s.en) }
func (s Scenario) Desc() string { return i18n.T(s.descZh, s.descEn) }

var lanCIDRs = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "224.0.0.0/4", "fc00::/7", "fe80::/10", "ff00::/8"}

// meshCIDRs are the ranges of Tailscale, and of the overlays that took
// the same CGNAT block after it.
var meshCIDRs = []string{"100.64.0.0/10", "fd7a:115c:a1e0::/48"}

var scenarios = []Scenario{
	{
		ID: "loopback", zh: "本机回环", en: "This Mac",
		descZh: "localhost 和 127.0.0.1 上的开发服务", descEn: "Development servers on localhost and 127.0.0.1",
		Domains: []string{"localhost"},
		CIDRs:   []string{"127.0.0.0/8", "::1/128"},
		Literal: []string{"localhost", "127.0.0.1", "::1"},
	},
	{
		ID: "lan", zh: "局域网地址", en: "LAN addresses",
		descZh: "内网网段、链路本地和组播，这些流量不进 TUN", descEn: "Private ranges, link-local and multicast; none of it enters TUN",
		CIDRs:   lanCIDRs,
		Outside: lanCIDRs,
	},
	{
		ID: "lan-names", zh: "局域网域名", en: "LAN names",
		descZh: ".local、.lan、.home.arpa、.corp 这类内网后缀", descEn: "Internal suffixes such as .local, .lan, .home.arpa and .corp",
		Domains:  []string{"local", "lan", "home", "home.arpa", "localdomain", "intranet", "corp"},
		Unlisted: true,
	},
	{
		ID: "dev-tld", zh: "本地开发域名", en: "Local development names",
		descZh: ".test 这类保留域名，Valet、puma-dev 和 dnsmasq 常用", descEn: "Reserved names such as .test, as Valet, puma-dev and dnsmasq use",
		Domains:  []string{"test", "example", "invalid"},
		Unlisted: true,
	},
	{
		ID: "containers", zh: "容器和虚拟机", en: "Containers and VMs",
		descZh: "host.docker.internal、Kubernetes 的 .svc 和 OrbStack 的 .orb.local", descEn: "host.docker.internal, Kubernetes .svc and OrbStack .orb.local",
		Domains:  []string{"internal", "svc", "cluster", "docker"},
		Unlisted: true,
	},
	{
		ID: "rebind", zh: "指向本机的公共域名", en: "Public names for local addresses",
		descZh: "nip.io、sslip.io、lvh.me 这类解析到内网地址的服务", descEn: "Services such as nip.io, sslip.io and lvh.me that resolve to local addresses",
		Domains: []string{"nip.io", "sslip.io", "lvh.me", "localtest.me", "traefik.me", "vcap.me", "localho.st"},
	},
	{
		ID: Mesh, zh: "Tailscale 和组网", en: "Tailscale and mesh networks",
		descZh: "对端、MagicDNS、子网路由和短主机名，运行时自动识别", descEn: "Peers, MagicDNS, subnet routes and short names, detected as they change",
		Domains: []string{"ts.net", "beta.tailscale.net"},
		CIDRs:   meshCIDRs,
		Outside: meshCIDRs,
	},
	{
		ID: VPN, zh: "其他 VPN 和分流 DNS", en: "Other VPNs and split DNS",
		descZh: "读取本机的 VPN 路由和 /etc/resolver 这类分流配置", descEn: "Reads this Mac's VPN routes and split DNS, such as /etc/resolver",
	},
}

// Scenarios are all of them, in the order the settings window lists them.
func Scenarios() []Scenario { return scenarios }

// Known says whether id is a scenario's.
func Known(id string) bool {
	for _, s := range scenarios {
		if s.ID == id {
			return true
		}
	}
	return false
}

// Valid says whether entry can be added by the user: a domain suffix or a
// network.
func Valid(entry string) bool {
	_, _, ok := parseExtra(entry)
	return ok
}

// parseExtra reads one of the user's entries as a CIDR or a domain.
func parseExtra(entry string) (netip.Prefix, string, bool) {
	entry = strings.TrimSpace(entry)
	if p, err := netip.ParsePrefix(entry); err == nil {
		return p.Masked(), "", true
	}
	if ip, err := netip.ParseAddr(entry); err == nil {
		return netip.PrefixFrom(ip, ip.BitLen()), "", true
	}
	d := cleanDomain(entry)
	if d == "" {
		return netip.Prefix{}, "", false
	}
	return netip.Prefix{}, d, true
}

// cleanDomain is s as a bare lower-case domain, "" if it cannot be one.
func cleanDomain(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "+.")
	s = strings.TrimPrefix(s, "*.")
	s = strings.Trim(s, ".")
	if s == "" || strings.ContainsAny(s, " \t,;/:=#*+@") {
		return ""
	}
	return s
}
