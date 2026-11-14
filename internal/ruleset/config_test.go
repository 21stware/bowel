package ruleset

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

const sampleConfig = `mixed-port: 7890
proxies:
  - {name: a, type: ss, server: 192.0.2.1, port: 1, cipher: aes-128-gcm, password: x}
proxy-groups:
  - {name: Proxy, type: select, proxies: [a]}
rule-providers:
  ads: {type: http, behavior: domain, url: https://example.com/ads.yaml, path: ./ads.yaml}
rules:
  - DOMAIN-SUFFIX,openai.com,Proxy
  - DOMAIN,Chat.Example.com,Proxy
  - IP-CIDR,1.12.12.12/32,DIRECT,no-resolve
  - AND,((DOMAIN,baidu.com),(NETWORK,UDP)),DIRECT
  - RULE-SET,ads,REJECT
  - GEOIP,CN,DIRECT
  - USER-AGENT,x,Proxy
  - MATCH,Proxy
`

// A whole config is told apart from a list, and says what it sends where.
func TestParseSaysAConfigIsAConfig(t *testing.T) {
	_, err := Parse([]byte(sampleConfig))
	var cfg *ConfigError
	if !errors.As(err, &cfg) {
		t.Fatalf("err = %v, want a ConfigError", err)
	}
	want := []ConfigTarget{{Name: "Proxy", Count: 2, Skipped: 2}, {Name: "DIRECT", Count: 3}}
	if !slices.Equal(cfg.Targets, want) || cfg.Skipped != 3 {
		t.Fatalf("targets = %+v, skipped %d; want %+v, 3", cfg.Targets, cfg.Skipped, want)
	}
}

// A config with nodes and no rules is still a config, not an empty list.
func TestParseConfigWithoutRules(t *testing.T) {
	_, err := Parse([]byte("proxies:\n  - {name: a, type: ss, server: 192.0.2.1, port: 1, cipher: aes-128-gcm, password: x}\n"))
	var cfg *ConfigError
	if !errors.As(err, &cfg) || len(cfg.Targets) != 0 {
		t.Fatalf("err = %v, want a ConfigError with no targets", err)
	}
}

// A source that names a target gives that target's rules, without it.
func TestFetchTakesOneTargetOfAConfig(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sub.yaml")
	if err := os.WriteFile(file, []byte(sampleConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	c, err := Fetch(ctx, nil, WithTarget(file, "DIRECT"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"IP-CIDR,1.12.12.12/32,no-resolve", "AND,((DOMAIN,baidu.com),(NETWORK,UDP))", "GEOIP,CN"}
	if c.Set.Behavior != Classical || !slices.Equal(c.Set.Rules, want) || c.Set.Skipped != 0 {
		t.Fatalf("DIRECT = %+v, want %v", c.Set, want)
	}
	c, err = Fetch(ctx, nil, WithTarget(file, "Proxy"))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.Set.Rules, []string{"DOMAIN-SUFFIX,openai.com", "DOMAIN,chat.example.com"}) || c.Set.Skipped != 2 {
		t.Fatalf("Proxy = %+v", c.Set)
	}
	if c.Name() != "Proxy" {
		t.Fatalf("name = %q, want the target", c.Name())
	}
	for _, target := range []string{"REJECT", "Nowhere"} {
		if c, err := Fetch(ctx, nil, WithTarget(file, target)); err == nil {
			t.Errorf("%s: read as %+v", target, c.Set)
		}
	}
	list := filepath.Join(t.TempDir(), "list.yaml")
	if err := os.WriteFile(list, []byte("payload:\n  - DOMAIN-SUFFIX,doh.pub\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if c, err := Fetch(ctx, nil, WithTarget(list, "DIRECT")); err == nil {
		t.Errorf("a list has no targets: read as %+v", c.Set)
	}
}

// Importing a logic rule must not retain references to the source config.
func TestConfigSkipsNestedDependencies(t *testing.T) {
	for _, rule := range []string{
		"NOT,((RULE-SET,cn)),Proxy",
		"AND,((DOMAIN,example.com),(RULE-SET,ads)),Proxy",
		"OR,((DOMAIN,example.com),(NOT,((SUB-RULE,(NETWORK,TCP),other)))),Proxy",
		"NOT,((MATCH)),Proxy",
	} {
		data := []byte("rules:\n  - DOMAIN,kept.example,Proxy\n  - " + rule + "\n")
		s, err := parseTarget(data, "Proxy")
		if err != nil || !slices.Equal(s.Rules, []string{"DOMAIN,kept.example"}) || s.Skipped != 1 {
			t.Errorf("%s: %+v, %v", rule, s, err)
		}
		bare, _ := splitRule(rule)
		list, err := Parse([]byte("DOMAIN,kept.example\n" + bare + "\n"))
		if err != nil || len(list.Rules) != 1 || list.Skipped != 1 {
			t.Errorf("list %s: %+v, %v", bare, list, err)
		}
	}
	good := "NOT,((DOMAIN,match.example))"
	if got, ok := classicalRule(good); !ok || got != good {
		t.Fatalf("self-contained logic rule = %q, %t", got, ok)
	}
}

// A source-address rule cannot silently become a destination-address rule.
func TestConfigKeepsSourceAddressRules(t *testing.T) {
	s, err := parseTarget([]byte("rules:\n  - IP-CIDR,192.168.1.50/32,Proxy,src,no-resolve\n  - DOMAIN,kept.example,Proxy\n"), "Proxy")
	want := []string{"IP-CIDR,192.168.1.50/32,no-resolve,src", "DOMAIN,kept.example"}
	if err != nil || !slices.Equal(s.Rules, want) || s.Skipped != 0 {
		t.Fatalf("source-address rules = %+v, %v", s, err)
	}
	m := newMatcher(s)
	if r, ok := m.match("", "192.168.1.50"); ok {
		t.Fatalf("a destination address matched the source rule %s", r)
	}
	if _, ok := m.match("kept.example", ""); !ok {
		t.Fatal("the ordinary destination rule stopped matching")
	}
}

func TestTarget(t *testing.T) {
	for source, want := range map[string][2]string{
		"https://x.example/a.yaml":                 {"https://x.example/a.yaml", ""},
		"https://x.example/sub?token=1#rules=节点 A": {"https://x.example/sub?token=1", "节点 A"},
		"/tmp/a.yaml#rules=DIRECT":                 {"/tmp/a.yaml", "DIRECT"},
	} {
		loc, target := Target(source)
		if loc != want[0] || target != want[1] {
			t.Errorf("%s = %q %q", source, loc, target)
		}
		if got := WithTarget(loc, target); got != source {
			t.Errorf("WithTarget(%q, %q) = %q", loc, target, got)
		}
	}
}
