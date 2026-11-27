package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/oboo/moho/internal/cli"
	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/mcp"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

// groupsApp is cliApp on a real core, on a port of its own, with three
// nodes that connect, of a provider called Sub. site is a page they reach,
// to be a main test site.
func groupsApp(t *testing.T) (a *App, asks *int, run func(agent string, args ...string) (int, string, string), node func(string) string, site string) {
	t.Helper()
	if testing.Short() {
		t.Skip("starts the core")
	}
	a, asks, run = cliApp(t)
	if !exists(bundledCore()) {
		t.Skip("no core; run scripts/fetch-core.sh")
	}
	sub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "proxies:\n  - {name: JP1, type: direct}\n  - {name: JP2, type: direct}\n  - {name: US1, type: direct}\n")
	}))
	t.Cleanup(sub.Close)
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(page.Close)
	a.http = sub.Client()
	port, err := core.FreePort()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.settings.Update(func(s *settings.Settings) { s.MixedPort, s.Language = port, "en" }); err != nil {
		t.Fatal(err)
	}
	i18n.Set("en")
	a.copyGeo()
	if err := a.applyConfig(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(a.shutdown)
	v, err := a.addProvider(context.Background(), sub.URL+"/s")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.providers.Rename(v.ID, "Sub"); err != nil {
		t.Fatal(err)
	}
	a.refresh()
	if n := len(a.snapshot().Nodes); n != 3 {
		t.Fatalf("%d nodes", n)
	}
	return a, asks, run, func(label string) string { return label + core.NodeSep + v.ID }, page.URL
}

// groupJSON is list_route_groups's answer for one group, as the command
// line prints it with --json.
func groupJSON(t *testing.T, run func(agent string, args ...string) (int, string, string), id string) map[string]any {
	t.Helper()
	code, out, errOut := run("", "route", "show", id, "--json")
	var got struct{ Groups []map[string]any }
	if err := json.Unmarshal([]byte(out), &got); code != 0 || err != nil || len(got.Groups) != 1 {
		t.Fatalf("route show %s --json: %d %v %q %q", id, code, err, out, errOut)
	}
	return got.Groups[0]
}

// TestRouteGroupsCLI: a route group is made, placed, given a strategy, a
// range, a way of choosing and test sites, sent traffic, renamed and
// deleted, all from the command line, as the window would leave it.
func TestRouteGroupsCLI(t *testing.T) {
	a, asks, run, node, site := groupsApp(t)
	must := func(args ...string) string {
		t.Helper()
		code, out, errOut := run("", args...)
		if code != 0 {
			t.Fatalf("%v: %d %q %q", args, code, out, errOut)
		}
		return out
	}
	plan := func() routes.Plan { return a.settings.Get().Routing }
	policy := func(id string) routes.Policy { return plan().Policy(id) }

	// Made, where it was asked to go, and named once only.
	out := must("route", "new", "Netflix", "--before", "media")
	if !strings.Contains(out, "Created route group Netflix (id g1)") {
		t.Errorf("route new: %q", out)
	}
	order := plan().MovableOrder()
	if i := slices.Index(order, "g1"); i < 0 || order[i+1] != "media" {
		t.Fatalf("g1 is not just before media: %v", order)
	}
	if code, _, errOut := run("", "route", "new", "Netflix"); code != cli.ExitFail || !strings.Contains(errOut, "already a route group by that name") {
		t.Errorf("a second Netflix: %d %q", code, errOut)
	}
	// Traffic, by the group's name; then it goes first.
	must("rules", "add", "netflix.com", "Netflix")
	if r := a.settings.Get().Rules; len(r) != 1 || r[0].Route != "g1" {
		t.Fatalf("the rule: %+v", r)
	}
	if out = must("route", "move", "Netflix", "--at", "1"); !strings.HasPrefix(out, "✓ Order: Netflix → ") {
		t.Errorf("route move: %q", out)
	}
	if plan().MovableOrder()[0] != "g1" {
		t.Errorf("g1 is not first: %v", plan().MovableOrder())
	}
	must("route", "move", "reject", "--after", "Netflix")
	if o := plan().MovableOrder(); o[0] != "g1" || o[1] != "reject" {
		t.Errorf("reject after Netflix: %v", o)
	}
	if out = must("route", "move", "reject", "--after", "Netflix"); !strings.Contains(out, "nothing changed") {
		t.Errorf("a move to where it is: %q", out)
	}

	// The three strategies, each keeping its settings.
	if out = must("route", "strategy", "Netflix", "fixed", "--node", "JP2"); !strings.Contains(out, "Netflix: Fixed node: JP2") {
		t.Errorf("fixed: %q", out)
	}
	if q := policy("g1"); q.Mode != routes.ModeFixed || q.Fixed != node("JP2") {
		t.Errorf("fixed: %+v", q)
	}
	if out = must("route", "order", "g1", "JP1", "US1", "JP1"); !strings.Contains(out, "Backup order: JP1 → US1") {
		t.Errorf("order: %q", out)
	}
	if q := policy("g1"); q.Mode != routes.ModeOrder || !slices.Equal(q.Order, []string{node("JP1"), node("US1")}) || q.Fixed != node("JP2") {
		t.Errorf("order: %+v", q)
	}
	must("route", "scope", "g1", "--regions", "jp,US", "--providers", "Sub", "--except")
	must("route", "strategy", "g1", "auto", "--by", "download", "--every", "24")
	q := policy("g1")
	if q.Mode != routes.ModeAuto || !slices.Equal(q.Regions, []string{"JP", "US"}) || len(q.Providers) != 1 || !q.ExceptProviders || q.DownloadEvery != 24 || len(q.Order) != 2 {
		t.Errorf("auto: %+v", q)
	}
	must("route", "scope", "g1", "--nodes", `["JP1","US1"]`, "--providers", "all")
	if q := policy("g1"); !slices.Equal(q.Nodes, []string{node("JP1"), node("US1")}) || q.Providers != nil || q.ExceptProviders {
		t.Errorf("picked nodes, every provider: %+v", q)
	}
	must("route", "strategy", "g1", "--by", "delay")
	g := groupJSON(t, run, "g1")
	if st, _ := g["strategy"].(map[string]any); st["mode"] != "auto" || st["by"] != "delay" || g["name"] != "Netflix" || g["position"] != 1.0 {
		t.Errorf("route show --json: %v", g)
	}
	if out = must("route", "show", "Netflix"); !strings.Contains(out, "Fastest · JP, US · 2 nodes") || !strings.Contains(out, "suffix netflix.com") {
		t.Errorf("route show:\n%s", out)
	}

	// Test sites: the main one first, names kept, the defaults back.
	must("route", "sites", "Netflix", site, "fast.com")
	if got := a.settings.Get().TargetsOf("g1"); len(got) != 2 || got[0].URL != site || got[1] != (routes.Target{Name: "fast.com", URL: "https://fast.com"}) {
		t.Errorf("sites: %+v", got)
	}
	if out = must("route", "sites", "Netflix"); !strings.Contains(out, "127.0.0.1") || !strings.Contains(out, "main") || strings.Contains(out, "default test sites") {
		t.Errorf("route sites: %q", out)
	}
	must("route", "sites", "Netflix", "--reset")
	if _, custom := a.settings.Get().RouteTargets["g1"]; custom {
		t.Errorf("the defaults are not back: %+v", a.settings.Get().RouteTargets)
	}
	if code, _, errOut := run("", "route", "sites", "Netflix", "ftp://x"); code != cli.ExitFail || !strings.Contains(errOut, "http or https") {
		t.Errorf("a bad site: %d %q", code, errOut)
	}

	// Direct, with the strategy kept; no sites while it is.
	must("route", "direct", "Netflix", "on")
	if q := policy("g1"); !q.Direct || q.Mode != routes.ModeAuto {
		t.Errorf("direct: %+v", q)
	}
	if code, _, errOut := run("", "route", "sites", "Netflix", "--reset"); code != cli.ExitFail || !strings.Contains(errOut, "uses no node") {
		t.Errorf("sites of a direct group: %d %q", code, errOut)
	}
	must("route", "direct", "Netflix", "off")

	// Renamed, then deleted: its rule goes with its services.
	must("route", "rename", "Netflix", "Video")
	if title(a.settings.Get(), "g1") != "Video" {
		t.Errorf("rename: %q", title(a.settings.Get(), "g1"))
	}
	must("route", "sites", "Video", site)
	must("route", "remove", "Video", "--to", "media")
	st := a.settings.Get()
	if slices.Contains(st.Routing.GroupOrder(), "g1") || st.Rules[0].Route != "media" || st.RouteTargets["g1"] != nil {
		t.Errorf("after delete: %v %+v %v", st.Routing.GroupOrder(), st.Rules, st.RouteTargets)
	}

	// What can't be, and what was typed wrong.
	for _, c := range []struct {
		args []string
		code int
		says string
	}{
		{[]string{"route", "move", "default", "--at", "1"}, cli.ExitFail, "always last"},
		{[]string{"route", "move", "ai", "--after", "default"}, cli.ExitFail, "always last"},
		{[]string{"route", "move", "ai"}, cli.ExitUsage, "--before, --after or --at"},
		{[]string{"route", "move", "ai", "--at", "1", "--before", "dev"}, cli.ExitUsage, "only one"},
		{[]string{"route", "rename", "direct", "Mine"}, cli.ExitFail, "keeps its name"},
		{[]string{"route", "remove", "reject"}, cli.ExitFail, "can't be deleted"},
		{[]string{"route", "strategy", "direct", "auto"}, cli.ExitFail, "never uses a node"},
		{[]string{"route", "strategy", "ai", "fastest"}, cli.ExitUsage, "auto, order, fixed"},
		{[]string{"route", "strategy", "ai"}, cli.ExitUsage, "Give a strategy"},
		{[]string{"route", "strategy", "ai", "--node", "nowhere"}, cli.ExitFail, "No such node"},
		{[]string{"route", "scope", "ai", "--regions", "Japan"}, cli.ExitFail, "Not a country code"},
		{[]string{"route", "scope", "ai", "--except"}, cli.ExitFail, "no providers to leave out"},
		{[]string{"route", "direct", "ai", "maybe"}, cli.ExitUsage, "on or off"},
		{[]string{"route", "sites", "ai", "x.com", "--reset"}, cli.ExitUsage, "--reset"},
		{[]string{"route", "show", "nowhere"}, cli.ExitFail, "No such route"},
	} {
		if code, _, errOut := run("", c.args...); code != c.code || !strings.Contains(errOut, c.says) {
			t.Errorf("%v: %d %q, want %d and %q", c.args, code, errOut, c.code, c.says)
		}
	}
	// A fixed node without one named starts from the node in use; with
	// none in use there is none to start from, and nothing is saved.
	inUse := ""
	for _, rv := range a.snapshot().Routes {
		if rv.ID == "dev" {
			inUse = rv.Node
		}
	}
	code, _, errOut := run("", "route", "strategy", "dev", "fixed")
	switch q := policy("dev"); {
	case inUse != "" && (code != 0 || q.Mode != routes.ModeFixed || q.Fixed != inUse):
		t.Errorf("fixed from the node in use (%s): %d %q %+v", inUse, code, errOut, q)
	case inUse == "" && (code != cli.ExitFail || !strings.Contains(errOut, "needs a node") || q.Mode == routes.ModeFixed):
		t.Errorf("fixed without a node: %d %q %+v", code, errOut, q)
	}
	if *asks != 0 || len(agentChanges()) != 0 {
		t.Errorf("the user's commands asked %d times", *asks)
	}
}

// TestRouteGroupsAgents: the same changes from an agent follow the user's
// policy, on the agents' socket and through the command line.
func TestRouteGroupsAgents(t *testing.T) {
	a, asks, run, _, site := groupsApp(t)
	srv, s := a.mcpServer(), &mcp.Session{}
	for _, name := range []string{"list_route_groups", "create_route_group", "rename_route_group", "delete_route_group", "move_route_group", "set_route_strategy", "set_route_sites", "set_routing_source"} {
		if !slices.ContainsFunc(srv.Tools, func(tool mcp.Tool) bool { return tool.Name == name }) {
			t.Errorf("the agents' server lacks %s", name)
		}
	}

	setPolicy(t, a, "deny")
	if out, err := callTool(t, srv, s, "list_route_groups", map[string]any{"route": "ai"}); err != nil || len(out["groups"].([]any)) != 1 {
		t.Errorf("reading under deny: %v %v", out, err)
	}
	if _, err := callTool(t, srv, s, "create_route_group", map[string]any{"name": "Work"}); err == nil || len(plan(a).GroupOrder()) != 6 {
		t.Errorf("deny let a group be made: %v", err)
	}
	if code, _, errOut := run("Robot", "route", "move", "dev", "--at", "1"); code != cli.ExitFail || !strings.Contains(errOut, "not to let agents make changes") {
		t.Errorf("deny let an agent's command move a group: %d %q", code, errOut)
	}

	setPolicy(t, a, "ask") // and the user says no
	if _, err := callTool(t, srv, s, "set_route_strategy", map[string]any{"route": "ai", "mode": "order", "order": []string{"JP1"}}); err == nil || *asks != 1 {
		t.Errorf("a refused strategy: %v, asks %d", err, *asks)
	}
	// Nothing to change asks nothing.
	if out, err := callTool(t, srv, s, "set_route_strategy", map[string]any{"route": "ai", "mode": "auto"}); err != nil || out["changed"] != false || *asks != 1 {
		t.Errorf("no change: %v %v, asks %d", out, err, *asks)
	}

	setPolicy(t, a, "allow")
	out, err := callTool(t, srv, s, "create_route_group", map[string]any{"name": "Work", "position": 1})
	if err != nil || out["created"] != "g1" || plan(a).MovableOrder()[0] != "g1" {
		t.Fatalf("create: %v %v", out, err)
	}
	// A group made after the server started is a route like the others.
	if _, err := callTool(t, srv, s, "add_rule", map[string]any{"target": "corp.example", "route": "g1"}); err != nil {
		t.Errorf("add_rule to the new group: %v", err)
	}
	if _, err := callTool(t, srv, s, "set_route_strategy", map[string]any{"route": "Work", "order": []string{"US1", "JP1"}, "regions": []string{"US"}}); err != nil {
		t.Fatal(err)
	}
	if q := plan(a).Policy("g1"); q.Mode != routes.ModeOrder || len(q.Order) != 2 || !slices.Equal(q.Regions, []string{"US"}) {
		t.Errorf("order and range at once: %+v", q)
	}
	if _, err := callTool(t, srv, s, "move_route_group", map[string]any{"route": "g1", "before": "default"}); err != nil {
		t.Fatal(err)
	}
	if o := plan(a).MovableOrder(); o[len(o)-1] != "g1" {
		t.Errorf("before default is last: %v", o)
	}
	if _, err := callTool(t, srv, s, "set_route_sites", map[string]any{"route": "g1", "sites": []string{site + "/corp"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := callTool(t, srv, s, "delete_route_group", map[string]any{"route": "g1"}); err != nil {
		t.Fatal(err)
	}
	if r := a.settings.Get().Rules; len(r) != 1 || r[0].Route != routes.Default {
		t.Errorf("the rule after delete: %+v", r)
	}
	if out, err := callTool(t, srv, s, "set_routing_source", map[string]any{"source": "bowel"}); err != nil || out["changed"] != false {
		t.Errorf("bowel's routing is in use already: %v %v", out, err)
	}
	ch := agentChanges()
	if len(ch) < 5 || ch[0].Outcome != "applied" || ch[0].What.en != "Delete a route group" {
		t.Errorf("the agent's changes are not listed: %+v", ch)
	}
}

func plan(a *App) routes.Plan { return a.settings.Get().Routing }

// TestRouteGroupsUnderImportedRouting: while a config's own groups are
// used, bowel's can't be changed, and the answer says how to go back; the
// source is switched back and forth by the same tools.
func TestRouteGroupsUnderImportedRouting(t *testing.T) {
	if testing.Short() {
		t.Skip("starts the core")
	}
	a, id, _ := importedApp(t)
	srv, s := a.mcpServer(), &mcp.Session{}
	setPolicy(t, a, "allow")
	if out, err := callTool(t, srv, s, "set_routing_source", map[string]any{"source": id}); err != nil || out["changed"] != true || a.importedNow() == nil {
		t.Fatalf("to the config's routing: %v %v", out, err)
	}
	before := a.settings.Get().Routing
	for name, args := range map[string]map[string]any{
		"create_route_group": {"name": "Work"},
		"move_route_group":   {"route": "Streaming", "position": 1},
		"set_route_strategy": {"route": "Streaming", "mode": "auto"},
		"set_route_sites":    {"route": "Streaming", "sites": []string{"netflix.com"}},
		"rename_route_group": {"route": "Streaming", "name": "TV"},
		"delete_route_group": {"route": "Streaming"},
	} {
		if _, err := callTool(t, srv, s, name, args); err == nil || !strings.Contains(err.Error(), "set_routing_source") {
			t.Errorf("%s under the config's routing: %v", name, err)
		}
	}
	if !routingCoreSame(before, a.settings.Get().Routing) {
		t.Errorf("bowel's groups changed")
	}
	out, err := callTool(t, srv, s, "list_route_groups", nil)
	if err != nil || out["imported"] == nil || len(out["groups"].([]any)) != 0 {
		t.Errorf("list_route_groups: %v %v", out, err)
	}
	if out, err := callTool(t, srv, s, "set_routing_source", map[string]any{"source": "bowel"}); err != nil || out["changed"] != true || a.importedNow() != nil {
		t.Errorf("back to bowel's routing: %v %v", out, err)
	}
	if _, err := callTool(t, srv, s, "create_route_group", map[string]any{"name": "Work"}); err != nil {
		t.Errorf("bowel's groups again: %v", err)
	}
}
