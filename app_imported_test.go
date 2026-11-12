package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/devnet"
	"github.com/oboo/moho/internal/mcp"
	"github.com/oboo/moho/internal/mihomo"
	"github.com/oboo/moho/internal/provider"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

// issue33Sub is the example of issue #33, with nodes that really connect
// and a rule a test can reach without the internet.
const issue33Sub = `proxies:
  - {name: JP-01, type: direct}
  - {name: JP-02, type: direct}
proxy-groups:
  - name: Japan
    type: url-test
    proxies: [JP-01, JP-02]
    url: http://127.0.0.1:9/
    interval: 300
  - name: Streaming
    type: select
    proxies: [Japan, DIRECT]
rules:
  - DOMAIN-SUFFIX,netflix.com,Streaming
  - MATCH,Japan
`

// importedApp is an app on a real core with a subscription whose body the
// returned function replaces.
func importedApp(t *testing.T) (*App, string, func(string)) {
	t.Helper()
	a, _ := routingApp(t, func(s *settings.Settings) {
		s.Rules = []routes.UserRule{{Kind: routes.KindDomain, Value: "mine.example", Route: "ai"}}
		s.Hosts = []string{"netflix.com=127.0.0.1"} // the example's rule, without the internet
		s.DevOff = true                             // which would keep loopback for itself
	})
	var mu sync.Mutex
	body := issue33Sub
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	v, err := a.addProvider(context.Background(), srv.URL+"/full")
	if err != nil {
		t.Fatal(err)
	}
	return a, v.ID, func(s string) {
		mu.Lock()
		defer mu.Unlock()
		body = s
	}
}

func hasRoute(a *App, name string) bool {
	return slices.ContainsFunc(a.routes(), func(r routes.Route) bool { return r.Name == name })
}

func TestImportedRoutingEndToEnd(t *testing.T) {
	a, id, serve := importedApp(t)
	ctx := context.Background()

	// Bringing groups changes nothing until asked for, updates included.
	pv := a.providerView(mustProvider(t, a, id))
	if pv.Routing.Groups != 2 || pv.Routing.Rules != 2 || pv.Routing.InUse {
		t.Fatalf("routing = %+v", pv.Routing)
	}
	if err := a.updateProvider(ctx, id); err != nil {
		t.Fatal(err)
	}
	if hasRoute(a, "Japan") || !hasRoute(a, "AI 服务") {
		t.Fatalf("a subscription's update changed the routing: %v", a.routes())
	}

	if err := a.setImportedRouting(id); err != nil {
		t.Fatal(err)
	}
	a.refresh()

	// The groups are as written, a group inside a group.
	px := coreProxies(t, a)
	if got := px["Streaming"].All; !slices.Equal(got, []string{"Japan", "DIRECT"}) {
		t.Errorf("Streaming = %v", got)
	}
	if got := px["Japan"].All; !slices.Equal(got, []string{"JP-01", "JP-02"}) {
		t.Errorf("Japan = %v", got)
	}
	if px["Japan"].Type != "URLTest" || px["Streaming"].Type != "Selector" {
		t.Errorf("types %s %s", px["Japan"].Type, px["Streaming"].Type)
	}

	// Routes are the groups rules point at; MATCH's is the default.
	if d := a.route(routes.Default); d.Name != "Japan" || d.Direct {
		t.Errorf("default = %+v", d)
	}
	snap := a.snapshot()
	if snap.Imported == nil || len(snap.Imported.Groups) != 2 || snap.Imported.PausedRules != 1 {
		t.Fatalf("imported = %+v", snap.Imported)
	}
	if snap.RuleCount != 2 {
		t.Errorf("rule count = %d", snap.RuleCount)
	}
	if snap.Exit == nil || !strings.HasPrefix(snap.Exit.Node, "JP-0") || providerOf(snap.Exit.Node) != id {
		t.Errorf("the exit is not a listed node: %+v", snap.Exit)
	}
	sid := a.routeOfGroup("Streaming", "x")
	if sid == "" || sid == routes.Default {
		t.Errorf("Streaming's route = %q", sid)
	}

	// A select group changes; another kind doesn't.
	if err := a.selectMember("Streaming", "DIRECT"); err != nil {
		t.Fatal(err)
	}
	if now := coreProxies(t, a)["Streaming"].Now; now != "DIRECT" {
		t.Errorf("Streaming now = %s", now)
	}
	if err := a.selectMember("Japan", "JP-02"); err == nil {
		t.Error("the core chooses in a url-test group")
	}

	// The rule hits the group.
	s := a.settings.Get()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var snapc mihomo.Snapshot
		_ = a.coreCall(5e9, func(ctx context.Context, c *mihomo.Client) (err error) { snapc, err = c.Connections(ctx); return err })
		b, _ := json.Marshal(snapc)
		_, _ = w.Write(b)
	}))
	defer target.Close()
	pu, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(s.MixedPort))
	hc := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(pu), DisableKeepAlives: true}}
	resp, err := hc.Get("http://netflix.com:" + target.URL[strings.LastIndex(target.URL, ":")+1:])
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got mihomo.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(got.Connections, func(c mihomo.Connection) bool {
		return c.Rule == "DomainSuffix" && c.RulePayload == "netflix.com" && slices.Contains(c.Chains, "Streaming") && c.Chains[0] == "DIRECT"
	}) {
		t.Errorf("no connection through the group: %+v", got.Connections)
	}

	// The user's rules wait.
	if _, err := a.addRule(routes.UserRule{Kind: routes.KindDomain, Value: "x.example", Route: routes.Default}); err == nil {
		t.Error("a rule can't take effect now and must not be taken")
	}

	// An update that would break the routing leaves it as it was.
	before, _ := os.ReadFile(a.providers.Path(id))
	for name, bad := range map[string]string{
		"only nodes": "proxies:\n  - {name: JP-01, type: direct}\n",
		"dangling":   strings.Replace(issue33Sub, "[Japan, DIRECT]", "[Gone, DIRECT]", 1),
		"no match":   strings.Replace(issue33Sub, "MATCH,Japan", "MATCH,Gone", 1),
	} {
		serve(bad)
		if err := a.updateProvider(ctx, id); err == nil {
			t.Errorf("%s: applied", name)
		}
		if now, _ := os.ReadFile(a.providers.Path(id)); string(now) != string(before) {
			t.Errorf("%s: the last good file was replaced", name)
		}
		if p := mustProvider(t, a, id); p.Error == "" {
			t.Errorf("%s: nothing says so", name)
		}
		if !hasRoute(a, "Japan") || hasRoute(a, "AI 服务") {
			t.Errorf("%s: the routing changed", name)
		}
	}
	if is := a.importedIssue(a.settings.Get(), a.snapshot()); is == nil || !strings.Contains(is.Text, "没有应用") {
		t.Errorf("issue = %+v", is)
	}

	// A good one replaces the groups; the choice in a group stays.
	serve(strings.Replace(issue33Sub, "rules:", "  - {name: Extra, type: select, proxies: [JP-02]}\nrules:", 1))
	if err := a.updateProvider(ctx, id); err != nil {
		t.Fatal(err)
	}
	if p := mustProvider(t, a, id); p.Error != "" {
		t.Errorf("error = %s", p.Error)
	}
	px = coreProxies(t, a)
	if _, ok := px["Extra"]; !ok {
		t.Error("the update's new group is missing")
	}
	if px["Streaming"].Now != "DIRECT" {
		t.Errorf("the choice was lost: %s", px["Streaming"].Now)
	}

	// Disabled, the template takes over; enabled, the source again.
	if err := a.setProviderEnabled(id, false); err != nil {
		t.Fatal(err)
	}
	if hasRoute(a, "Japan") || !hasRoute(a, "AI 服务") {
		t.Errorf("disabled: %v", a.routes())
	}
	if err := a.setProviderEnabled(id, true); err != nil {
		t.Fatal(err)
	}
	if !hasRoute(a, "Japan") {
		t.Error("enabled: the routing didn't come back")
	}

	// Back to bowel's, with the user's rules where they were.
	if err := a.setImportedRouting(""); err != nil {
		t.Fatal(err)
	}
	if hasRoute(a, "Japan") || len(a.settings.Get().Rules) != 1 {
		t.Errorf("back: %v, %v", a.routes(), a.settings.Get().Rules)
	}
	if _, ok := coreProxies(t, a)["Japan"]; ok {
		t.Error("the source's groups stayed in the core")
	}

	// Deleting the source ends its use for good.
	if err := a.setImportedRouting(id); err != nil {
		t.Fatal(err)
	}
	if err := a.deleteProvider(id); err != nil {
		t.Fatal(err)
	}
	if a.settings.Get().ImportedRouting != "" || hasRoute(a, "Japan") {
		t.Errorf("deleted: %q", a.settings.Get().ImportedRouting)
	}
}

// TestImportedConnectionNames: list_connections names a connection's route
// and rule as the window's list does, not only by the route's id, which here
// is g1, g2 and so on.
func TestImportedConnectionNames(t *testing.T) {
	a, id, _ := importedApp(t)
	if err := a.setImportedRouting(id); err != nil {
		t.Fatal(err)
	}
	a.refresh()
	a.winShown.Store(true) // the monitor polls as with the window shown
	a.mon.Wake()
	// Open long enough for the monitor to see it.
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(3 * time.Second) }))
	defer target.Close()
	pu, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(a.settings.Get().MixedPort))
	hc := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(pu), DisableKeepAlives: true}, Timeout: 10 * time.Second}
	go func() {
		if resp, err := hc.Get("http://netflix.com:" + target.URL[strings.LastIndex(target.URL, ":")+1:]); err == nil {
			resp.Body.Close()
		}
	}()

	srv, s := a.mcpServer(), &mcp.Session{}
	var got map[string]any
	for i := 0; i < 50 && got == nil; i++ {
		time.Sleep(200 * time.Millisecond)
		out, err := callTool(t, srv, s, "list_connections", map[string]any{"filter": "netflix.com"})
		if err != nil {
			t.Fatal(err)
		}
		if cs, _ := out["connections"].([]any); len(cs) > 0 {
			got, _ = cs[0].(map[string]any)
		}
	}
	if got == nil {
		t.Fatalf("connection not seen: %+v", a.connections())
	}
	if got["route"] != a.routeOfGroup("Streaming", "x") || got["routeName"] != "Streaming" {
		t.Errorf("route %v, routeName %v", got["route"], got["routeName"])
	}
	if got["ruleName"] != "DomainSuffix(netflix.com)" || got["rule"] != "DomainSuffix" {
		t.Errorf("rule %v, ruleName %v", got["rule"], got["ruleName"])
	}
}

// TestImportedUnusedSettings: what of the config's own settings acts on
// traffic and is not used comes with the routing in use, to the window and
// to agents, and follows the config's updates.
func TestImportedUnusedSettings(t *testing.T) {
	a, id, serve := importedApp(t)
	if err := a.setImportedRouting(id); err != nil {
		t.Fatal(err)
	}
	a.refresh()
	if got := a.snapshot().Imported.Unused; len(got) != 0 {
		t.Errorf("issue33 has nothing unused: %v", got)
	}
	serve(issue33Sub + "dns: {enable: true, nameserver-policy: {+.ts.net: 100.100.100.100}}\nlisteners:\n  - {name: hk, type: mixed, port: 7901, proxy: Streaming}\nlog-level: debug\n")
	if err := a.updateProvider(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	a.refresh()
	if got := a.snapshot().Imported.Unused; !slices.Equal(got, []string{"dns", "listeners"}) {
		t.Errorf("unused = %v", got)
	}
	out, err := callTool(t, a.mcpServer(), &mcp.Session{}, "list_routes", nil)
	if err != nil {
		t.Fatal(err)
	}
	im, _ := out["imported"].(map[string]any)
	if got, _ := im["unusedSettings"].([]any); len(got) != 2 || got[0] != "dns" || got[1] != "listeners" {
		t.Errorf("list_routes: %v", im["unusedSettings"])
	}
}

// TestImportedDNSFill: the config's DNS is not used, and can be copied into
// the settings once. What the config lacks stays, the result is one the core
// takes with TUN and the gateway on, and a later update changes nothing.
func TestImportedDNSFill(t *testing.T) {
	a, id, serve := importedApp(t)
	ctx := context.Background()
	if err := a.setImportedRouting(id); err != nil {
		t.Fatal(err)
	}
	a.refresh()
	if im := a.snapshot().Imported; len(im.DNSFill) != 0 {
		t.Errorf("issue33 has no DNS to copy: %v", im.DNSFill)
	}
	if took, err := a.fillImportedDNS(); err != nil || len(took) != 0 {
		t.Errorf("nothing to copy: %v, %v", took, err)
	}
	const dns = `dns:
  enable: true
  enhanced-mode: redir-host
  listen: 0.0.0.0:5353
  nameserver: [223.5.5.5, 119.29.29.29]
  nameserver-policy: {"+.corp.example": [10.0.0.2, 10.0.0.3], "rule-set:mine": 1.2.3.4}
  fake-ip-filter: ["+.corp.example"]
hosts: {nas.home: 192.168.1.9}
`
	serve(issue33Sub + dns)
	if err := a.updateProvider(ctx, id); err != nil {
		t.Fatal(err)
	}
	a.refresh()
	before := a.settings.Get()
	im := a.snapshot().Imported
	if want := []string{"nameserver", "nameserver-policy", "fake-ip-filter", "hosts"}; !slices.Equal(im.DNSFill, want) {
		t.Errorf("to copy = %v, want %v", im.DNSFill, want)
	}
	if !slices.Equal(im.DNSSkipped, []string{"nameserver-policy: rule-set:mine"}) {
		t.Errorf("skipped = %v", im.DNSSkipped)
	}
	if got := a.settings.Get(); !slices.Equal(got.Nameservers, before.Nameservers) {
		t.Error("the update changed the settings by itself")
	}
	took, err := a.fillImportedDNS()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(took, im.DNSFill) {
		t.Errorf("took %v", took)
	}
	s := a.settings.Get()
	if !slices.Equal(s.Nameservers, []string{"223.5.5.5", "119.29.29.29"}) ||
		!slices.Equal(s.NameserverPolicy, []string{"+.corp.example=10.0.0.2", "+.corp.example=10.0.0.3"}) ||
		!slices.Equal(s.FakeIPFilter, []string{"+.corp.example"}) || !slices.Equal(s.Hosts, []string{"nas.home=192.168.1.9"}) {
		t.Errorf("settings: %v %v %v %v", s.Nameservers, s.NameserverPolicy, s.FakeIPFilter, s.Hosts)
	}
	// Not the config's to say: the mode, and the lists it has nothing of.
	if s.DNSMode != before.DNSMode || !slices.Equal(s.DefaultNameservers, before.DefaultNameservers) || !slices.Equal(s.Fallback, before.Fallback) || s.DNS != before.DNS {
		t.Errorf("more than the config's lists changed: %+v", s.Options)
	}
	a.refresh()
	if im := a.snapshot().Imported; len(im.DNSFill) != 0 || len(im.DNSSkipped) != 0 {
		t.Errorf("still offered after copying: %v %v", im.DNSFill, im.DNSSkipped)
	}
	// The core takes it as it does any settings, whichever way traffic is
	// taken over and with the developer networks' own DNS beside it. Built
	// here, since TUN is not installed where the tests run.
	dev := devnet.Overlay{
		Rules:     []devnet.Rule{{Text: "IP-CIDR,10.0.0.0/8,no-resolve", Target: "DIRECT"}},
		Policy:    []devnet.Policy{{Pattern: "+.corp.example", Server: "10.9.9.9"}, {Pattern: "+.ts.net", Server: "100.100.100.100"}},
		SystemDNS: []string{"+.vpn.example"}, FakeIPFilter: []string{"+.ts.net"},
	}
	for _, c := range []struct{ dns, tun, gateway bool }{{}, {dns: true}, {tun: true}, {tun: true, gateway: true}} {
		in := core.Input{Mode: "rule", MixedPort: s.MixedPort, LogLevel: "info", TestURL: "http://x/", ProbePort: a.probePort,
			Tun: c.tun, Gateway: c.gateway, Dev: dev, Imported: a.importedNow(), Options: s.Options.Clone()}
		in.Options.DNS = c.dns
		cfg, err := core.BuildConfig(in)
		if err != nil {
			t.Fatal(err)
		}
		if err := a.core.Check(cfg); err != nil {
			t.Errorf("%+v: %v\n%s", c, err, cfg)
		}
		// The user's servers for a pattern are kept; the developer
		// networks' are for the patterns the user says nothing of.
		var got struct {
			DNS struct {
				Mode   string              `yaml:"enhanced-mode"`
				Policy map[string][]string `yaml:"nameserver-policy"`
				Filter []string            `yaml:"fake-ip-filter"`
			} `yaml:"dns"`
		}
		if err := yaml.Unmarshal(cfg, &got); err != nil {
			t.Fatal(err)
		}
		pol := got.DNS.Policy
		if !slices.Equal(pol["+.corp.example"], []string{"10.0.0.2", "10.0.0.3"}) || !slices.Equal(pol["+.ts.net"], []string{"100.100.100.100"}) {
			t.Errorf("%+v: policy %v", c, pol)
		}
		if c.tun && !slices.Equal(pol["+.vpn.example"], []string{"system"}) {
			t.Errorf("%+v: no system DNS for the VPN's names: %v", c, pol)
		}
		if c.gateway && (got.DNS.Mode != "fake-ip" || !slices.Contains(got.DNS.Filter, "+.ts.net") || !slices.Contains(got.DNS.Filter, "+.corp.example")) {
			t.Errorf("gateway: %s %v", got.DNS.Mode, got.DNS.Filter)
		}
	}
	// A later update of the config leaves the settings alone, and offers
	// what is new in it.
	serve(issue33Sub + strings.Replace(dns, "223.5.5.5, 119.29.29.29", "9.9.9.9", 1))
	if err := a.updateProvider(ctx, id); err != nil {
		t.Fatal(err)
	}
	a.refresh()
	if got := a.settings.Get().Nameservers; !slices.Equal(got, []string{"223.5.5.5", "119.29.29.29"}) {
		t.Errorf("the update changed the servers: %v", got)
	}
	if im := a.snapshot().Imported; !slices.Equal(im.DNSFill, []string{"nameserver"}) {
		t.Errorf("after the update, to copy = %v", im.DNSFill)
	}
	// Back on bowel's routing there is no config to copy from.
	if err := a.setImportedRouting(""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.fillImportedDNS(); err == nil {
		t.Error("copied with no config in use")
	}
}

// TestImportedGlobalAndDirect: the modes override the source's rules as they
// do the route groups'.
func TestImportedGlobalAndDirect(t *testing.T) {
	a, id, _ := importedApp(t)
	if err := a.setImportedRouting(id); err != nil {
		t.Fatal(err)
	}
	if err := a.setMode("global"); err != nil {
		t.Fatal(err)
	}
	if now := coreProxies(t, a)["GLOBAL"].Now; now != "Japan" {
		t.Errorf("global = %s", now)
	}
	if err := a.setMode("direct"); err != nil {
		t.Fatal(err)
	}
	if a.snapshot().Mode != "direct" {
		t.Error("mode")
	}
}

func TestImportedRefusedWithAReason(t *testing.T) {
	a, _ := routingApp(t, nil)
	v, err := a.addProvider(context.Background(), "proxies:\n  - {name: A, type: direct}\nrule-providers:\n  r: {type: file, path: ./r.yaml, behavior: domain}\nproxy-groups:\n  - {name: G, type: select, proxies: [A]}\nrules:\n  - RULE-SET,r,G\n  - MATCH,G\n")
	if err != nil {
		t.Fatal(err)
	}
	if v.Routing.Problem == "" || v.Routing.Groups != 0 {
		t.Errorf("routing = %+v", v.Routing)
	}
	if err := a.setImportedRouting(v.ID); err == nil || !strings.Contains(err.Error(), "本地文件") {
		t.Errorf("err = %v", err)
	}
	if a.settings.Get().ImportedRouting != "" {
		t.Error("stayed set")
	}
}

func TestImportedRoutesOfDirectDefault(t *testing.T) {
	a, _ := routingApp(t, nil)
	v, err := a.addProvider(context.Background(), "proxies:\n  - {name: A, type: direct}\nproxy-groups:\n  - {name: G, type: select, proxies: [A, DIRECT]}\nrules:\n  - DOMAIN,a.example,G\n  - DOMAIN,b.example,REJECT\n  - MATCH,DIRECT\n")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.setImportedRouting(v.ID); err != nil {
		t.Fatal(err)
	}
	all := a.routes()
	var ids []string
	for _, r := range all {
		ids = append(ids, r.ID)
	}
	if !slices.Equal(ids, []string{routes.Default, "g1", routes.Reject}) {
		t.Fatalf("routes = %v", ids)
	}
	if d := a.route(routes.Default); !d.Direct || routes.Group(d) != "DIRECT" {
		t.Errorf("default = %+v", d)
	}
	a.refresh()
	if ex := a.snapshot().Exit; ex != nil {
		t.Errorf("exit = %+v", ex)
	}
}

func mustProvider(t *testing.T, a *App, id string) provider.Provider {
	t.Helper()
	p, ok := a.providers.Get(id)
	if !ok {
		t.Fatalf("no provider %s", id)
	}
	return p
}

// issue75Config has a group that takes its nodes from a local file, as the
// machine that wrote the config knew it.
const issue75Config = `proxy-providers:
  nodes: {type: file, path: /etc/passwd, health-check: {enable: false}}
proxy-groups:
  - {name: G, type: select, use: [nodes]}
rules:
  - MATCH,G
`

// TestImportedLocalFileProvider is issue #75: a config's `type: file`
// provider is the local file Provider of the same name, whose copy bowel
// keeps; the path in the config is never read.
func TestImportedLocalFileProvider(t *testing.T) {
	a, _ := routingApp(t, nil)
	ctx := context.Background()
	v, err := a.addProvider(ctx, "proxies:\n"+issue75Config)
	if err != nil {
		t.Fatal(err)
	}
	if v.Routing.NeedsFile != "nodes" || v.Routing.Groups != 0 || !strings.Contains(v.Routing.Problem, "nodes") {
		t.Fatalf("routing = %+v", v.Routing)
	}
	if err := a.setImportedRouting(v.ID); err == nil {
		t.Fatal("used without its file")
	}

	file := t.TempDir() + "/whatever.yaml"
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("proxies:\n  - {name: N1, type: direct}\n")
	if err := a.addFileAs(file, "nodes"); err != nil {
		t.Fatal(err)
	}
	var local provider.Provider
	for _, p := range a.providers.List() {
		if p.Name == "nodes" {
			local = p
		}
	}
	if local.Kind != provider.File || local.Source != file {
		t.Fatalf("the file's provider = %+v", local)
	}
	if r := a.providerView(mustProvider(t, a, v.ID)).Routing; r.Groups != 1 || r.Problem != "" || r.NeedsFile != "" {
		t.Fatalf("with its file: %+v", r)
	}
	if err := a.setImportedRouting(v.ID); err != nil {
		t.Fatal(err)
	}
	s := a.settings.Get()
	cfg, err := a.buildConfig(s, overlayOf(s, a.devEnv()), a.importedNow())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cfg), "/etc/passwd") || strings.Count(string(cfg), a.providers.Path(local.ID)) != 2 {
		t.Errorf("the config reads the wrong file:\n%s", cfg)
	}
	if got := coreProxies(t, a)["G"].All; !slices.Equal(got, []string{"N1"}) {
		t.Errorf("G = %v", got)
	}

	// 重新读取 brings what the file holds now.
	write("proxies:\n  - {name: N1, type: direct}\n  - {name: N2, type: direct}\n")
	if err := a.reloadFile(local.ID, ""); err != nil {
		t.Fatal(err)
	}
	if got := coreProxies(t, a)["G"].All; !slices.Equal(got, []string{"N1", "N2"}) {
		t.Errorf("G after the reload = %v", got)
	}

	// An update that needs a file there is none for is not applied.
	more := strings.Replace(issue75Config, "use: [nodes]", "use: [nodes, more]", 1)
	more = "proxies:\n" + strings.Replace(more, "proxy-groups:", "  more: {type: file, path: ./more.yaml}\nproxy-groups:", 1)
	if err := a.vetUpdate(v.ID, []byte(more)); err == nil || !strings.Contains(err.Error(), "more") {
		t.Errorf("vetUpdate = %v", err)
	}

	// Under another name the file is no longer the config's: bowel's own
	// routing takes over, with the reason, until the name is back.
	if err := a.renameProvider(local.ID, "elsewhere"); err != nil {
		t.Fatal(err)
	}
	if a.importedNow() != nil || !hasRoute(a, "AI 服务") {
		t.Error("the config is still in use without its file")
	}
	a.refresh()
	if is := a.importedIssue(a.settings.Get(), a.snapshot()); is == nil || !strings.Contains(is.Detail, "nodes") {
		t.Errorf("issue = %+v", is)
	}
	if err := a.renameProvider(local.ID, "nodes"); err != nil {
		t.Fatal(err)
	}
	if a.importedNow() == nil || !hasRoute(a, "G") {
		t.Error("the config did not come back with its file")
	}
}
