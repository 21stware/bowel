package bridge

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/mihomo"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

// fullSub is a complete config, as issue #184 has one: nodes, groups that
// name each other, and rules.
const fullSub = `mixed-port: 7890
dns:
  enable: true
  nameserver: [223.5.5.5]
proxies:
  - {name: JP-01, type: ss, server: 192.0.2.11, port: 8388, cipher: aes-128-gcm, password: x}
  - {name: JP-02, type: ss, server: 192.0.2.12, port: 8388, cipher: aes-128-gcm, password: x}
proxy-groups:
  - name: Japan
    type: url-test
    proxies: [JP-02, JP-01]
    url: http://127.0.0.1:9/
    interval: 300
  - name: Streaming
    type: select
    proxies: [Japan, JP-01, DIRECT]
rules:
  - DOMAIN-SUFFIX,netflix.com,Streaming
  - DOMAIN-SUFFIX,ads.example,REJECT
  - MATCH,Japan
`

// importedBridge is testBridge with fullSub added as a subscription, its
// body behind p, and updates vetted as Start has them.
func importedBridge(t *testing.T) (*bridge, string, *page) {
	t.Helper()
	x := testBridge(t)
	x.store.Vet = x.vetUpdate
	if err := x.change(context.Background(), func(s *settings.Settings) {
		s.Rules = []routes.UserRule{{Kind: routes.KindDomain, Value: "mine.example", Route: "ai"}}
	}); err != nil {
		t.Fatal(err)
	}
	p, base := serve(t, fullSub)
	if err := x.addProvider(context.Background(), base+"/full"); err != nil {
		t.Fatal(err)
	}
	for _, pv := range x.view().Providers {
		if pv.Kind == "subscription" {
			return x, pv.ID, p
		}
	}
	t.Fatal("the subscription wasn't added")
	return nil, "", nil
}

func groupNames(cfg map[string]any) []string {
	var out []string
	for _, g := range cfg["proxy-groups"].([]any) {
		out = append(out, g.(map[string]any)["name"].(string))
	}
	return out
}

func routeNames(x *bridge) []string {
	var out []string
	for _, r := range x.routes() {
		out = append(out, r.Name)
	}
	return out
}

func TestImportedRoutingOnThePhone(t *testing.T) {
	x, id, _ := importedBridge(t)
	ctx := context.Background()

	// Until it is chosen, the config only gives its nodes.
	row := func() providerData {
		for _, pv := range x.view().Providers {
			if pv.ID == id {
				return pv
			}
		}
		t.Fatal("no row")
		return providerData{}
	}
	if r := row().Routing; r == nil || r.Groups != 2 || r.Rules != 3 || r.InUse {
		t.Fatalf("routing on its row = %+v, want 2 groups and 3 rules, not in use", r)
	}
	if slices.Contains(groupNames(config(t, x)), "Streaming") {
		t.Fatal("its groups are used before the user chose them")
	}

	if err := x.setImportedRouting(ctx, id); err != nil {
		t.Fatal(err)
	}
	if r := row().Routing; r == nil || !r.InUse {
		t.Fatalf("routing on its row = %+v, want it in use", r)
	}
	cfg, rules := rulesOfConfig(t, x)
	if g := groupNames(cfg); !slices.Contains(g, "Japan") || !slices.Contains(g, "Streaming") {
		t.Fatalf("groups = %v, want the config's", g)
	}
	if !slices.Contains(rules, "DOMAIN-SUFFIX,netflix.com,Streaming") || slices.ContainsFunc(rules, func(r string) bool { return strings.Contains(r, "mine.example") }) {
		t.Fatalf("rules = %v, want the config's and not the user's", rules)
	}
	if tun := cfg["tun"].(map[string]any); tun["stack"] != "gvisor" || cfg["mixed-port"] != nil {
		t.Fatalf("the phone's own parts were lost: tun %v, mixed-port %v", tun, cfg["mixed-port"])
	}
	if got := routeNames(x); !slices.Equal(got, []string{"Japan", "Streaming", "REJECT"}) {
		t.Fatalf("routes = %v, want MATCH's group first, then the others", got)
	}

	v := x.view()
	if v.Imported == nil || v.Imported.Rules != 3 || v.Imported.PausedRules != 1 || !slices.Contains(v.Imported.Unused, "dns") || v.Imported.Groups != nil {
		t.Fatalf("imported in the view = %+v", v.Imported)
	}
	if v.RuleCount != 3 {
		t.Fatalf("rule count = %d, want the config's 3", v.RuleCount)
	}
	rd := x.routing()
	if rd.Imported == nil || len(rd.Imported.Groups) != 2 {
		t.Fatalf("imported in the 路由组 data = %+v", rd.Imported)
	}
	for _, g := range rd.Imported.Groups {
		if want := g.Name == "Streaming"; g.Selectable != want {
			t.Errorf("%s selectable = %v, want %v", g.Name, g.Selectable, want)
		}
	}

	// What would change bowel's routing can't take effect, so it is refused.
	if err := x.routeOp(ctx, callArgs{Op: "newGroup", Name: "x"}); err == nil {
		t.Error("a route group was added under an imported config")
	}
	if err := x.addRule(ctx, "example.org", routes.Default); err == nil {
		t.Error("a rule was added under an imported config")
	}
	if _, err := x.importRuleSet(ctx, callArgs{Input: "https://x.example/a.list", Route: routes.Default}); err == nil || !strings.Contains(err.Error(), "暂停") && !strings.Contains(err.Error(), "paused") {
		t.Errorf("import rule set = %v, want refused as paused", err)
	}
	if d := x.explain("netflix.com"); d.Error == "" {
		t.Error("explain answered for rules it didn't write")
	}
	if err := x.selectMember(ctx, "Japan", "JP-01"); err == nil {
		t.Error("a url-test group was switched by hand")
	}

	// Back to bowel's: its routes and the user's rules return.
	if err := x.setImportedRouting(ctx, ""); err != nil {
		t.Fatal(err)
	}
	_, rules = rulesOfConfig(t, x)
	if !slices.ContainsFunc(rules, func(r string) bool { return strings.Contains(r, "mine.example") }) || slices.Contains(rules, "DOMAIN-SUFFIX,netflix.com,Streaming") {
		t.Fatalf("rules = %v, want bowel's again", rules)
	}
	if slices.Contains(routeNames(x), "Streaming") {
		t.Fatal("the config's routes stayed")
	}
}

// An update keeps the choice; one that would break it is not applied.
func TestImportedRoutingThroughUpdates(t *testing.T) {
	x, id, p := importedBridge(t)
	ctx := context.Background()
	if err := x.setImportedRouting(ctx, id); err != nil {
		t.Fatal(err)
	}

	p.set(200, "proxies:\n  - {name: JP-01, type: ss, server: 192.0.2.11, port: 8388, cipher: aes-128-gcm, password: x}\n")
	if err := x.updateProvider(ctx, id); err == nil {
		t.Fatal("an update without groups was applied")
	}
	if x.set.Get().ImportedRouting != id || !slices.Contains(groupNames(config(t, x)), "Streaming") {
		t.Fatal("a failed update changed the routing")
	}
	if v := x.view(); v.Issue == nil || !strings.Contains(v.Issue.Detail, "继续用上一份") && !strings.Contains(v.Issue.Detail, "last one stays") {
		t.Fatalf("issue = %+v, want the update that wasn't applied", v.Issue)
	}

	p.set(200, strings.Replace(fullSub, "  - MATCH,Japan", "  - DOMAIN-SUFFIX,news.example,Japan\n  - MATCH,Japan", 1))
	if err := x.updateProvider(ctx, id); err != nil {
		t.Fatal(err)
	}
	if v := x.view(); x.set.Get().ImportedRouting != id || v.Imported == nil || v.Imported.Rules != 4 {
		t.Fatalf("after a good update: chosen %q, imported %+v; want it kept with 4 rules", x.set.Get().ImportedRouting, v.Imported)
	}
}

// Disabling the provider goes to bowel's routing until it is enabled
// again; deleting it ends the choice.
func TestImportedRoutingDisabledAndDeleted(t *testing.T) {
	x, id, _ := importedBridge(t)
	ctx := context.Background()
	if err := x.setImportedRouting(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := x.setProviderEnabled(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	if x.importedNow() != nil || slices.Contains(routeNames(x), "Streaming") {
		t.Fatal("a disabled provider's routing is in use")
	}
	if err := x.setImportedRouting(ctx, id); err == nil {
		t.Fatal("a disabled provider's routing was chosen")
	}
	if err := x.setProviderEnabled(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(routeNames(x), "Streaming") {
		t.Fatal("enabling it again didn't bring its routing back")
	}
	if err := x.deleteProvider(ctx, id); err != nil {
		t.Fatal(err)
	}
	if got := x.set.Get().ImportedRouting; got != "" {
		t.Fatalf("ImportedRouting = %q after its provider was deleted", got)
	}
}

// A config whose nodes all come from its own subscriptions is no node file:
// the core refuses a provider file without proxies.
func TestImportedOnlyRemoteNodes(t *testing.T) {
	x := testBridge(t)
	x.store.Vet = x.vetUpdate
	ctx := context.Background()
	_, base := serve(t, `proxies: []
proxy-providers:
  airport:
    type: http
    url: https://sub.example/nodes
    interval: 3600
proxy-groups:
  - name: Auto
    type: url-test
    use: [airport]
rules:
  - MATCH,Auto
`)
	if err := x.addProvider(ctx, base+"/remote"); err != nil {
		t.Fatal(err)
	}
	var id string
	for _, pv := range x.view().Providers {
		if pv.Kind == "subscription" {
			id = pv.ID
			if pv.Routing == nil || pv.Routing.Remote != 1 {
				t.Fatalf("routing = %+v, want one subscription of its own", pv.Routing)
			}
		}
	}
	if _, ok := config(t, x)["proxy-providers"].(map[string]any)["p-"+id]; ok {
		t.Fatal("a config without proxies was given to the core as a node file")
	}
	if err := x.setImportedRouting(ctx, id); err != nil {
		t.Fatal(err)
	}
	pp := config(t, x)["proxy-providers"].(map[string]any)
	if _, ok := pp["airport"]; !ok {
		t.Fatalf("proxy-providers = %v, want the config's own", pp)
	}
}

// A route's node is at the end of its group's chain, as the node list names it.
func TestImportedRouteFollowsTheChain(t *testing.T) {
	x, id, _ := importedBridge(t)
	if err := x.setImportedRouting(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	jp2 := "JP-02" + core.NodeSep + id
	x.mu.Lock()
	x.proxies = map[string]mihomo.Proxy{
		"Streaming": {Name: "Streaming", Now: "Japan", All: []string{"Japan", "JP-01", "DIRECT"}},
		"Japan":     {Name: "Japan", Now: "JP-02", All: []string{"JP-02", "JP-01"}},
	}
	x.provNodes = map[string][]string{id: {"JP-01" + core.NodeSep + id, jp2}}
	x.mu.Unlock()
	v := x.view()
	for _, r := range v.Routes {
		switch r.Name {
		case "Streaming":
			if r.Node != jp2 || r.Auto {
				t.Errorf("Streaming = %+v, want on %q and chosen by hand", r, jp2)
			}
		case "Japan":
			if r.Node != jp2 || !r.Auto {
				t.Errorf("Japan = %+v, want on %q and chosen by the core", r, jp2)
			}
		}
	}
	if v.Exit == nil || v.Exit.Name != jp2 {
		t.Fatalf("exit = %+v, want %q", v.Exit, jp2)
	}
	if x.sourceName(jp2) != "JP-02" {
		t.Fatalf("source name of %q = %q", jp2, x.sourceName(jp2))
	}
	for _, g := range x.routing().Imported.Groups {
		if g.Name == "Streaming" && (g.Exit != jp2 || len(g.Members) != 3 || !g.Members[0].Group) {
			t.Errorf("Streaming in the 路由组 data = %+v", g)
		}
	}
}
