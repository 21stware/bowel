package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oboo/moho/internal/mcp"
)

// A subscription's URL holds its token: neither the dialog that asks the
// user, nor the error an agent gets, nor the list of agents' changes has
// it whole.
func TestProviderAddKeepsTheTokenOut(t *testing.T) {
	a, _, _ := mcpApp(t)
	// Closed: the download fails with an error that quotes the URL.
	sub := httptest.NewServer(http.NotFoundHandler())
	sub.Close()
	const secret = "s3cr3t-t0ken-abcdef"
	src := sub.URL + "/api/v1/client/" + secret + "?token=" + secret

	var details []string
	saved := askAgent
	askAgent = func(_ context.Context, _, _, detail string) (bool, bool, error) {
		details = append(details, detail)
		return true, false, nil
	}
	t.Cleanup(func() { askAgent = saved })

	_, err := callTool(t, a.mcpServer(), &mcp.Session{}, "provider_add", map[string]any{"source": src})
	if err == nil {
		t.Fatal("a subscription nobody answers for was added")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("the error has the token: %v", err)
	}
	if len(details) != 1 || strings.Contains(details[0], secret) || !strings.Contains(details[0], strings.TrimPrefix(sub.URL, "http://")) {
		t.Errorf("the dialog said %q", details)
	}
	for _, ch := range agentChanges() {
		if strings.Contains(ch.Error, secret) {
			t.Errorf("the list of changes has the token: %q", ch.Error)
		}
	}
	if len(agentChanges()) != 1 || agentChanges()[0].Outcome != "failed" {
		t.Errorf("changes: %+v", agentChanges())
	}
}

// Over HTTP the agent is on another machine: this one's files are not its
// to read into a provider. On the socket they are the user's machine's.
func TestProviderAddFileNotOverHTTP(t *testing.T) {
	a, _, _ := mcpApp(t)
	setPolicy(t, a, "allow")
	remote := &mcp.Session{}
	remote.Set(remoteKey, true)
	if _, err := callTool(t, a.mcpServer(), remote, "provider_add", map[string]any{"file": "/etc/hosts"}); err == nil || !strings.Contains(err.Error(), "HTTP") {
		t.Fatalf("remote file: %v", err)
	}
	if len(agentChanges()) != 0 {
		t.Errorf("a refused file was noted as a change: %+v", agentChanges())
	}
}
