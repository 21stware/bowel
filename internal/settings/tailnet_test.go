package settings

import "testing"

// A self-hosted control server (Headscale) is an http or https link, takes
// keys of its own, and its MagicDNS domain can't hide the server itself.
func TestTailnetValidate(t *testing.T) {
	cases := []struct {
		name string
		tn   Tailnet
		ok   bool
	}{
		{"Tailscale's own", Tailnet{AuthKey: "tskey-auth-x"}, true},
		{"Tailscale's own, other key", Tailnet{AuthKey: "a1b2c3"}, false},
		{"https server", Tailnet{ControlURL: "https://hs.example.com"}, true},
		{"http server with a port", Tailnet{ControlURL: "http://192.168.1.5:8080/"}, true},
		{"no scheme", Tailnet{ControlURL: "hs.example.com"}, false},
		{"other scheme", Tailnet{ControlURL: "ftp://hs.example.com"}, false},
		{"no host", Tailnet{ControlURL: "https://"}, false},
		{"Headscale's hex key", Tailnet{ControlURL: "https://hs.example.com", AuthKey: "0123456789abcdef0123456789abcdef0123456789abcdef"}, true},
		{"Headscale's new key", Tailnet{ControlURL: "https://hs.example.com", AuthKey: "hskey-auth-AbC_123-xyz"}, true},
		{"Tailscale's key on Headscale", Tailnet{ControlURL: "https://hs.example.com", AuthKey: "tskey-auth-x"}, true},
		{"key with a space", Tailnet{ControlURL: "https://hs.example.com", AuthKey: "abc def"}, false},
		{"domain", Tailnet{ControlURL: "https://hs.example.com", Domain: "tail.example.com"}, true},
		{"domain in capitals", Tailnet{ControlURL: "https://hs.example.com", Domain: "Tail.Example.COM"}, true},
		{"single-label domain", Tailnet{ControlURL: "https://hs.example.com", Domain: "tailnet"}, true},
		{"domain with a wildcard", Tailnet{ControlURL: "https://hs.example.com", Domain: "*.tail.example.com"}, false},
		{"domain with a dot at the end", Tailnet{ControlURL: "https://hs.example.com", Domain: "tail.example.com."}, false},
		{"domain with a space", Tailnet{ControlURL: "https://hs.example.com", Domain: "tail example.com"}, false},
		{"domain with a scheme", Tailnet{ControlURL: "https://hs.example.com", Domain: "https://tail.example.com"}, false},
		{"domain above the server", Tailnet{ControlURL: "https://hs.example.com", Domain: "example.com"}, false},
		{"domain of the server", Tailnet{ControlURL: "https://HS.example.com", Domain: "hs.example.com"}, false},
		{"domain beside the server", Tailnet{ControlURL: "https://hs.example.com", Domain: "ts.example.com"}, true},
	}
	for _, c := range cases {
		s := Defaults.Clone()
		s.Tailnet = c.tn
		if err := s.Validate(); (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

// The core puts its paths after the control URL: no slash at its end.
func TestTailnetControl(t *testing.T) {
	for in, want := range map[string]string{
		"":                         "",
		"https://hs.example.com":   "https://hs.example.com",
		"https://hs.example.com/":  "https://hs.example.com",
		"http://10.0.0.2:8080//":   "http://10.0.0.2:8080",
		"https://example.com/hs/":  "https://example.com/hs",
		"https://example.com/hs/x": "https://example.com/hs/x",
	} {
		if got := (Tailnet{ControlURL: in}).Control(); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}
