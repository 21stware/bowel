package core

import (
	"slices"
	"testing"
)

const dnsConfig = `
proxies: [{name: n, type: direct}]
proxy-groups: [{name: g, type: select, proxies: [n]}]
rules: ["MATCH,g"]
shared: &ns [https://dns.example/dns-query, "tls://8.8.4.4"]
dns:
  enable: false
  listen: 0.0.0.0:1053
  enhanced-mode: redir-host
  fake-ip-range: 10.9.0.1/16
  default-nameserver: [223.5.5.5, "tls://1.12.12.12", system, https://doh.pub/dns-query]
  nameserver: *ns
  fallback: tls://1.1.1.1
  proxy-server-nameserver: [223.6.6.6, 223.6.6.6]
  direct-nameserver: [system]
  fake-ip-filter: ["*.lan", "rule-set:cn", "geosite:private"]
  nameserver-policy:
    "+.corp.example": [10.0.0.2, 10.0.0.3]
    "geosite:cn,private": https://doh.pub/dns-query
    "rule-set:mine": 1.2.3.4
hosts:
  nas.home: 192.168.1.9
  two.home: [10.0.0.1, 10.0.0.2]
  bad.home: {}
`

func TestImportedDNS(t *testing.T) {
	im, err := ParseImported([]byte(dnsConfig), nil)
	if err != nil || im == nil {
		t.Fatal(im, err)
	}
	f := im.DNS
	for _, c := range []struct {
		key       string
		got, want []string
	}{
		{"nameserver", f.Nameservers, []string{"https://dns.example/dns-query", "tls://8.8.4.4"}},
		{"default-nameserver", f.DefaultNameservers, []string{"223.5.5.5", "tls://1.12.12.12"}},
		{"fallback", f.Fallback, []string{"tls://1.1.1.1"}},
		{"proxy-server-nameserver", f.ProxyNameservers, []string{"223.6.6.6"}},
		{"direct-nameserver", f.DirectNameservers, []string{"system"}},
		{"fake-ip-filter", f.FakeIPFilter, []string{"*.lan", "geosite:private"}},
		{"nameserver-policy", f.NameserverPolicy, []string{"+.corp.example=10.0.0.2", "+.corp.example=10.0.0.3", "geosite:cn,private=https://doh.pub/dns-query"}},
		{"hosts", f.Hosts, []string{"nas.home=192.168.1.9", "two.home=10.0.0.1"}},
		{"skipped", f.Skipped, []string{
			"default-nameserver: system", "default-nameserver: https://doh.pub/dns-query",
			"fake-ip-filter: rule-set:cn", "nameserver-policy: rule-set:mine",
			"hosts: bad.home", "hosts: two.home → 10.0.0.2",
		}},
	} {
		if !slices.Equal(c.got, c.want) {
			t.Errorf("%s = %q, want %q", c.key, c.got, c.want)
		}
	}

	// Into the options: what the config has replaces, the rest stays, and how
	// the resolver runs is not the config's to say.
	o := DefaultOptions.Clone()
	o.Fallback = []string{"9.9.9.9"}
	mode, rng := o.DNSMode, o.FakeIPRange
	took := f.Apply(&o)
	if want := []string{"nameserver", "default-nameserver", "fallback", "proxy-server-nameserver", "direct-nameserver", "nameserver-policy", "fake-ip-filter", "hosts"}; !slices.Equal(took, want) {
		t.Errorf("took %q, want %q", took, want)
	}
	if err := o.Validate(); err != nil {
		t.Errorf("the filled options are refused: %v", err)
	}
	if o.DNSMode != mode || o.FakeIPRange != rng || o.DNS {
		t.Errorf("the resolver's own settings changed: %s %s %v", o.DNSMode, o.FakeIPRange, o.DNS)
	}
	if again := f.Apply(&o); len(again) != 0 {
		t.Errorf("a second fill changed %q", again)
	}
	// The result is a config like any other: the policy is the user's, the
	// developer networks' go beside it, TUN and the gateway decide the rest.
	in := Input{Mode: "rule", MixedPort: 7890, Tun: true, Gateway: true, Imported: im, Options: o}
	cfg := build(t, in)
	dns := cfg["dns"].(map[string]any)
	if dns["enhanced-mode"] != "fake-ip" || dns["listen"] != "0.0.0.0:53" || dns["fake-ip-range"] != DefaultOptions.FakeIPRange {
		t.Errorf("gateway: %v", dns)
	}
	if pol := dns["nameserver-policy"].(map[string]any); len(pol["+.corp.example"].([]any)) != 2 {
		t.Errorf("policy: %v", pol)
	}
	if hosts := cfg["hosts"].(map[string]any); hosts["nas.home"] != "192.168.1.9" {
		t.Errorf("hosts: %v", hosts)
	}
}

func TestImportedDNSLeavesAlone(t *testing.T) {
	base := "proxies: [{name: n, type: direct}]\nproxy-groups: [{name: g, type: select, proxies: [n]}]\nrules: [\"MATCH,g\"]\n"
	for name, extra := range map[string]string{
		"no dns":    "",
		"empty dns": "dns: {enable: true}\nhosts: {}\n",
		"null":      "dns:\nhosts:\n",
		"not a map": "dns: yes\nhosts: [a]\n",
	} {
		im, err := ParseImported([]byte(base+extra), nil)
		if err != nil || im == nil {
			t.Fatal(name, err)
		}
		o := DefaultOptions.Clone()
		if took := im.DNS.Apply(&o); len(took) != 0 || len(im.DNS.Skipped) != 0 {
			t.Errorf("%s: took %q, skipped %q", name, took, im.DNS.Skipped)
		}
	}
	// A filter that lists what does get a fake address is the opposite of
	// the one the settings have: not taken, and said.
	if im, _ := ParseImported([]byte(base+"dns: {nameserver: [9.9.9.9]}\nhosts: [a]\n"), nil); !slices.Equal(im.DNS.Nameservers, []string{"9.9.9.9"}) {
		t.Errorf("a malformed hosts took the servers with it: %+v", im.DNS)
	}
	im, _ := ParseImported([]byte(base+"dns: {fake-ip-filter-mode: whitelist, fake-ip-filter: [\"+.x.com\"], nameserver: [1.1.1.1]}\n"), nil)
	if len(im.DNS.FakeIPFilter) != 0 || !slices.Equal(im.DNS.Skipped, []string{"fake-ip-filter: fake-ip-filter-mode: whitelist"}) || !slices.Equal(im.DNS.Nameservers, []string{"1.1.1.1"}) {
		t.Errorf("whitelist: %+v", im.DNS)
	}
}
