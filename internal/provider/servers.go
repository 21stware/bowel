package provider

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Servers maps a provider's node names, as the provider's content has
// them, to their servers' hosts. Nodes it can't read are absent.
func (s *Store) Servers(id string) map[string]string {
	b, err := os.ReadFile(s.Path(id))
	if err != nil {
		return nil
	}
	return parseServers(b)
}

// Names are the names of provider id's nodes as its file has them, those
// without a server of their own (direct, WireGuard's peers) too.
func (s *Store) Names(id string) []string {
	b, err := os.ReadFile(s.Path(id))
	if err != nil {
		return nil
	}
	return parseNames(b)
}

func parseNames(data []byte) []string {
	var doc struct {
		Proxies []struct {
			Name string `yaml:"name"`
		} `yaml:"proxies"`
	}
	var out []string
	if yaml.Unmarshal(data, &doc) == nil && len(doc.Proxies) > 0 {
		for _, p := range doc.Proxies {
			if p.Name != "" {
				out = append(out, p.Name)
			}
		}
		return out
	}
	for name := range parseServers(data) {
		out = append(out, name)
	}
	return out
}

// parseServers reads what mihomo reads: Clash YAML, or node links, perhaps
// in Base64.
func parseServers(data []byte) map[string]string {
	var doc struct {
		Proxies []struct {
			Name   string `yaml:"name"`
			Server string `yaml:"server"`
		} `yaml:"proxies"`
	}
	out := map[string]string{}
	if yaml.Unmarshal(data, &doc) == nil && len(doc.Proxies) > 0 {
		for _, p := range doc.Proxies {
			if p.Server != "" {
				out[p.Name] = p.Server
			}
		}
		return out
	}
	text := strings.TrimSpace(string(data))
	if !strings.Contains(text, "://") {
		if b := decodeB64(text); b != nil {
			text = string(b)
		}
	}
	for _, line := range strings.Split(text, "\n") {
		if name, host, ok := parseLink(strings.TrimSpace(line)); ok {
			out[name] = host
		}
	}
	return out
}

func decodeB64(s string) []byte {
	s = strings.Join(strings.Fields(s), "")
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return b
		}
	}
	return nil
}

func parseLink(line string) (string, string, bool) {
	scheme, rest, ok := strings.Cut(line, "://")
	if !ok {
		return "", "", false
	}
	if strings.EqualFold(scheme, "vmess") {
		return parseVmess(rest)
	}
	rest, frag, _ := strings.Cut(rest, "#")
	name, err := url.PathUnescape(frag)
	if err != nil {
		name = frag
	}
	rest, _, _ = strings.Cut(rest, "?")
	rest, _, _ = strings.Cut(rest, "/")
	if strings.EqualFold(scheme, "ss") && !strings.Contains(rest, "@") {
		// ss://<base64 of method:password@host:port>
		b := decodeB64(rest)
		if b == nil {
			return "", "", false
		}
		rest = string(b)
	}
	host := authorityHost(rest[strings.LastIndex(rest, "@")+1:])
	return name, host, host != ""
}

func parseVmess(b64 string) (string, string, bool) {
	var v struct {
		Name string `json:"ps"`
		Host string `json:"add"`
	}
	if b := decodeB64(b64); b == nil || json.Unmarshal(b, &v) != nil {
		return "", "", false
	}
	return v.Name, v.Host, v.Host != ""
}

// authorityHost is the host of host:port, "" if that isn't what s is. The
// port may be a hopping range such as "443,20000-30000".
func authorityHost(s string) string {
	i := strings.LastIndex(s, ":")
	if i < 0 || !validPort(s[i+1:]) {
		return ""
	}
	host := strings.Trim(s[:i], "[]")
	if net.ParseIP(host) == nil && strings.Contains(host, ":") {
		return ""
	}
	return host
}

func validPort(s string) bool {
	s, _, _ = strings.Cut(s, ",")
	s, _, _ = strings.Cut(s, "-")
	p, err := strconv.Atoi(strings.TrimSpace(s))
	return err == nil && p > 0 && p <= 65535
}
