package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
	"github.com/oboo/moho/internal/sysproxy"
)

// exitTo is an HTTP proxy for a node to point at whose every connection
// comes out at upstream, whatever it was asked for: an exit that reaches
// one site only. It gives its port.
func exitTo(t *testing.T, upstream string) int {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "CONNECT only", http.StatusMethodNotAllowed)
			return
		}
		up, err := net.Dial("tcp", upstream)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer up.Close()
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection established\r\n\r\n")
		go func() { _, _ = io.Copy(up, conn) }()
		_, _ = io.Copy(conn, up)
	}))
	t.Cleanup(srv.Close)
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	n, _ := strconv.Atoi(port)
	return n
}

// TestRefusedBySiteEndToEnd runs the core on a site that tells a refusal by
// its status, as api.openai.com does (issue 260): 401 to an exit it serves
// and 403 to one it doesn't. The node it turns away fails the test as a
// refusal, which a dead node's failure is not; the automatic group does
// not choose it; and a locked route left with nothing else refuses its
// traffic instead of sending it there.
func TestRefusedBySiteEndToEnd(t *testing.T) {
	status := func(code int) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) }))
		t.Cleanup(srv.Close)
		return srv
	}
	serves, refuses := status(http.StatusUnauthorized), status(http.StatusForbidden)
	_, port, _ := net.SplitHostPort(serves.Listener.Addr().String())
	// The name is what makes the site one that refuses by its status; the
	// hosts entry brings it here.
	site := "http://api.openai.com:" + port + "/v1/models"
	if routes.Expected(site) == "" {
		t.Fatal("the test's site expects no status")
	}
	a, _ := routingApp(t, func(s *settings.Settings) {
		s.Hosts = []string{"api.openai.com=127.0.0.1"}
		s.DevOff = true // which would keep loopback for itself
		s.RouteTargets = map[string][]routes.Target{"ai": {{Name: "OpenAI", URL: site}}}
	})
	ctx := context.Background()

	// Two more nodes: one whose exit the site turns away, one that
	// reaches nothing.
	nowhere, err := core.FreePort()
	if err != nil {
		t.Fatal(err)
	}
	sub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "proxies:\n"+
			"  - {name: Blocked, type: http, server: 127.0.0.1, port: "+strconv.Itoa(exitTo(t, refuses.Listener.Addr().String()))+"}\n"+
			"  - {name: Dead, type: http, server: 127.0.0.1, port: "+strconv.Itoa(nowhere)+"}\n")
	}))
	t.Cleanup(sub.Close)
	pv, err := a.addProvider(ctx, sub.URL+"/more")
	if err != nil {
		t.Fatal(err)
	}
	blocked := "Blocked" + core.NodeSep + pv.ID

	read := func() map[string]NodeView {
		t.Helper()
		if err := a.testRoute(ctx, "ai"); err != nil {
			t.Fatal(err)
		}
		a.refresh()
		out := map[string]NodeView{}
		for _, n := range a.snapshot().Nodes {
			out[display(n.Name)] = n.On("ai")
		}
		return out
	}
	nodes := read()
	if len(nodes) != 5 {
		t.Fatalf("nodes: %+v", nodes)
	}
	for name, n := range nodes {
		switch name {
		case "Blocked":
			if !n.Tested || n.Delay != 0 || !n.Refused {
				t.Errorf("the node the site turns away: %+v", n)
			}
		case "Dead":
			if !n.Tested || n.Delay != 0 || n.Refused {
				t.Errorf("the node that reaches nothing: %+v", n)
			}
		default:
			if n.Delay <= 0 || n.Refused {
				t.Errorf("%s, which the site serves: %+v", name, n)
			}
		}
	}
	for _, n := range mcpNodes(a.snapshot(), "ai") {
		if want := n.Name == blocked; n.RefusedBySite != want {
			t.Errorf("agents are told %s is refused: %v", n.Label, n.RefusedBySite)
		}
	}

	// The automatic group chooses among the nodes the site serves.
	r := routeOf(t, a, "ai")
	if now := coreProxies(t, a)[routes.Auto(r)].Now; display(now) == "Blocked" || display(now) == "Dead" || now == "" {
		t.Errorf("the automatic group is on %q", now)
	}

	// Locked, with the refused node alone in its range: REJECT, not that
	// node, which answers and so would do for a group that takes any
	// status. With takeover off the lock holds by the system proxy, which
	// the test only pretends to set (routingApp puts the real one back).
	osProxy.enable = func(string, int, []string) error { return nil }
	osProxy.disable = func() error { return nil }
	osProxy.restore = func(context.Context, sysproxy.Saved) error { return nil }
	if err := a.updateSettings(func(s *settings.Settings) {
		s.Routing.ChangePolicy("ai", func(q *routes.Policy) { q.Nodes = []string{blocked} })
		s.Routing.SetLocked("ai", true)
	}); err != nil {
		t.Fatal(err)
	}
	if n := read()["Blocked"]; !n.Refused {
		t.Fatalf("after the reload: %+v", n)
	}
	r = routeOf(t, a, "ai")
	if now := coreProxies(t, a)[routes.Auto(r)].Now; now != "REJECT" {
		t.Errorf("the locked group, its only node refused, is on %q", now)
	}
	for _, rv := range a.snapshot().Routes {
		if rv.ID == "ai" && (rv.Node != "" || rv.Broken != "scope") { // no usable node in its range
			t.Errorf("the locked route shows node %q, broken %q", rv.Node, rv.Broken)
		}
	}
}
