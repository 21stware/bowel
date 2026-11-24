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
		if got := mcpClientURL(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func TestMCPTokenCommand(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{}
	getenv := func(k string) string { return env[k] }
	var out, errOut bytes.Buffer

	// Off, with no token yet: says so, and fails.
	if err := printMCPToken(&out, &errOut, dir, getenv, false); err == nil || !strings.Contains(errOut.String(), mcpHTTPEnv) {
		t.Errorf("off: %v %q", err, errOut.String())
	}

	env[mcpHTTPEnv] = ":7900"
	out.Reset()
	if err := printMCPToken(&out, &errOut, dir, getenv, false); err != nil {
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
	_ = printMCPToken(&out, &errOut, dir, getenv, true)
	if out.String() != tok+"\n" {
		t.Errorf("quiet: %q", out.String())
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
