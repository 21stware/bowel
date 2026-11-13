package devnet

import (
	"net/netip"
	"slices"
	"strings"
	"testing"
)

const netstatSample = `Routing tables

Internet:
Destination        Gateway            Flags               Netif Expire
default            192.168.50.1       UGScg                 en0
default            link#31            UCSIg               utun9
1                  198.18.0.1         UGSc              utun13
2/7                198.18.0.1         UGSc              utun13
128.0/1            198.18.0.1         UGSc              utun13
10.8/16            link#20            UCS                utun5
10.8.0.2           10.8.0.1           UHWIi              utun5
100.64/10          link#25            UCS                utun4
100.100.100.100/32 link#25            UCS                utun4
100.101.5.6        link#25            UHWIi              utun4
172.31.7/24        link#25            UCS                utun4
192.168.50         link#14            UCS                  en0
192.168.50.1/32    link#14            UCS                  en0
224.0.0/4          link#20            UmCS               utun5
255.255.255.255/32 link#14            UCSI                 en0

Internet6:
Destination                             Gateway                                 Flags               Netif Expire
default                                 fe80::%utun0                            UGcIg               utun0
fd7a:115c:a1e0::/48                     fe80::%utun4                            UCS                 utun4
fd12:3456::/64                          fe80::%utun5                            UCS                 utun5
fe80::%utun0/64                         fe80::1%utun0                           UcI                 utun0
ff00::/8                                ::1                                     UmCI                 lo0
`

func TestParseRoutes(t *testing.T) {
	rs := parseRoutes(netstatSample)
	vpn, subnets, holders := tunnels(rs, "utun4")
	if want := []string{"utun13"}; !slices.Equal(holders, want) {
		t.Errorf("holders %v, want %v", holders, want)
	}
	if want := []netip.Prefix{netip.MustParsePrefix("172.31.7.0/24")}; !slices.Equal(subnets, want) {
		t.Errorf("subnets %v, want %v", subnets, want)
	}
	if len(vpn) != 1 || vpn[0].Iface != "utun5" {
		t.Fatalf("vpn %+v", vpn)
	}
	want := []netip.Prefix{netip.MustParsePrefix("10.8.0.0/16"), netip.MustParsePrefix("fd12:3456::/64")}
	if !slices.Equal(vpn[0].Prefixes, want) {
		t.Errorf("prefixes %v, want %v", vpn[0].Prefixes, want)
	}
}

func TestParseDest(t *testing.T) {
	for in, want := range map[string]string{
		"10":              "10.0.0.0/8",
		"172.16/12":       "172.16.0.0/12",
		"192.168.50":      "192.168.50.0/24",
		"1.1.1.1":         "1.1.1.1/32",
		"128.0/1":         "128.0.0.0/1",
		"fd7a::/48":       "fd7a::/48",
		"fe80::%utun0/64": "fe80::/64",
	} {
		p, ok := parseDest(in)
		if !ok || p.String() != want {
			t.Errorf("parseDest(%q) = %v %v, want %s", in, p, ok, want)
		}
	}
	if _, ok := parseDest("link#14"); ok {
		t.Error("link#14 is no destination")
	}
}

const scutilSample = `DNS configuration

resolver #1
  nameserver[0] : 198.18.0.2
  if_index : 31 (utun13)
  flags    : Request A records
  reach    : 0x00000002 (Reachable)

resolver #2
  domain   : local
  options  : mdns
  timeout  : 5
  order    : 300000

resolver #3
  domain   : 254.169.in-addr.arpa
  options  : mdns

resolver #4
  domain   : aws
  nameserver[0] : 127.0.0.1
  port     : 5053
  flags    : Request A records

resolver #5
  domain   : tail17b34.ts.net
  nameserver[0] : 100.100.100.100
  if_index : 25 (utun4)

resolver #6
  domain   : corp.example.
  nameserver[0] : 10.8.0.53
  nameserver[1] : 198.18.0.2
  if_index : 20 (utun5)

DNS configuration (for scoped queries)

resolver #1
  domain   : scoped.example
  nameserver[0] : 8.8.8.8
`

func TestParseResolvers(t *testing.T) {
	got := parseResolvers(scutilSample, proxyDNS)
	want := []Resolver{
		{Domain: "aws", Servers: []string{"127.0.0.1:5053"}},
		{Domain: "tail17b34.ts.net", Servers: []string{magicResolver}, Iface: "utun4"},
		{Domain: "corp.example", Servers: []string{"10.8.0.53"}, Iface: "utun5"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i].Domain != want[i].Domain || got[i].Iface != want[i].Iface || !slices.Equal(got[i].Servers, want[i].Servers) {
			t.Errorf("%d: %+v, want %+v", i, got[i], want[i])
		}
	}
}

const statusSample = `{
  "BackendState": "Running",
  "TailscaleIPs": ["100.100.1.1", "fd7a:115c:a1e0::1"],
  "MagicDNSSuffix": "tail17b34.ts.net.",
  "CurrentTailnet": {"Name": "me@example.com", "MagicDNSSuffix": "tail17b34.ts.net", "MagicDNSEnabled": true},
  "Peer": {
    "a": {"HostName": "MyBox", "DNSName": "mybox.tail17b34.ts.net.", "TailscaleIPs": ["100.100.1.2"], "ExitNode": true},
    "b": {"HostName": "Other Host", "DNSName": "", "TailscaleIPs": []}
  }
}`

func TestParseStatus(t *testing.T) {
	ts := parseStatus([]byte(statusSample))
	if ts.State != "Running" || ts.Suffix != "tail17b34.ts.net" || !ts.MagicDNS || ts.ExitNode != "mybox" {
		t.Fatalf("%+v", ts)
	}
	if len(ts.Peers) != 1 || ts.Peers[0].Name != "mybox" || ts.Peers[0].FQDN != "mybox.tail17b34.ts.net" {
		t.Fatalf("peers %+v", ts.Peers)
	}
	stopped := parseStatus([]byte(strings.Replace(statusSample, `"Running"`, `"Stopped"`, 1)))
	if len(stopped.Peers) != 0 || stopped.ExitNode != "" {
		t.Errorf("a stopped tailnet has no peers: %+v", stopped)
	}
}

func TestTailscaleWithoutCLI(t *testing.T) {
	rs := []Resolver{{Domain: "ts.example", Servers: []string{magicResolver}}}
	if ts := tailscaleByInterface(Tailscale{}, "utun4", rs); ts.Iface != "utun4" || !ts.MagicDNS {
		t.Errorf("%+v", ts)
	}
	if ts := tailscaleByInterface(Tailscale{}, "utun4", nil); ts.Iface != "" {
		t.Errorf("a CGNAT tunnel alone is not Tailscale: %+v", ts)
	}
	if ts := tailscaleByInterface(Tailscale{Installed: true, State: "Stopped"}, "utun4", rs); ts.Iface != "" {
		t.Errorf("it said it was stopped: %+v", ts)
	}
}

func runningEnv() Env {
	return Env{
		Tailscale: Tailscale{
			Installed: true, State: "Running", Iface: "utun4", Suffix: "tail17b34.ts.net", MagicDNS: true,
			Peers:   []Peer{{Name: "mybox", FQDN: "mybox.tail17b34.ts.net", IPs: []netip.Addr{netip.MustParseAddr("100.100.1.2")}}},
			Subnets: []netip.Prefix{netip.MustParsePrefix("172.31.7.0/24")},
		},
		VPNs:      []Tunnel{{Iface: "utun5", Kind: "vpn", Prefixes: []netip.Prefix{netip.MustParsePrefix("10.8.0.0/16")}}},
		Resolvers: []Resolver{{Domain: "corp.example", Servers: []string{"10.8.0.53"}, Iface: "utun5"}, {Domain: "aws", Servers: []string{"127.0.0.1:5053"}}},
	}
}

func ruleOf(o Overlay, text string) (Rule, bool) {
	i := slices.IndexFunc(o.Rules, func(r Rule) bool { return strings.HasPrefix(r.Text, text) })
	if i < 0 {
		return Rule{}, false
	}
	return o.Rules[i], true
}

func TestCompileCatalogAlone(t *testing.T) {
	o := Compile(Options{}, Env{})
	if len(o.Outbounds) != 0 || len(o.Policy) != 0 {
		t.Errorf("nothing was detected: %+v %+v", o.Outbounds, o.Policy)
	}
	if r, ok := ruleOf(o, "DOMAIN-SUFFIX,ts.net"); !ok || r.Target != Direct {
		t.Errorf("ts.net: %+v %v", r, ok)
	}
	for _, want := range []string{"10.0.0.0/8", "100.64.0.0/10", "fe80::/10", "fd7a:115c:a1e0::/48"} {
		if !slices.Contains(o.TunExclude, want) {
			t.Errorf("%s is not excluded from TUN: %v", want, o.TunExclude)
		}
	}
	for _, public := range []string{"foo.dev", "foo.app", "login.tailscale.com", "example.com"} {
		if r, ok := o.Match(public); ok {
			t.Errorf("%s is a public name: %+v", public, r)
		}
	}
	for _, private := range []string{"localhost", "app.localhost", "printer.local", "api.test", "host.docker.internal", "web.default.svc", "127.0.0.1", "192.168.1.1", "100.100.1.2", "10.0.0.1", "10.0.0.1.nip.io"} {
		if _, ok := o.Match(private); !ok {
			t.Errorf("%s should stay direct", private)
		}
	}
}

func TestCompileTailscale(t *testing.T) {
	o := Compile(Options{}, runningEnv())
	if !slices.Contains(o.Outbounds, Outbound{Name: TailscaleOutbound, Iface: "utun4"}) || !slices.Contains(o.Outbounds, Outbound{Name: "VPN-utun5", Iface: "utun5"}) {
		t.Errorf("outbounds %+v", o.Outbounds)
	}
	for host, target := range map[string]string{
		"mybox":                  TailscaleOutbound,
		"mybox.tail17b34.ts.net": TailscaleOutbound,
		"other.ts.net":           TailscaleOutbound,
		"100.100.1.2":            TailscaleOutbound,
		"fd7a:115c:a1e0::5":      TailscaleOutbound,
		"172.31.7.9":             TailscaleOutbound,
		"10.8.1.1":               "VPN-utun5",
		"git.corp.example":       "VPN-utun5",
		"foo.aws":                Direct,
		"10.9.0.1":               Direct,
	} {
		if r, ok := o.Match(host); !ok || r.Target != target {
			t.Errorf("%s: %+v %v, want %s", host, r, ok, target)
		}
	}
	for _, p := range []Policy{
		{"+.tail17b34.ts.net", magicResolver + "#" + TailscaleOutbound},
		{"+.ts.net", magicResolver + "#" + TailscaleOutbound},
		{"+.corp.example", "10.8.0.53#VPN-utun5"},
		{"+.aws", "127.0.0.1:5053"},
	} {
		if !slices.Contains(o.Policy, p) {
			t.Errorf("no %+v in %+v", p, o.Policy)
		}
	}
	if got := o.Hosts["mybox"]; !slices.Equal(got, []string{"100.100.1.2"}) {
		t.Errorf("hosts: %v", o.Hosts)
	}
	if !slices.Contains(o.Bypass, "mybox") || !slices.Contains(o.FakeIPFilter, "mybox") || !slices.Contains(o.FakeIPFilter, "+.tail17b34.ts.net") {
		t.Errorf("bypass %v filter %v", o.Bypass, o.FakeIPFilter)
	}
	// A VPN that comes and goes must not rebuild TUN.
	if slices.Contains(o.TunExclude, "10.8.0.0/16") || slices.Contains(o.TunExclude, "172.31.7.0/24") {
		t.Errorf("tun exclude follows what was detected: %v", o.TunExclude)
	}
}

// The core picks one address of a hosts entry at random and tries no other.
func TestHostsHoldOneFamily(t *testing.T) {
	env := runningEnv()
	both := []netip.Addr{netip.MustParseAddr("fd7a:115c:a1e0::2"), netip.MustParseAddr("100.100.1.2")}
	env.Tailscale.Peers = []Peer{
		{Name: "mybox", FQDN: "mybox.tail17b34.ts.net", IPs: both},
		{Name: "six", FQDN: "six.tail17b34.ts.net", IPs: both[:1]},
	}
	o := Compile(Options{}, env)
	if got := o.Hosts["mybox"]; !slices.Equal(got, []string{"100.100.1.2"}) {
		t.Errorf("mybox: %v", got)
	}
	if got := o.Hosts["six"]; !slices.Equal(got, []string{"fd7a:115c:a1e0::2"}) {
		t.Errorf("six: %v", got)
	}
	if _, ok := o.Hosts["mybox.tail17b34.ts.net"]; ok {
		t.Errorf("MagicDNS answers for the full name: %v", o.Hosts)
	}
	env.Tailscale.MagicDNS = false
	if got := Compile(Options{}, env).Hosts["mybox.tail17b34.ts.net"]; !slices.Equal(got, []string{"100.100.1.2"}) {
		t.Errorf("without MagicDNS only hosts can name it: %v", got)
	}
}

func TestShadowed(t *testing.T) {
	const table = `100.64/10          192.168.50.1       UGSc                  en0
100.64/10          link#34            UCSI               utun11
100.100.100.100/32 link#34            UCS                utun11
`
	if !shadowed(parseRoutes(table), "utun11") {
		t.Error("the physical gateway holds the range and Tailscale's route is scoped")
	}
	const own = `default            192.168.50.1       UGScg                 en0
100.64/10          link#34            UCS                utun11
100.100.100.100/32 link#34            UCS                utun11
`
	if shadowed(parseRoutes(own), "utun11") {
		t.Error("the range is Tailscale's")
	}
	if shadowed(parseRoutes(table), "") {
		t.Error("nothing to shadow while Tailscale is off")
	}
}

// When the system would lose the tailnet, everything has to reach the core.
func TestShadowedGoesThroughTheCore(t *testing.T) {
	env := runningEnv()
	env.Tailscale.Shadowed = true
	o := Compile(Options{}, env)
	for _, gone := range []string{"100.64.0.0/10", "*.ts.net", "*.tail17b34.ts.net", "mybox"} {
		if slices.Contains(o.Bypass, gone) {
			t.Errorf("%s bypasses the core: %v", gone, o.Bypass)
		}
	}
	if slices.Contains(o.TunExclude, "100.64.0.0/10") {
		t.Errorf("tun exclude: %v", o.TunExclude)
	}
	for _, host := range []string{"mybox", "other.ts.net", "100.100.1.2"} {
		if r, ok := o.Match(host); !ok || r.Target != TailscaleOutbound {
			t.Errorf("%s: %+v %v", host, r, ok)
		}
	}
}

func TestRulesLongestFirst(t *testing.T) {
	o := Compile(Options{Extra: []string{"10.1.2.0/24", "a.b.lan"}}, runningEnv())
	r, _ := o.Match("10.1.2.3")
	if !strings.Contains(r.Text, "10.1.2.0/24") {
		t.Errorf("10.1.2.3 matched %+v", r)
	}
	if r, _ := o.Match("x.a.b.lan"); !strings.Contains(r.Text, "a.b.lan") {
		t.Errorf("x.a.b.lan matched %+v", r)
	}
}

func TestDisabledScenario(t *testing.T) {
	o := Compile(Options{Disabled: []string{Mesh, "lan-names"}}, runningEnv())
	if len(o.Hosts) != 0 || slices.Contains(o.TunExclude, "100.64.0.0/10") {
		t.Errorf("mesh is off: %+v %v", o.Hosts, o.TunExclude)
	}
	if _, ok := o.Match("printer.local"); ok {
		t.Error("lan-names is off")
	}
	if slices.ContainsFunc(o.Outbounds, func(b Outbound) bool { return b.Name == TailscaleOutbound }) {
		t.Error("no outbound for a switched-off mesh")
	}
}

func TestProxiedRulesReachTheCore(t *testing.T) {
	o := Compile(Options{Proxied: []string{"DOMAIN-SUFFIX,wiki.corp", "IP-CIDR,10.20.0.0/16"}}, Env{})
	if slices.Contains(o.Bypass, "*.corp") || slices.Contains(o.NoProxy, ".corp") {
		t.Errorf("the system keeps .corp from the core: %v", o.Bypass)
	}
	if slices.Contains(o.TunExclude, "10.0.0.0/8") || slices.Contains(o.Bypass, "10.0.0.0/8") {
		t.Errorf("10/8 stays out of the core: %v", o.TunExclude)
	}
	if !slices.Contains(o.Bypass, "*.lan") || !slices.Contains(o.TunExclude, "192.168.0.0/16") {
		t.Error("the rest stays as it was")
	}
	if !o.Covers("DOMAIN-SUFFIX,wiki.corp") || !o.Covers("IP-CIDR,10.20.0.0/16") || o.Covers("DOMAIN-SUFFIX,example.com") {
		t.Error("Covers")
	}
}

func TestKeeps(t *testing.T) {
	o := Compile(Options{}, runningEnv())
	if !o.Keeps("", []string{TailscaleOutbound}) || !o.Keeps("100.64.0.0/10", []string{"DIRECT"}) || !o.Keeps("local", []string{"DIRECT"}) {
		t.Error("the machine's own networks stay")
	}
	if o.Keeps("", []string{"DIRECT"}) || o.Keeps("example.com", []string{"节点"}) {
		t.Error("everything else goes")
	}
}

func TestLargeTailnetIsBounded(t *testing.T) {
	env := Env{Tailscale: Tailscale{Iface: "utun4"}}
	for i := range 2000 {
		name := "m" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + itoa(i)
		env.Tailscale.Peers = append(env.Tailscale.Peers, Peer{Name: name, IPs: []netip.Addr{netip.MustParseAddr("100.64.0.1")}})
	}
	o := Compile(Options{}, env)
	if len(o.Hosts) > 2*maxPeers {
		t.Errorf("%d hosts", len(o.Hosts))
	}
	if n := len(o.Bypass); n > maxShortNames+60 {
		t.Errorf("%d bypass entries", n)
	}
}

func itoa(i int) string {
	s := ""
	for ; i > 0; i /= 10 {
		s = string(rune('0'+i%10)) + s
	}
	return s
}

func TestValid(t *testing.T) {
	for _, ok := range []string{"corp.example", "*.corp.example", "+.corp.example", "10.8.0.0/16", "10.8.0.1", "fd00::/8"} {
		if !Valid(ok) {
			t.Errorf("%s is valid", ok)
		}
	}
	for _, bad := range []string{"", "a b", "a,b", "http://x/", "1.2.3.4/99"} {
		if Valid(bad) {
			t.Errorf("%q is not valid", bad)
		}
	}
}

// The servers every other name goes to: the first resolver that is for no
// domain of its own (#117).
func TestParseDefaultDNS(t *testing.T) {
	if got := parseDefaultDNS(scutilSample); !slices.Equal(got, []string{"198.18.0.2"}) {
		t.Errorf("sample: %v", got)
	}
	lan := `DNS configuration

resolver #1
  domain   : corp.example
  nameserver[0] : 10.9.0.53

resolver #2
  nameserver[0] : 192.168.50.1
  nameserver[1] : fe80::1%en1
  if_index : 16 (en1)
  flags    : Request A records

resolver #3
  domain   : local
  options  : mdns

DNS configuration (for scoped queries)

resolver #1
  nameserver[0] : 172.16.0.1
`
	if got := parseDefaultDNS(lan); !slices.Equal(got, []string{"192.168.50.1", "fe80::1%en1"}) {
		t.Errorf("lan: %v", got)
	}
	if got := parseDefaultDNS("DNS configuration\n\nresolver #1\n  domain : local\n  options : mdns\n\nDNS configuration (for scoped queries)\n\nresolver #1\n  nameserver[0] : 172.16.0.1\n"); got != nil {
		t.Errorf("offline: %v", got)
	}
}

func TestParseResolvConf(t *testing.T) {
	got := parseResolvConf("# Generated by NetworkManager\nsearch lan\nnameserver 192.168.1.1\nnameserver\t2400:3200::1\noptions edns0\n")
	if !slices.Equal(got, []string{"192.168.1.1", "2400:3200::1"}) {
		t.Errorf("resolv.conf: %v", got)
	}
}

// The network's own servers reach the overlay only with names to ask them.
func TestLocalDNSInOverlay(t *testing.T) {
	env := Env{LocalDNS: []string{"192.168.50.1"}}
	if o := Compile(Options{}, env); !slices.Equal(o.LocalDNS, env.LocalDNS) || len(o.SystemDNS) == 0 {
		t.Errorf("overlay: local %v system %v", o.LocalDNS, o.SystemDNS)
	}
	if a, b := Compile(Options{}, env).Fingerprint(), Compile(Options{}, Env{}).Fingerprint(); a == b {
		t.Error("a change of the network's servers does not reload the config")
	}
}

// The core's own node gets the tailnet's addresses and names, and nothing
// is kept from the core: no bypass, no fake-ip exception, no TUN exclusion.
func TestTsnetOverlay(t *testing.T) {
	o := TsnetOverlay(Tsnet{Hostname: "p"})
	if len(o.Outbounds) != 1 || o.Outbounds[0].Tsnet == nil || o.Outbounds[0].Name != TailscaleOutbound {
		t.Fatalf("outbounds %+v", o.Outbounds)
	}
	for _, h := range []string{"100.85.4.93", "box.tail17b34.ts.net", "fd7a:115c:a1e0::1"} {
		if r, ok := o.Match(h); !ok || r.Target != TailscaleOutbound {
			t.Errorf("%s: %+v %v", h, r, ok)
		}
	}
	if _, ok := o.Match("example.com"); ok {
		t.Error("example.com matched")
	}
	if !o.Covers("IP-CIDR,100.85.0.0/16,x") || !o.Covers("DOMAIN-SUFFIX,box.ts.net,x") {
		t.Error("Covers")
	}
	if len(o.Bypass)+len(o.NoProxy)+len(o.FakeIPFilter)+len(o.TunExclude)+len(o.SniffSkip)+len(o.Policy) > 0 {
		t.Errorf("keeps something from the core: %+v", o)
	}
}
