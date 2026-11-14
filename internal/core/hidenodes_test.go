package core

import (
	"slices"
	"strings"
	"testing"

	"github.com/dlclark/regexp2"
	"gopkg.in/yaml.v3"

	"github.com/oboo/moho/internal/provider"
	"github.com/oboo/moho/internal/routes"
)

// The nodes the user removed from a Provider (#160) are left out by the
// core: its exclude-filter gains a part, split off by a backtick as the core
// splits it, that matches each of them literally and whole, by the name the
// file has (the core filters before it adds the provider's suffix), and
// nothing else.
func TestProvidersExcludeHiddenNodes(t *testing.T) {
	hidden := []string{"香港 01", "a.b (x)", "tick`y", "node"}
	b, err := BuildConfig(Input{
		Providers: []ProviderRef{{ID: "aaa", Path: "/store/aaa.txt", Hidden: hidden}, {ID: "bbb", Path: "/store/bbb.txt"}},
		Routes:    routes.Template(), Mode: "rule", MixedPort: 7890, TestURL: "http://t",
	})
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Providers map[string]map[string]any `yaml:"proxy-providers"`
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg.Providers["p-bbb"]["exclude-filter"]; got != provider.InfoPattern {
		t.Fatalf("a provider with nothing removed: %v", got)
	}
	got, _ := cfg.Providers["p-aaa"]["exclude-filter"].(string)
	parts := strings.Split(got, "`")
	if len(parts) != 2 || parts[0] != provider.InfoPattern {
		t.Fatalf("exclude-filter = %q", got)
	}
	re := regexp2.MustCompile(parts[1], regexp2.None)
	for _, n := range hidden {
		if ok, _ := re.MatchString(n); !ok {
			t.Errorf("%q is not left out", n)
		}
	}
	for _, n := range []string{"香港 02", "香港 01 x", "axb (x)", "tickxy", "tick.y", "node\n", "node" + NodeSep + "aaa"} {
		if ok, _ := re.MatchString(n); ok {
			t.Errorf("%q is left out", n)
		}
	}
}

// A removed node leaves the provider's own routing too: its proxies, the
// groups that list it, and the rules that send traffic straight to it.
func TestImportedWithout(t *testing.T) {
	im, err := ParseImported([]byte(`proxies:
  - {name: A, type: direct}
  - {name: B, type: direct}
proxy-groups:
  - {name: Pick, type: select, proxies: [A, B]}
  - {name: OnlyA, type: select, proxies: [A]}
  - {name: Fall, type: url-test, proxies: [A], empty-fallback: DIRECT}
  - {name: All, type: select, include-all-proxies: true}
rules:
  - DOMAIN,a.example,A
  - DOMAIN,p.example,Pick,no-resolve
  - AND,((DOMAIN,x),(NETWORK,TCP)),A
  - MATCH,A
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	before := slices.Clone(im.Rules)
	out := im.Without([]string{"A"})
	if !slices.Equal(im.Rules, before) || len(im.Proxies) != 2 || len(stringsOf(im.Groups[0]["proxies"])) != 2 {
		t.Fatal("the parsed config was changed")
	}
	if len(out.Proxies) != 1 || nameOf(out.Proxies[0]) != "B" {
		t.Errorf("proxies = %v", out.Proxies)
	}
	want := map[string][]string{"Pick": {"B"}, "OnlyA": {"COMPATIBLE"}, "Fall": {"DIRECT"}}
	for _, g := range out.Groups {
		if w, ok := want[nameOf(g)]; ok && !slices.Equal(stringsOf(g["proxies"]), w) {
			t.Errorf("%s = %v", nameOf(g), g["proxies"])
		}
	}
	groups, err := out.runtimeGroups()
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range groups {
		if m := g.(map[string]any); nameOf(m) == "All" && !slices.Equal(m["proxies"].([]string), []string{"B"}) {
			t.Errorf("include-all = %v", m["proxies"])
		}
	}
	wantRules := []string{"DOMAIN,a.example,REJECT", "DOMAIN,p.example,Pick,no-resolve", "AND,((DOMAIN,x),(NETWORK,TCP)),REJECT", "MATCH,REJECT"}
	if !slices.Equal(out.Rules, wantRules) {
		t.Errorf("rules = %q", out.Rules)
	}
	if im.Without(nil) != im {
		t.Error("nothing removed should be the same routing")
	}
}
