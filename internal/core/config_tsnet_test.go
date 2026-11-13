package core

import (
	"slices"
	"testing"

	"github.com/oboo/moho/internal/devnet"
)

// The phone's own node of a tailnet is a tailscale proxy with its state in
// the core's folder, and the tailnet's addresses and names go to it in every
// mode, names as names: only the node knows MagicDNS.
func TestTsnetOutbound(t *testing.T) {
	dev := devnet.TsnetOverlay(devnet.Tsnet{Hostname: "pixel", AuthKey: "tskey-auth-x", StateDir: "tailscale/ab12"})
	for _, mode := range []string{"rule", "global", "direct"} {
		cfg := build(t, devInput(mode, dev))
		if cfg["mode"] != "rule" {
			t.Errorf("%s: mode = %v", mode, cfg["mode"])
		}
		proxies, _ := cfg["proxies"].([]any)
		if len(proxies) != 1 {
			t.Fatalf("%s: proxies = %v", mode, proxies)
		}
		p := proxies[0].(map[string]any)
		want := map[string]any{"name": devnet.TailscaleOutbound, "type": "tailscale", "hostname": "pixel", "auth-key": "tskey-auth-x", "state-dir": "tailscale/ab12", "udp": true}
		for k, v := range want {
			if p[k] != v {
				t.Errorf("%s: %s = %v, want %v", mode, k, p[k], v)
			}
		}
		rules := strs(cfg["rules"])
		for _, r := range []string{
			"DOMAIN-SUFFIX,ts.net," + devnet.TailscaleOutbound,
			"IP-CIDR,100.64.0.0/10," + devnet.TailscaleOutbound + ",no-resolve",
			"IP-CIDR6,fd7a:115c:a1e0::/48," + devnet.TailscaleOutbound + ",no-resolve",
		} {
			if !slices.Contains(rules, r) {
				t.Errorf("%s: no %q in %v", mode, r, rules)
			}
		}
		last := rules[len(rules)-1]
		if want := map[string]string{"global": "MATCH,GLOBAL", "direct": "MATCH,DIRECT"}[mode]; want != "" && last != want {
			t.Errorf("%s: last rule %q", mode, last)
		}
		dns, _ := cfg["dns"].(map[string]any)
		if f := strs(dns["fake-ip-filter"]); slices.Contains(f, "+.ts.net") {
			t.Errorf("%s: ts.net kept from fake-ip: %v", mode, f)
		}
		tun, _ := cfg["tun"].(map[string]any)
		if tun != nil && tun["route-exclude-address"] != nil {
			t.Errorf("%s: TUN excludes %v", mode, tun["route-exclude-address"])
		}
	}
}

// Without a key or a name the core's defaults stand.
func TestTsnetOutboundDefaults(t *testing.T) {
	cfg := build(t, devInput("rule", devnet.TsnetOverlay(devnet.Tsnet{})))
	p := cfg["proxies"].([]any)[0].(map[string]any)
	if _, ok := p["auth-key"]; ok {
		t.Errorf("auth-key = %v", p["auth-key"])
	}
	if _, ok := p["hostname"]; ok {
		t.Errorf("hostname = %v", p["hostname"])
	}
	if p["state-dir"] != TsnetStateDir {
		t.Errorf("state-dir = %v", p["state-dir"])
	}
}
