package core

import (
	"strings"
	"testing"

	"github.com/dlclark/regexp2"
	"gopkg.in/yaml.v3"

	"github.com/oboo/moho/internal/provider"
	"github.com/oboo/moho/internal/routes"
)

// The nodes the user removed from a Provider (#160) are left out by the
// core: its exclude-filter gains a part, split off by a backtick as the core
// splits it, that matches each of them, by its own name or with the
// provider's suffix, and nothing else.
func TestProvidersExcludeHiddenNodes(t *testing.T) {
	hidden := []string{"香港 01", "a.b (x)", "tick`y"}
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
	for _, name := range hidden {
		for _, n := range []string{name, name + NodeSep + "aaa"} {
			if ok, _ := re.MatchString(n); !ok {
				t.Errorf("%q is not left out", n)
			}
		}
	}
	for _, n := range []string{"香港 02", "香港 01 x", "a.b (x)" + NodeSep + "bbb", "axb (x)"} {
		if ok, _ := re.MatchString(n); ok {
			t.Errorf("%q is left out", n)
		}
	}
}
