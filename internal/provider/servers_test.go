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
