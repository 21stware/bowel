package mcp

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const testToken = "0123456789abcdef-test"

// httpClient talks to an endpoint as an agent would.
type httpClient struct {
	t       *testing.T
	url     string
	token   string
	session string
	seq     int
}

func startHTTP(t *testing.T, srv *Server) (*HTTPHandler, *httpClient) {
	t.Helper()
	h := srv.HTTPHandler(testToken)
	ts := httptest.NewServer(h)
	t.Cleanup(func() { h.Close(); ts.Close() })
	return h, &httpClient{t: t, url: ts.URL + HTTPPath, token: testToken}
}

// post sends body, with the session and token the client has.
func (c *httpClient) post(body string, edit func(*http.Request)) (*http.Response, map[string]any) {
	c.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, c.url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if c.session != "" {
		req.Header.Set(sessionHeader, c.session)
	}
	if edit != nil {
		edit(req)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&m)
	return resp, m
}

func (c *httpClient) call(method string, params any) (*http.Response, map[string]any) {
	c.t.Helper()
	c.seq++
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": c.seq, "method": method, "params": params})
	return c.post(string(b), nil)
}

func (c *httpClient) initialize(name string) {
	c.t.Helper()
	resp, m := c.call("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": name, "version": "1"}})
	if resp.StatusCode != http.StatusOK || m["result"] == nil {
		c.t.Fatalf("initialize: %d %v", resp.StatusCode, m)
	}
	c.session = resp.Header.Get(sessionHeader)
	if len(c.session) < 32 {
		c.t.Fatalf("session id %q", c.session)
	}
}

func noteServer() *Server {
	srv := testServer(make(chan struct{}))
	srv.Tools = append(srv.Tools, Tool{Name: "note", Run: func(_ context.Context, s *Session, raw json.RawMessage) (any, error) {
		var p struct{ N string }
		if err := Args(raw, &p); err != nil {
			return nil, err
		}
		was, _ := s.Get("n").(string)
		s.Set("n", p.N)
		return map[string]any{"was": was, "client": s.Name()}, nil
	}})
	return srv
}

func TestHTTPAuth(t *testing.T) {
	_, c := startHTTP(t, noteServer())
	init := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	for _, tok := range []string{"", "wrong-token-but-long-enough"} {
		c.token = tok
		resp, _ := c.post(init, nil)
		if resp.StatusCode != http.StatusUnauthorized || !strings.HasPrefix(resp.Header.Get("WWW-Authenticate"), "Bearer") {
			t.Errorf("token %q: %d %v", tok, resp.StatusCode, resp.Header)
		}
	}
	c.token = testToken
	resp, _ := c.post(init, func(r *http.Request) { r.Header.Set("Authorization", "Basic "+testToken) })
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("not a bearer token: %d", resp.StatusCode)
	}
	if resp, _ := c.post(init, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("right token: %d", resp.StatusCode)
	}
}

func TestHTTPOrigin(t *testing.T) {
	_, c := startHTTP(t, noteServer())
	init := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	resp, _ := c.post(init, func(r *http.Request) { r.Header.Set("Origin", "http://evil.example") })
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("another origin: %d", resp.StatusCode)
	}
	resp, _ = c.post(init, func(r *http.Request) { r.Header.Set("Origin", "http://"+r.URL.Host) })
	if resp.StatusCode != http.StatusOK {
		t.Errorf("its own origin: %d", resp.StatusCode)
	}
}

func TestHTTPFlow(t *testing.T) {
	_, c := startHTTP(t, noteServer())

	// Without a session only initialize is taken.
	if resp, _ := c.call("tools/list", nil); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("tools/list before initialize: %d", resp.StatusCode)
	}
	c.initialize("claude-code")
	if resp, _ := c.post(`{"jsonrpc":"2.0","method":"notifications/initialized"}`, nil); resp.StatusCode != http.StatusAccepted {
		t.Errorf("notification: %d", resp.StatusCode)
	}
	resp, m := c.call("tools/list", nil)
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("tools/list: %d %v", resp.StatusCode, resp.Header)
	}
	if tools := m["result"].(map[string]any)["tools"].([]any); len(tools) != 6 {
		t.Errorf("tools: %v", tools)
	}
	if int(m["id"].(float64)) != c.seq {
		t.Errorf("answer to %v, want %d", m["id"], c.seq)
	}
	_, m = c.call("tools/call", map[string]any{"name": "echo", "arguments": map[string]any{"say": "hi"}})
	if sc := m["result"].(map[string]any)["structuredContent"].(map[string]any); sc["said"] != "hi" || sc["client"] != "claude-code 1" {
		t.Errorf("echo: %v", m)
	}
	if _, m = c.call("nope", nil); m["error"].(map[string]any)["code"].(float64) != codeNoMethod {
		t.Errorf("unknown method: %v", m)
	}
	if resp, _ := c.post(`{not json`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("parse error: %d", resp.StatusCode)
	}
	if resp, _ := c.post(`[{"jsonrpc":"2.0","id":9,"method":"ping"}]`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("batch: %d", resp.StatusCode)
	}

	// A GET stream is not offered.
	req, _ := http.NewRequest(http.MethodGet, c.url, nil)
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set(sessionHeader, c.session)
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET: %v %v", resp, err)
	}

	// An unknown session is gone; DELETE ends this one.
	good := c.session
	c.session = "00000000000000000000000000000000"
	if resp, _ := c.call("ping", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown session: %d", resp.StatusCode)
	}
	c.session = good
	req, _ = http.NewRequest(http.MethodDelete, c.url, nil)
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set(sessionHeader, c.session)
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE: %v %v", resp, err)
	}
	if resp, _ := c.call("ping", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("ended session: %d", resp.StatusCode)
	}
}

func TestHTTPLimits(t *testing.T) {
	_, c := startHTTP(t, noteServer())
	c.initialize("a")
	big := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"x":"` + strings.Repeat("x", maxMessage) + `"}}`
	if resp, _ := c.post(big, nil); resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("too big: %d", resp.StatusCode)
	}
	ver := func(v string) func(*http.Request) { return func(r *http.Request) { r.Header.Set(versionHeader, v) } }
	if resp, _ := c.post(`{"jsonrpc":"2.0","id":2,"method":"ping"}`, ver("1999-01-01")); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("unknown protocol version: %d", resp.StatusCode)
	}
	if resp, _ := c.post(`{"jsonrpc":"2.0","id":3,"method":"ping"}`, ver("2025-06-18")); resp.StatusCode != http.StatusOK {
		t.Errorf("known protocol version: %d", resp.StatusCode)
	}
}

func TestHTTPSessions(t *testing.T) {
	h, a := startHTTP(t, noteServer())
	b := &httpClient{t: t, url: a.url, token: a.token}
	a.initialize("one")
	b.initialize("two")
	note := func(c *httpClient, n string) map[string]any {
		_, m := c.call("tools/call", map[string]any{"name": "note", "arguments": map[string]any{"n": n}})
		return m["result"].(map[string]any)["structuredContent"].(map[string]any)
	}
	note(a, "A")
	if r := note(b, "B"); r["was"] != "" || r["client"] != "two 1" {
		t.Errorf("b saw a's state: %v", r)
	}
	if r := note(a, "A2"); r["was"] != "A" || r["client"] != "one 1" {
		t.Errorf("a lost its state: %v", r)
	}

	// An idle session expires; the cap ends the one unused longest.
	now := time.Now()
	h.now = func() time.Time { return now.Add(2 * sessionIdle) }
	if resp, _ := a.call("ping", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("expired session: %d", resp.StatusCode)
	}
	h.now = time.Now
	first := &httpClient{t: t, url: a.url, token: a.token}
	first.initialize("first")
	for i := 0; i < maxSessions; i++ {
		c := &httpClient{t: t, url: a.url, token: a.token}
		c.initialize("more")
	}
	if resp, _ := first.call("ping", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("oldest session kept past the cap: %d", resp.StatusCode)
	}
	if n := len(h.sessions); n != maxSessions {
		t.Errorf("%d sessions", n)
	}
}

func TestAcceptHTTP(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := noteServer().AcceptHTTP(context.Background(), l, "short"); err == nil {
		t.Error("a short token was taken")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- noteServer().AcceptHTTP(ctx, l, testToken) }()
	c := &httpClient{t: t, url: "http://" + l.Addr().String() + HTTPPath, token: testToken}
	c.initialize("x")
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("AcceptHTTP did not end with its context")
	}
}

func TestToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "mcp-token")
	if _, err := ReadToken(path); err == nil {
		t.Error("read a token that isn't there")
	}
	tok, err := Token(path)
	if err != nil || len(tok) != 43 {
		t.Fatalf("token %q: %v", tok, err)
	}
	if runtime.GOOS != "windows" {
		if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("mode %v: %v", fi.Mode(), err)
		}
	}
	if again, err := Token(path); err != nil || again != tok {
		t.Errorf("not reused: %q %v", again, err)
	}
	if read, err := ReadToken(path); err != nil || read != tok {
		t.Errorf("ReadToken: %q %v", read, err)
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Errorf("left behind: %v", entries)
	}
	_ = os.WriteFile(path, nil, 0o600) // emptied: a new one is made
	if fresh, err := Token(path); err != nil || fresh == tok || len(fresh) != 43 {
		t.Errorf("empty file: %q %v", fresh, err)
	}
}
