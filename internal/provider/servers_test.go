package provider

import (
	"encoding/base64"
	"testing"
)

func TestParseServersYAML(t *testing.T) {
	got := parseServers([]byte("mixed-port: 7890\nproxies:\n  - {name: a, type: trojan, server: a.example, port: 443}\n  - {name: b, type: hysteria2, server: 1.2.3.4, port: '8443'}\n  - {name: c, type: ss}\n"))
	if got["a"] != "a.example" || got["b"] != "1.2.3.4" {
		t.Fatalf("got %v", got)
	}
	if _, ok := got["c"]; ok {
		t.Fatal("a node with no server was kept")
	}
}

func TestParseServersLinks(t *testing.T) {
	vmess := base64.StdEncoding.EncodeToString([]byte(`{"ps":"v","add":"v.example","port":443}`))
	vmessStr := base64.StdEncoding.EncodeToString([]byte(`{"ps":"v2","add":"v2.example","port":"8080"}`))
	ss := base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:pw@s.example:8388"))
	userinfo := base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:pw"))
	links := "vmess://" + vmess + "\nvmess://" + vmessStr + "\nss://" + ss + "#old\nss://" + userinfo + "@n.example:1234#new\n" +
		"trojan://pw@t.example:443?sni=x#T%20one\nhysteria2://pw@[2001:db8::1]:443/?insecure=1#six\nhy2://pw@h.example:443,20000-30000#hop\nnonsense\n"
	want := map[string]string{
		"v": "v.example", "v2": "v2.example", "old": "s.example", "new": "n.example",
		"T one": "t.example", "six": "2001:db8::1", "hop": "h.example",
	}
	for _, in := range []string{links, base64.StdEncoding.EncodeToString([]byte(links))} {
		got := parseServers([]byte(in))
		if len(got) != len(want) {
			t.Fatalf("got %v", got)
		}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s = %q, want %q", k, got[k], v)
			}
		}
	}
}

// TestParseNames: nodes with no server of their own still have names.
func TestParseNames(t *testing.T) {
	got := parseNames([]byte("proxies:\n  - {name: a, type: direct}\n  - {name: b, type: trojan, server: b.example}\n"))
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("yaml: %q", got)
	}
	got = parseNames([]byte("trojan://pw@203.0.113.11:443#hk\n"))
	if len(got) != 1 || got[0] != "hk" {
		t.Errorf("links: %q", got)
	}
}

// The entries a subscription lists only to say how much traffic is left,
// when the plan ends or where its website is are no nodes; real nodes,
// named for where they exit, stay (#139).
func TestInfoEntries(t *testing.T) {
	for _, name := range []string{
		"剩余流量：915.41 GB", "套餐到期：长期有效", "官网：tejiajichang.com", "🇨🇳 剩余流量：228.09 GB",
		"距离下次重置剩余：12 天", "过期时间：2026-12-31", "最新网址：example.com", "Traffic: 100 GB / 200 GB",
		"Expire: 2026-12-31", "remaining：3 days", "剩餘流量：12 GB", "官網：example.com", "剩余流量：915.41 GB‹f3a63f",
	} {
		if !IsInfo(name) {
			t.Errorf("%q is a node", name)
		}
	}
	for _, name := range []string{
		"日本高速03|BGP|CUCM", "🇭🇰 香港 03 IPLC", "JP-Tokyo-02", "美国 圣何塞", "新加坡高速01|BGP|CTCUCM",
		"Relay-7 专线", "US Los Angeles", "Expressway HK", "SG Remaining-Capacity 01",
	} {
		if IsInfo(name) {
			t.Errorf("%q is taken for information", name)
		}
	}
	yamlDoc := "proxies:\n  - {name: \"剩余流量：1 GB\", type: vless, server: a.example, port: 443}\n  - {name: hk, type: trojan, server: b.example, port: 443}\n"
	if got := parseNames([]byte(yamlDoc)); len(got) != 1 || got[0] != "hk" {
		t.Errorf("names = %q", got)
	}
	if got := parseServers([]byte(yamlDoc)); len(got) != 1 || got["hk"] != "b.example" {
		t.Errorf("servers = %v", got)
	}
	links := "trojan://pw@203.0.113.11:443#%E5%AE%98%E7%BD%91%EF%BC%9Aexample.com\ntrojan://pw@203.0.113.12:443#hk\n"
	if got := parseNames([]byte(links)); len(got) != 1 || got[0] != "hk" {
		t.Errorf("names from links = %q", got)
	}
}
