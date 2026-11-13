package core

import (
	"slices"
	"testing"

	"github.com/oboo/moho/internal/devnet"
)

// Without TUN, without the built-in DNS switch and with no developer
// networks, the core used to be given no resolver at all. Then a node whose
// server is a domain could not be dialled, and one with ECH — which asks DNS
// for its config before the TLS handshake — could not be dialled either,
// while every other provider was fine.
func TestBuildConfigKeepsAResolver(t *testing.T) {
	in := devInput("rule", devnet.Overlay{})
	dns, _ := build(t, in)["dns"].(map[string]any)
	if dns == nil || dns["enable"] != true {
		t.Fatalf("no resolver without TUN: %v", dns)
	}
	// Normal mode: with no tunnel there is nothing to map fake addresses
	// back from, and nothing may listen on the machine's port 53 either.
	if dns["enhanced-mode"] != "normal" || dns["listen"] != nil {
		t.Fatalf("%+v", dns)
	}
	if got := strs(dns["nameserver"]); len(got) == 0 {
		t.Fatalf("no nameserver: %v", dns)
	}

	// A settings file with none chosen still leaves a resolver that works.
	empty := devInput("rule", devnet.Overlay{})
	empty.Options.Nameservers = nil
	dns, _ = build(t, empty)["dns"].(map[string]any)
	if got := strs(dns["nameserver"]); len(got) == 0 {
		t.Fatalf("no nameserver and no default: %v", dns)
	}
}

// TUN and the gateway keep their own resolver: this did not touch them.
func TestBuildConfigResolverWithTun(t *testing.T) {
	in := devInput("rule", devnet.Overlay{})
	in.Tun = true
	dns, _ := build(t, in)["dns"].(map[string]any)
	if dns == nil || dns["listen"] == nil {
		t.Fatalf("TUN lost its resolver: %v", dns)
	}
	if dns["enhanced-mode"] != "fake-ip" {
		t.Fatalf("TUN mode changed: %v", dns["enhanced-mode"])
	}
}

// The core looks a DNS server's own name up over plain port 53 before it can
// ask it anything. Where that question is led back into the tunnel it is
// answered with a fake address, and then nothing resolves: the servers Moho
// knows the addresses of are written into hosts, which the core reads first.
func TestBuildConfigKnowsItsNameservers(t *testing.T) {
	in := devInput("rule", devnet.Overlay{})
	in.Tun = true
	hosts, _ := build(t, in)["hosts"].(map[string]any)
	for _, name := range []string{"dns.alidns.com", "doh.pub"} {
		got := strs(hosts[name])
		if len(got) == 0 || !slices.Equal(got, NameserverHosts[name]) {
			t.Fatalf("%s: %v in %v", name, got, hosts)
		}
	}
	if _, ok := hosts["dot.pub"]; ok {
		t.Fatalf("a server nobody asks: %v", hosts)
	}

	// Without TUN the resolver is the same servers, and so are the hosts.
	in.Tun = false
	if hosts, _ = build(t, in)["hosts"].(map[string]any); len(strs(hosts["doh.pub"])) == 0 {
		t.Fatalf("no hosts without TUN: %v", hosts)
	}

	// The user's own line for the name stays the user's.
	in.Options.Hosts = []string{"doh.pub=10.0.0.53"}
	if hosts, _ = build(t, in)["hosts"].(map[string]any); hosts["doh.pub"] != "10.0.0.53" {
		t.Fatalf("the user's host was replaced: %v", hosts["doh.pub"])
	}

	// Other servers get nothing: by address there is no name to look up,
	// and an unknown name is the bootstrap servers' to answer.
	in.Options.Hosts = nil
	in.Options.Nameservers = []string{"https://223.5.5.5/dns-query", "tls://dns.example.com", "system"}
	in.Options.Fallback = []string{"tls://dot.pub#DIRECT"}
	in.Options.NameserverPolicy = []string{"+.cn=https://DNS.alidns.com/dns-query"}
	hosts, _ = build(t, in)["hosts"].(map[string]any)
	if len(hosts) != 2 || len(strs(hosts["dot.pub"])) == 0 || len(strs(hosts["dns.alidns.com"])) == 0 {
		t.Fatalf("%v", hosts)
	}
}

// Connections that go direct are resolved by the upstream servers unless
// they are given servers of their own. On a network that reaches a site only
// at the addresses its own DNS gives, the public servers' answer times out.
func TestBuildConfigDirectNameservers(t *testing.T) {
	in := devInput("rule", devnet.Overlay{})
	for _, tun := range []bool{false, true} {
		in.Tun = tun
		in.Options.DirectNameservers = nil
		dns, _ := build(t, in)["dns"].(map[string]any)
		if dns["direct-nameserver"] != nil || dns["direct-nameserver-follow-policy"] != nil {
			t.Fatalf("tun=%v: servers nobody chose: %v", tun, dns)
		}
		in.Options.DirectNameservers = []string{"system", "10.0.0.53"}
		dns, _ = build(t, in)["dns"].(map[string]any)
		if got := strs(dns["direct-nameserver"]); !slices.Equal(got, []string{"system", "10.0.0.53"}) {
			t.Fatalf("tun=%v: %v", tun, dns)
		}
		// The policy's domains keep their servers.
		if dns["direct-nameserver-follow-policy"] != true {
			t.Fatalf("tun=%v: the policy is skipped: %v", tun, dns)
		}
	}

	// With split DNS and no resolver switched on, the third shape.
	split := devInput("rule", devnet.Overlay{Policy: []devnet.Policy{{Pattern: "+.corp.example", Server: "10.0.0.53"}}})
	split.Options.DirectNameservers = []string{"system"}
	dns, _ := build(t, split)["dns"].(map[string]any)
	if got := strs(dns["direct-nameserver"]); !slices.Equal(got, []string{"system"}) {
		t.Fatalf("split DNS: %v", dns)
	}

	// A server known by name that only these use still has its address.
	in.Options.Nameservers = []string{"223.5.5.5"}
	in.Options.DirectNameservers = []string{"tls://dot.pub"}
	hosts, _ := build(t, in)["hosts"].(map[string]any)
	if !slices.Equal(strs(hosts["dot.pub"]), NameserverHosts["dot.pub"]) {
		t.Fatalf("no hosts for the direct server: %v", hosts)
	}
}
