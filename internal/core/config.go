package core

import (
	"errors"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/routes"
)

// NodeSep separates a node's own name from the provider marker Moho appends
// (mihomo would otherwise see duplicate names across providers). Display
// names cut at it.
const NodeSep = "​‹"

// ProbeGroup is the selector the location probe points at a node.
const ProbeGroup = "MOHO-PROBE"

// ProviderRef is an enabled provider and its file.
type ProviderRef struct {
	ID   string
	Path string
}

// Input is everything the runtime config is made of.
type Input struct {
	Providers  []ProviderRef
	Routes     []routes.Route
	Mode       string
	MixedPort  int
	AllowLan   bool
	Tun        bool
	LogLevel   string
	TestURL    string
	ProbePort  int
	Controller map[string]any

	// Options are the 设置 window's; zero values fall back to Moho's.
	Options
}

func providerName(id string) string { return "p-" + id }

// withTarget puts the target before a rule's trailing options:
// "GEOIP,CN,no-resolve" + DIRECT → "GEOIP,CN,DIRECT,no-resolve".
func withTarget(rule, target string) string {
	if r, ok := strings.CutSuffix(rule, ",no-resolve"); ok {
		return r + "," + target + ",no-resolve"
	}
	return rule + "," + target
}

// BuildConfig writes the mihomo config: one file proxy-provider per enabled
// provider, two groups per route (a selector, and an automatic url-test
// group the selector can point at), the rules of the template, and a
// listener bound to the probe group.
func BuildConfig(in Input) ([]byte, error) {
	o := in.Options.Clone()
	o.Fill()
	cfg := map[string]any{
		"mixed-port":          in.MixedPort,
		"allow-lan":           in.AllowLan,
		"mode":                in.Mode,
		"log-level":           in.LogLevel,
		"ipv6":                o.IPv6,
		"unified-delay":       o.UnifiedDelay,
		"tcp-concurrent":      o.TCPConcurrent,
		"find-process-mode":   o.FindProcess,
		"keep-alive-interval": o.KeepAliveInterval,
		"disable-keep-alive":  o.DisableKeepAlive,
		"profile":             map[string]any{"store-selected": true},
		"geodata-mode":        true,
		"geo-auto-update":     o.GeoAutoUpdate,
		"geo-update-interval": o.GeoUpdateInterval,
		"geox-url":            map[string]any{"geoip": o.GeoIPURL, "geosite": o.GeoSiteURL, "mmdb": o.MMDBURL},
	}
	if o.HTTPPort > 0 {
		cfg["port"] = o.HTTPPort
	}
	if o.SocksPort > 0 {
		cfg["socks-port"] = o.SocksPort
	}
	if in.AllowLan {
		cfg["lan-allowed-ips"] = o.LanAllowedIPs
	}
	if len(o.Authentication) > 0 {
		cfg["authentication"] = o.Authentication
		// This Mac (the system proxy, Moho's own probes) needs no password.
		cfg["skip-auth-prefixes"] = []string{"127.0.0.1/8", "::1/128"}
	}
	if o.Interface != "" {
		cfg["interface-name"] = o.Interface
	}
	if o.Fingerprint != "" {
		cfg["global-client-fingerprint"] = o.Fingerprint
	}
	for k, v := range in.Controller {
		if v != "" {
			cfg[k] = v
		}
	}
	// The dashboards' controller, unless Moho's own already took TCP.
	if own, _ := in.Controller["external-controller"].(string); own == "" && o.ExtController != "" {
		cfg["external-controller"] = o.ExtController
		cfg["secret"] = o.ExtSecret
	}
	if len(o.Hosts) > 0 {
		hosts := map[string]any{}
		for _, h := range o.Hosts {
			if k, v, ok := Pair(h); ok {
				hosts[k] = v
			}
		}
		cfg["hosts"] = hosts
	}
	if o.Sniff {
		cfg["sniffer"] = map[string]any{
			"enable": true, "override-destination": o.SniffOverride,
			"force-dns-mapping": o.SniffForceDNSMapping, "parse-pure-ip": o.SniffParsePureIP,
			"sniff": map[string]any{
				"HTTP": map[string]any{"ports": o.SniffHTTP},
				"TLS":  map[string]any{"ports": o.SniffTLS},
				"QUIC": map[string]any{"ports": o.SniffQUIC},
			},
			"skip-domain": o.SniffSkip,
		}
	}

	use := []string{}
	providers := map[string]any{}
	for _, p := range in.Providers {
		name := providerName(p.ID)
		use = append(use, name)
		providers[name] = map[string]any{
			"type":         "file",
			"path":         p.Path,
			"health-check": map[string]any{"enable": true, "url": in.TestURL, "interval": o.HealthInterval, "lazy": true},
			"override":     map[string]any{"additional-suffix": NodeSep + p.ID},
		}
	}
	if len(providers) > 0 {
		cfg["proxy-providers"] = providers
	}

	withNodes := func(g map[string]any, extra ...string) map[string]any {
		if len(use) > 0 {
			g["use"] = use
		}
		if len(extra) > 0 || len(use) == 0 {
			if len(extra) == 0 {
				extra = []string{"DIRECT"}
			}
			g["proxies"] = extra
		}
		return g
	}
	var groups []any
	var rules []string
	for _, r := range in.Routes {
		if r.Direct {
			for _, rule := range r.Rules {
				rules = append(rules, withTarget(rule, "DIRECT"))
			}
			continue
		}
		groups = append(groups,
			withNodes(map[string]any{"name": routes.Auto(r), "type": "url-test", "url": in.TestURL, "interval": o.URLTestInterval, "tolerance": o.URLTestTolerance, "lazy": true}),
			withNodes(map[string]any{"name": routes.Group(r), "type": "select"}, routes.Auto(r)),
		)
		if r.ID != routes.Default {
			for _, rule := range r.Rules {
				rules = append(rules, withTarget(rule, routes.Group(r)))
			}
		}
	}
	var def routes.Route
	for _, r := range in.Routes {
		if r.ID == routes.Default {
			def = r
		}
	}
	if def.Name == "" {
		return nil, errors.New(i18n.T("模板里没有默认路由", "The template has no default route"))
	}
	rules = append(rules, "MATCH,"+routes.Group(def))
	groups = append(groups, withNodes(map[string]any{"name": ProbeGroup, "type": "select"}, "DIRECT"))
	cfg["proxy-groups"] = groups
	cfg["rules"] = rules
	cfg["listeners"] = []any{map[string]any{
		"name": "moho-probe", "type": "mixed", "listen": "127.0.0.1", "port": in.ProbePort, "proxy": ProbeGroup,
	}}

	if in.Tun {
		tun := map[string]any{
			"enable": true, "stack": o.TunStack, "auto-route": true, "auto-detect-interface": o.AutoDetectInterface,
			"strict-route": o.StrictRoute, "dns-hijack": o.DNSHijack, "mtu": o.TunMTU,
			"endpoint-independent-nat": o.EndpointIndependentNAT,
		}
		if o.TunDevice != "" {
			tun["device"] = o.TunDevice
		}
		cfg["tun"] = tun
	}
	if in.Tun || o.DNS {
		dns := map[string]any{
			"enable": true, "ipv6": o.IPv6, "enhanced-mode": o.DNSMode,
			"default-nameserver": o.DefaultNameservers,
			"nameserver":         o.Nameservers,
			"use-hosts":          o.UseHosts,
			"use-system-hosts":   o.UseSystemHosts,
			"prefer-h3":          o.PreferH3,
			"respect-rules":      o.RespectRules,
		}
		if o.DNSMode == "fake-ip" {
			dns["fake-ip-range"] = o.FakeIPRange
			dns["fake-ip-filter"] = o.FakeIPFilter
		}
		if len(o.Fallback) > 0 {
			dns["fallback"] = o.Fallback
		}
		if len(o.ProxyNameservers) > 0 {
			dns["proxy-server-nameserver"] = o.ProxyNameservers
		}
		if len(o.NameserverPolicy) > 0 {
			policy := map[string][]string{}
			for _, p := range o.NameserverPolicy {
				if k, v, ok := Pair(p); ok {
					policy[k] = append(policy[k], v)
				}
			}
			dns["nameserver-policy"] = policy
		}
		if in.Tun {
			dns["listen"] = "127.0.0.1:1053"
		}
		cfg["dns"] = dns
	}
	return yaml.Marshal(cfg)
}
