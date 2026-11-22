package core

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/oboo/moho/internal/devnet"
	"github.com/oboo/moho/internal/routes"
)

// lockDNSConfig is the config for in, its IPv6 and DNS read back.
func lockDNSConfig(t *testing.T, in Input) (ipv6, dnsIPv6 bool, policy map[string][]string) {
	t.Helper()
	in.MixedPort, in.ProbePort, in.TestURL = 7899, 7900, "x"
	if len(in.Providers) == 0 {
		in.Providers = []ProviderRef{{ID: "a", Path: "a.yaml"}}
	}
	out, err := BuildConfig(in)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		IPv6 bool `yaml:"ipv6"`
		DNS  struct {
			IPv6   bool                `yaml:"ipv6"`
			Policy map[string][]string `yaml:"nameserver-policy"`
		} `yaml:"dns"`
	}
	if err := yaml.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg.IPv6, cfg.DNS.IPv6, cfg.DNS.Policy
}

// TestLockedDNS: the names of a locked group's rules are asked of the
// core's servers through the group; another group's aren't, nor are the
// patterns the user has servers for, nor what has no name to go by.
func TestLockedDNS(t *testing.T) {
	users := []routes.UserRule{
		{Kind: routes.KindSuffix, Value: "held.bowel-test.dev", Route: "ai"},
		{Kind: routes.KindDomain, Value: "one.bowel-test.dev", Route: "ai"},
		{Kind: routes.KindDomain, Value: "mine.bowel-test.dev", Route: "ai"},
		{Kind: routes.KindKeyword, Value: "heldword", Route: "ai"},
		{Kind: routes.KindApp, Value: "ChatGPT.exe", Route: "ai"},
		{Kind: routes.KindSuffix, Value: "free.bowel-test.dev", Route: "media"},
	}
	in := Input{Routes: lockedWith(users), UserRules: users, Mode: "rule"}
	in.Options.Nameservers = []string{"https://doh.pub/dns-query", "system", "223.5.5.5", "dhcp://en0", "tls://1.1.1.1#其他", "tailscale://x"}
	in.Options.NameserverPolicy = []string{"mine.bowel-test.dev=10.0.0.53"}
	for _, tun := range []bool{false, true} {
		in.Tun = tun
		_, _, policy := lockDNSConfig(t, in)
		via := []string{"https://doh.pub/dns-query#AI 服务", "223.5.5.5#AI 服务"}
		for _, key := range []string{"+.held.bowel-test.dev", "one.bowel-test.dev", "+.claude.ai"} {
			if !slices.Equal(policy[key], via) {
				t.Errorf("tun=%v: %s asks %v, not %v", tun, key, policy[key], via)
			}
		}
		if !slices.Equal(policy["mine.bowel-test.dev"], []string{"10.0.0.53"}) {
			t.Errorf("tun=%v: the user's own server was replaced: %v", tun, policy["mine.bowel-test.dev"])
		}
		for key, servers := range policy {
			if strings.Contains(key, "heldword") || strings.Contains(key, "free.bowel-test.dev") {
				t.Errorf("tun=%v: %s got %v", tun, key, servers)
			}
		}
	}
}

// TestLockedDNSNot: no lock, or takeover off (the group is refused, and
// its names with it), and no server goes by a group.
func TestLockedDNSNot(t *testing.T) {
	users := []routes.UserRule{{Kind: routes.KindSuffix, Value: "held.bowel-test.dev", Route: "ai"}}
	for name, in := range map[string]Input{
		"unlocked": {Routes: routes.Build(routes.Plan{}, users), UserRules: users, Mode: "rule"},
		"guard":    {Routes: lockedWith(users), UserRules: users, Mode: "rule", Guard: true},
	} {
		_, _, policy := lockDNSConfig(t, in)
		for key, servers := range policy {
			for _, s := range servers {
				if strings.Contains(s, "#") {
					t.Errorf("%s: %s asks %s", name, key, s)
				}
			}
		}
	}
}

// TestLockedDNSRuleSets: a rule set of domains, or of whole rules, is a
// pattern; one of addresses, which the core refuses as one, isn't.
func TestLockedDNSRuleSets(t *testing.T) {
	var p routes.Plan
	p.SetLocked("ai", true)
	names := p.AddRuleSet(nil, "ai", "names", "https://x.example/names.yaml")
	rules := p.AddRuleSet(nil, "ai", "rules", "https://x.example/rules.yaml")
	addrs := p.AddRuleSet(nil, "ai", "addrs", "https://x.example/addrs.yaml")
	in := Input{Routes: routes.Build(p, nil), Mode: "rule", RuleSets: []RuleSetRef{
		{ID: names, Path: "/x/names.yaml", Behavior: "domain"},
		{ID: rules, Path: "/x/rules.yaml", Behavior: "classical"},
		{ID: addrs, Path: "/x/addrs.yaml", Behavior: "ipcidr"},
	}}
	_, _, policy := lockDNSConfig(t, in)
	for id, want := range map[string]bool{names: true, rules: true, addrs: false} {
		if _, ok := policy["rule-set:"+routes.RuleSetProvider(id)]; ok != want {
			t.Errorf("rule set %s in the policy: %v", id, ok)
		}
	}
}

// withName is all with route id's group named name in the core.
func withName(all []routes.Route, id, name string) []routes.Route {
	for i := range all {
		if all[i].ID == id {
			all[i].Name = name
		}
	}
	return all
}

// TestLockedDNSGroupNames: the core reads a server's # part as the proxy
// and & or = in it as options, unescaping % first. A group the user makes
// is named by its id in the core; a built-in one by its Chinese name.
func TestLockedDNSGroupNames(t *testing.T) {
	users := []routes.UserRule{{Kind: routes.KindSuffix, Value: "held.bowel-test.dev", Route: "ai"}}
	for name, want := range map[string]string{"全 100%": "#全 100%25", "A&B": "", "k=v": ""} {
		in := Input{Routes: withName(lockedWith(users), "ai", name), UserRules: users, Mode: "rule"}
		got := lockedDNS(in, Options{})["+.held.bowel-test.dev"]
		switch {
		case want == "" && got != nil:
			t.Errorf("%s: asks %v", name, got)
		case want != "" && (len(got) == 0 || !strings.HasSuffix(got[0], want)):
			t.Errorf("%s: asks %v, not by %s", name, got, want)
		}
	}
}

func TestPolicyPattern(t *testing.T) {
	for text, want := range map[string]string{
		"DOMAIN-SUFFIX,openai.com":      "+.openai.com",
		"DOMAIN,chatgpt.com":            "chatgpt.com",
		"GEOSITE,openai":                "geosite:openai",
		"DOMAIN-KEYWORD,openai":         "",
		"IP-CIDR,1.2.3.0/24,no-resolve": "",
		"GEOIP,US,no-resolve":           "",
		"PROCESS-NAME,ChatGPT.exe":      "",
	} {
		if got := policyPattern(routes.Line{Text: text}, nil); got != want {
			t.Errorf("%s: %q, not %q", text, got, want)
		}
	}
	if got := policyPattern(routes.Line{Text: "RULE-SET,rs-x,no-resolve", Set: "x"}, nil); got != "" {
		t.Errorf("a rule set whose file isn't there: %q", got)
	}
}

// TestLockTakesIPv6: under TUN with a group locked, the TUN takes IPv6
// whatever the setting, and DNS keeps to it.
func TestLockTakesIPv6(t *testing.T) {
	users := []routes.UserRule{{Kind: routes.KindApp, Value: "ChatGPT.exe", Route: "ai"}}
	for _, c := range []struct {
		name          string
		tun, lock     bool
		mode          IPv6Mode
		usable        bool
		ipv6, dnsIPv6 bool
	}{
		{"locked, off", true, true, IPv6Off, false, true, false},
		{"locked, auto without", true, true, IPv6Auto, false, true, false},
		{"locked, auto with", true, true, IPv6Auto, true, true, true},
		{"unlocked, off", true, false, IPv6Off, false, false, false},
		{"locked, no TUN", false, true, IPv6Off, false, false, false},
		{"unlocked, on", true, false, IPv6On, false, true, true},
	} {
		all := routes.Build(routes.Plan{}, users)
		if c.lock {
			all = lockedWith(users)
		}
		in := Input{Routes: all, UserRules: users, Mode: "rule", Tun: c.tun, IPv6Usable: c.usable}
		in.Options.IPv6 = c.mode
		ipv6, dnsIPv6, _ := lockDNSConfig(t, in)
		if ipv6 != c.ipv6 || dnsIPv6 != c.dnsIPv6 {
			t.Errorf("%s: ipv6 %v, dns ipv6 %v; want %v, %v", c.name, ipv6, dnsIPv6, c.ipv6, c.dnsIPv6)
		}
	}
	// Android's VPN refuses IPv6 by itself: the lock leaves its IPv6 off.
	defer func(was bool) { ipv6Leaks = was }(ipv6Leaks)
	ipv6Leaks = false
	in := Input{Routes: lockedWith(users), UserRules: users, Mode: "rule", Tun: true}
	in.Options.IPv6 = IPv6Off
	if ipv6, _, _ := lockDNSConfig(t, in); ipv6 {
		t.Error("the lock gave Android's core IPv6")
	}
}

// The real core takes the policy a locked group gets, and the TUN with
// IPv6 while IPv6 is off.
func TestLockedDNSAccepted(t *testing.T) {
	bin, geo := bundledMihomo(t)
	dir := coreDir(t, geo)
	var p routes.Plan
	p.SetLocked("ai", true)
	set := p.AddRuleSet(nil, "ai", "names", "https://x.example/names.yaml")
	users := []routes.UserRule{
		{Kind: routes.KindSuffix, Value: "held.bowel-test.dev", Route: "ai"},
		{Kind: routes.KindDomain, Value: "one.bowel-test.dev", Route: "ai"},
	}
	path := filepath.Join(dir, "names.yaml")
	if err := os.WriteFile(path, []byte("payload:\n  - '+.set.bowel-test.dev'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tun := range []bool{false, true} {
		in := devInput("rule", devnet.Overlay{})
		in.Routes, in.UserRules, in.Tun = withName(routes.Build(p, users), "ai", "全 100%"), users, tun
		in.RuleSets = []RuleSetRef{{ID: set, Path: path, Behavior: "domain"}}
		in.Options.IPv6 = IPv6Off
		in.Options.Nameservers = []string{"https://doh.pub/dns-query", "223.5.5.5", "system"}
		b, err := BuildConfig(in)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "rule-set:"+routes.RuleSetProvider(set)) || !strings.Contains(string(b), "#全 100%25") {
			t.Errorf("tun=%v: the policy isn't there:\n%s", tun, b)
		}
		cfgPath := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(cfgPath, b, 0o600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(bin, "-t", "-d", dir, "-f", cfgPath).CombinedOutput(); err != nil {
			t.Errorf("tun=%v: %v\n%s\n%s", tun, err, out, b)
		}
	}
}

// Through the real core: a locked group's name is asked of the DNS server
// by the group's node; another group's name of the same server directly.
func TestLockedDNSRuns(t *testing.T) {
	bin, geo := bundledMihomo(t)
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))
	defer web.Close()
	node, asked := connectProxy(t)
	dnsAddr, heard := tcpDNS(t)
	ctl, err := FreePort()
	if err != nil {
		t.Fatal(err)
	}
	users := []routes.UserRule{
		{Kind: routes.KindIP, Value: "127.0.0.1/32", Route: "ai"},
		{Kind: routes.KindSuffix, Value: "held.bowel-test.dev", Route: "ai"},
		{Kind: routes.KindSuffix, Value: "free.bowel-test.dev", Route: "media"},
	}
	dir := coreDir(t, geo)
	nodes := fmt.Sprintf("proxies:\n  - {name: hop, type: http, server: 127.0.0.1, port: %d}\n", node)
	if err := os.WriteFile(filepath.Join(dir, "nodes.yaml"), []byte(nodes), 0o600); err != nil {
		t.Fatal(err)
	}
	in := devInput("rule", devnet.Overlay{})
	in.Routes, in.UserRules = lockedWith(users), users
	in.FindProcess, in.TestURL = "off", web.URL+"/"
	in.Providers = []ProviderRef{{ID: "a", Path: "nodes.yaml"}}
	// A wider pattern of the user's keeps the other names; the locked one's,
	// more specific, still goes by the node.
	userAddr, userHeard := tcpDNS(t)
	in.Options.Nameservers = []string{"tcp://" + dnsAddr}
	in.Options.NameserverPolicy = []string{"+.bowel-test.dev=tcp://" + userAddr}
	in.Controller = map[string]any{"external-controller": fmt.Sprintf("127.0.0.1:%d", ctl)}
	port, log := runCore(t, bin, dir, in)

	// The node is in use once TCP by the group is answered: until its test,
	// the locked group's automatic one holds REJECT.
	proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}}
	answered := false
	for i := 0; i < 40 && !answered; i++ {
		if res, err := client.Get(web.URL + "/"); err == nil {
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			answered = string(body) == "ok"
		}
		if !answered {
			time.Sleep(200 * time.Millisecond)
		}
	}
	if !answered {
		t.Fatalf("TCP by the node wasn't answered:\n%s", log.String())
	}
	query := func(name string) {
		res, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/dns/query?name=%s&type=TXT", ctl, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
	}
	query("www.held.bowel-test.dev")
	if heard("www.held.bowel-test.dev") == 0 || asked(dnsAddr) == 0 {
		t.Errorf("the locked name: heard %d, by the node %d\n%s", heard("www.held.bowel-test.dev"), asked(dnsAddr), log.String())
	}
	if userHeard("www.held.bowel-test.dev") != 0 {
		t.Errorf("the locked name was asked of the user's wider pattern's server")
	}
	query("www.free.bowel-test.dev")
	if userHeard("www.free.bowel-test.dev") == 0 {
		t.Errorf("the other name wasn't asked of the user's server\n%s", log.String())
	}
	if asked(userAddr) != 0 {
		t.Errorf("the other name went by the node too")
	}
}

// tcpDNS is a DNS server over TCP that answers every question with no
// records. It returns its address, and how many times a name was asked.
func tcpDNS(t *testing.T) (string, func(name string) int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	var mu sync.Mutex
	names := map[string]int{}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				for {
					var n uint16
					if err := binary.Read(c, binary.BigEndian, &n); err != nil {
						return
					}
					msg := make([]byte, n)
					if _, err := io.ReadFull(c, msg); err != nil || len(msg) < 12 {
						return
					}
					var labels []string
					for i := 12; i < len(msg) && msg[i] != 0; i += int(msg[i]) + 1 {
						if end := i + 1 + int(msg[i]); end <= len(msg) {
							labels = append(labels, string(msg[i+1:end]))
						}
					}
					mu.Lock()
					names[strings.ToLower(strings.Join(labels, "."))]++
					mu.Unlock()
					msg[2] |= 0x80 // a response
					msg[3] = 0x80  // recursion available, no error
					binary.Write(c, binary.BigEndian, n)
					c.Write(msg)
				}
			}()
		}
	}()
	return ln.Addr().String(), func(name string) int {
		mu.Lock()
		defer mu.Unlock()
		return names[name]
	}
}
