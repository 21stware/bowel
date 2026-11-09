package core

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/routes"
)

// LocalDNSPort is where the core's resolver listens on this Mac, when it
// has one and devices on the LAN don't use it. A development build moves
// it, to run beside the released app.
var LocalDNSPort = 1053

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
	Providers []ProviderRef
	Routes    []routes.Route
	UserRules []routes.UserRule // sorted in with the routes' own, the more specific first
	Mode      string
	MixedPort int
	AllowLan  bool
	Tun       bool
	Gateway   bool // LAN devices use this Mac as gateway and DNS (needs Tun): the resolver answers them on port 53, with fake IPs
	LogLevel  string
	TestURL   string
	MainURLs  map[string]string // route id → what its automatic group tests; TestURL where absent
	// Scopes are the nodes a route's automatic group may pick from, by
	// route id; every node where absent. An empty list is no node at all:
	// a range is never widened on its own.
	Scopes map[string][]string
	// Orders are the nodes of a route's fallback group, first preferred,
	// by route id. Routes absent have no such group.
	Orders     map[string][]string
	ProbePort  int
	Controller map[string]any

	// Options are the 设置 window's; zero values fall back to Moho's.
	Options
}

func providerName(id string) string { return "p-" + id }

// providerOf is the provider a node's full name says it came from.
func providerOf(node string) string {
	if i := strings.LastIndex(node, NodeSep); i >= 0 {
		return node[i+len(NodeSep):]
	}
	return ""
}

// exactly is a group filter that keeps just these node names. mihomo
// splits a filter at backticks, so one in a name is matched by any
// character.
func exactly(names []string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = strings.ReplaceAll(regexp.QuoteMeta(n), "`", ".")
	}
	return "^(?:" + strings.Join(quoted, "|") + ")$"
}

// withTarget puts the target before a rule's trailing options:
// "GEOIP,CN,no-resolve" + DIRECT → "GEOIP,CN,DIRECT,no-resolve".
func withTarget(rule, target string) string {
	if r, ok := strings.CutSuffix(rule, ",no-resolve"); ok {
		return r + "," + target + ",no-resolve"
	}
	return rule + "," + target
}

// BuildConfig writes the mihomo config: one file proxy-provider per enabled
// provider, a selector per route with the groups it can point at (an
// automatic url-test one, and a fallback one if it has an order), every
// rule, the more specific first, and a listener bound to the probe group.
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
	for _, r := range in.Routes {
		if r.Direct {
			continue
		}
		mainURL := in.MainURLs[r.ID]
		if mainURL == "" {
			mainURL = in.TestURL
		}
		auto := map[string]any{"name": routes.Auto(r), "type": "url-test", "url": mainURL, "interval": o.URLTestInterval, "tolerance": o.URLTestTolerance, "lazy": true}
		if names, scoped := in.Scopes[r.ID]; !scoped {
			auto = withNodes(auto)
		} else if len(names) == 0 {
			auto["proxies"] = []string{"REJECT"} // nothing in range: no way out, not another way out
		} else {
			auto["use"], auto["filter"] = use, exactly(names)
		}
		groups = append(groups, auto)
		members := []string{routes.Auto(r)}
		if order, ok := in.Orders[r.ID]; ok {
			backup := map[string]any{"name": routes.Backup(r), "type": "fallback", "url": mainURL, "interval": o.URLTestInterval, "lazy": true}
			var steps []string
			for i, n := range order {
				step := routes.BackupStep(r, i)
				steps = append(steps, step)
				groups = append(groups, map[string]any{"name": step, "type": "select", "use": []string{providerName(providerOf(n))}, "filter": exactly([]string{n})})
			}
			if len(steps) == 0 {
				steps = []string{"REJECT"}
			}
			backup["proxies"] = steps
			groups = append(groups, backup)
			members = append(members, routes.Backup(r))
		}
		groups = append(groups, withNodes(map[string]any{"name": routes.Group(r), "type": "select"}, members...))
	}
	var rules []string
	for _, l := range routes.Lines(in.Routes, in.UserRules) {
		for _, r := range in.Routes {
			if r.ID != l.Route {
				continue
			}
			target := routes.Group(r)
			switch {
			case r.Reject:
				target = "REJECT"
			case r.Direct:
				target = "DIRECT"
			}
			rules = append(rules, withTarget(l.Text, target))
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
	gateway := in.Tun && in.Gateway
	if gateway {
		o.DNSMode = "fake-ip"
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
		switch {
		case gateway:
			dns["listen"] = "0.0.0.0:53"
		case in.Tun:
			dns["listen"] = fmt.Sprintf("127.0.0.1:%d", LocalDNSPort)
		}
		cfg["dns"] = dns
	}
	return yaml.Marshal(cfg)
}
