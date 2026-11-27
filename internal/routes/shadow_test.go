package routes

import (
	"strings"
	"testing"
)

// TestShadowed: a rule all of whose traffic a group above takes first is
// told of, with that group's rule: by a broader rule, a rule set's line or
// the developer networks. #294's list, dragged above AI 服务, takes
// OpenAI's chat.com; below it, it doesn't. Rules a group above takes only
// in part, and the default route's, aren't told of.
func TestShadowed(t *testing.T) {
	var p Plan
	g := p.AddGroup(nil, "国外穿墙")
	gfw := p.AddRuleSet(nil, g, "GFW", "https://x.example/gfw.txt")
	users := []UserRule{
		{Kind: KindSuffix, Value: "api.openai.com", Route: "dev"},
		{Kind: KindDomain, Value: "www.google.com", Route: "direct"},
		{Kind: KindSuffix, Value: "example.net", Route: "direct"},
		{Kind: KindSuffix, Value: "google.com", Route: "direct"},
		{Kind: KindIP, Value: "10.1.0.0/16", Route: "direct"},
		{Kind: KindDomain, Value: "x.example.com", Route: Default},
	}
	match := func(set, host, ip string) (string, bool) {
		if set == gfw {
			for _, d := range []string{"chat.com", "google.com"} {
				if host == d || strings.HasSuffix(host, "."+d) {
					return "+." + d, true
				}
			}
		}
		return "", false
	}
	dev := func(text string) bool {
		return strings.HasPrefix(text, "IP-CIDR,10.1.") || text == "DOMAIN-SUFFIX,github.com" || text == "DOMAIN-SUFFIX,wiki.corp"
	}
	key := func(service, text string) string { return ShadowKey(service, text) }

	below := Shadowed(Build(p, append(users, UserRule{Kind: KindSuffix, Value: "wiki.corp", Route: "ai"})), dev, match)
	if s, ok := below[key("github", "DOMAIN-SUFFIX,github.com")]; !ok || !s.Dev() {
		t.Errorf("GitHub's github.com, the developer networks': %+v %v", s, ok)
	}
	if s, ok := below[key(MineID("ai"), "DOMAIN-SUFFIX,wiki.corp")]; ok {
		t.Errorf("the user's wiki.corp, which goes before the developer networks, is taken by %+v", s)
	}
	if s, ok := below[key("openai", "DOMAIN-SUFFIX,chat.com")]; ok {
		t.Errorf("with the list below AI 服务, chat.com is taken by %+v", s)
	}
	if s := below[key(MineID("dev"), "DOMAIN-SUFFIX,api.openai.com")]; s != (Shadow{Route: "ai", Service: "openai", Rule: "*.openai.com"}) {
		t.Errorf("api.openai.com in 开发: %+v", s)
	}
	if s := below[key(MineID("direct"), "DOMAIN,www.google.com")]; s != (Shadow{Route: g, Service: gfw, Rule: "+.google.com"}) {
		t.Errorf("www.google.com in 直连: %+v", s)
	}
	if s := below[key(MineID("direct"), "DOMAIN-SUFFIX,google.com")]; s != (Shadow{Route: g, Service: gfw, Rule: "+.google.com"}) {
		t.Errorf("*.google.com in 直连: %+v", s)
	}
	for _, k := range []string{key(MineID("direct"), "DOMAIN-SUFFIX,example.net"), key(MineID(Default), "DOMAIN,x.example.com"), key("cn", "GEOSITE,cn"), key(MineID("direct"), "IP-CIDR,10.1.0.0/16,no-resolve")} {
		if s, ok := below[k]; ok {
			t.Errorf("%q is taken by %+v", k, s)
		}
	}

	p.MoveGroup(g, "ai")
	above := Shadowed(Build(p, users), dev, match)
	if s := above[key("openai", "DOMAIN-SUFFIX,chat.com")]; s != (Shadow{Route: g, Service: gfw, Rule: "+.chat.com"}) {
		t.Errorf("with the list above AI 服务, chat.com: %+v", s)
	}
	if s := above[key("gemini", "DOMAIN-SUFFIX,gemini.google.com")]; s.Route != g {
		t.Errorf("gemini.google.com: %+v", s)
	}
}

func TestCovers(t *testing.T) {
	for _, c := range []struct {
		a, b string
		ok   bool
	}{
		{"DOMAIN-SUFFIX,openai.com", "DOMAIN,api.openai.com", true},
		{"DOMAIN-SUFFIX,openai.com", "DOMAIN-SUFFIX,OpenAI.com", true},
		{"DOMAIN-SUFFIX,openai.com", "DOMAIN-SUFFIX,notopenai.com", false},
		{"DOMAIN-SUFFIX,api.openai.com", "DOMAIN-SUFFIX,openai.com", false},
		{"DOMAIN,openai.com", "DOMAIN-SUFFIX,openai.com", false},
		{"DOMAIN-KEYWORD,google", "DOMAIN-SUFFIX,googleapis.com", true},
		{"DOMAIN-KEYWORD,google", "DOMAIN-KEYWORD,googleads", true},
		{"DOMAIN-KEYWORD,googleads", "DOMAIN-KEYWORD,google", false},
		{"IP-CIDR,10.0.0.0/8,no-resolve", "IP-CIDR,10.1.0.0/16,no-resolve", true},
		{"IP-CIDR,10.1.0.0/16,no-resolve", "IP-CIDR,10.0.0.0/8,no-resolve", false},
		{"GEOSITE,cn", "GEOSITE,cn", true},
		{"GEOSITE,cn", "DOMAIN-SUFFIX,baidu.com", false},
		{"PROCESS-NAME,Codex.exe", "PROCESS-NAME,Codex.exe", true},
		{"DOMAIN-SUFFIX,openai.com", "PROCESS-NAME,Codex.exe", false},
	} {
		if _, ok := covers(ruleOf(c.a), ruleOf(c.b)); ok != c.ok {
			t.Errorf("%s covers %s: %v", c.a, c.b, ok)
		}
	}
}
