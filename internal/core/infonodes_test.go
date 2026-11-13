package core

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/oboo/moho/internal/provider"
	"github.com/oboo/moho/internal/routes"
)

// Each Provider tells the core to leave out the entries that only say how
// much traffic is left and the like (#139), and the pattern reaches it
// as written.
func TestProvidersExcludeInfoEntries(t *testing.T) {
	b, err := BuildConfig(Input{
		Providers: []ProviderRef{{ID: "aaa", Path: "/store/aaa.txt"}},
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
	if got := cfg.Providers["p-aaa"]["exclude-filter"]; got != provider.InfoPattern {
		t.Fatalf("exclude-filter = %v", got)
	}
}
