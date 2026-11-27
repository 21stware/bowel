package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestMCPHTTPAddr(t *testing.T) {
	for in, want := range map[string]string{"": "", "7900": ":7900", ":7900": ":7900", "127.0.0.1:7900": "127.0.0.1:7900"} {
		if got := mcpHTTPAddr(func(string) string { return in }); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{":7900": "http://localhost:7900/mcp", "0.0.0.0:7901": "http://localhost:7901/mcp",
		"10.0.0.2:7900": "http://10.0.0.2:7900/mcp", "[::]:7900": "http://localhost:7900/mcp"} {
		if got := mcpClientURL(in, false); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	if got := mcpClientURL("[::]:7900", true); got != "https://localhost:7900/mcp" {
		t.Errorf("with TLS: %q", got)
	}
}

func TestMCPTokenCommand(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{}
	getenv := func(k string) string { return env[k] }
	var out, errOut bytes.Buffer
	plain := mcpTokenOpts{name: "bowel"}

	// Off, with no token yet: says so, and fails.
	if err := printMCPToken(&out, &errOut, dir, getenv, plain); err == nil || !strings.Contains(errOut.String(), mcpHTTPEnv) {
		t.Errorf("off: %v %q", err, errOut.String())
	}

	env[mcpHTTPEnv] = ":7900"
	out.Reset()
	if err := printMCPToken(&out, &errOut, dir, getenv, plain); err != nil {
		t.Fatal(err)
	}
	tok, _, err := mcpHTTPToken(dir, getenv, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), tok+"\n") || !strings.Contains(out.String(),
		`claude mcp add --transport http bowel http://localhost:7900/mcp --header "Authorization: Bearer `+tok+`"`) {
		t.Errorf("output:\n%s", out.String())
	}
	out.Reset()
	_ = printMCPToken(&out, &errOut, dir, getenv, mcpTokenOpts{quiet: true, name: "bowel"})
	if out.String() != tok+"\n" {
		t.Errorf("quiet: %q", out.String())
	}

	// Named, for one of several bowels.
	out.Reset()
	_ = printMCPToken(&out, &errOut, dir, getenv, mcpTokenOpts{name: "bowel-vps"})
	if !strings.Contains(out.String(), "claude mcp add --transport http bowel-vps ") || !strings.Contains(out.String(), `"bowel-vps": {`) {
		t.Errorf("named:\n%s", out.String())
	}

	env[mcpTokenEnv] = "short"
	if _, _, err := mcpHTTPToken(dir, getenv, true); err == nil {
		t.Error("a short BOWEL_MCP_TOKEN was taken")
	}
	env[mcpTokenEnv] = "a-token-of-the-users-own"
	if got, from, _ := mcpHTTPToken(dir, getenv, true); got != env[mcpTokenEnv] || from != mcpTokenEnv {
		t.Errorf("env token: %q from %q", got, from)
	}
}

func TestMCPTokenArgs(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		quiet bool
		name  string
		bad   bool
	}{
		{nil, false, "bowel", false},
		{[]string{"-q"}, true, "bowel", false},
		{[]string{"--name", "bowel-home"}, false, "bowel-home", false},
		{[]string{"--name=bowel-nas", "--quiet"}, true, "bowel-nas", false},
		{[]string{"--name", "two words"}, false, "", true},
		{[]string{"--name"}, false, "", true},
		{[]string{"--json"}, false, "", true},
	} {
		o, err := mcpTokenArgs(tc.args)
		if (err != nil) != tc.bad || !tc.bad && (o.quiet != tc.quiet || o.name != tc.name) {
			t.Errorf("%q: %+v %v", tc.args, o, err)
		}
	}
}
