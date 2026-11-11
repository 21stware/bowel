package settings

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/oboo/moho/internal/routes"
)

// A file as a version with route groups saved it: the user's groups, their
// strategies, a service moved into one and a rule pointing at one.
const savedWithGroups = `{
  "language": "zh", "mixedPort": 7899, "mode": "rule",
  "rules": [{"kind": "suffix", "value": "example.com", "route": "g1", "service": "mine:g1"}],
  "routing": {
    "groups": [
      {"id": "default", "policy": {"mode": "auto"}},
      {"id": "ai", "policy": {"mode": "fixed", "fixed": "HK 01"}},
      {"id": "media", "policy": {}},
      {"id": "dev", "policy": {}},
      {"id": "g1", "name": "工作", "policy": {"mode": "auto", "regions": ["JP"]}},
      {"id": "g2", "name": "游戏", "policy": {"mode": "order", "order": ["SG 01", "SG 02"]}},
      {"id": "direct", "policy": {}}
    ],
    "services": [{"id": "github", "group": "g1"}, {"id": "s1", "custom": true, "name": "内部工具", "group": "g2"}]
  },
  "routeTargets": {"g1": [{"name": "example.com", "url": "https://example.com"}]}
}`

// TestGroupsSurviveAnUpgrade: this version reads such a file whole, finds
// nothing wrong with it, and saves it back with the groups as they were.
func TestGroupsSurviveAnUpgrade(t *testing.T) {
	st := openWith(t, savedWithGroups)
	s := st.Get()
	if err := s.Validate(); err != nil {
		t.Fatalf("an old file isn't valid any more: %v", err)
	}
	check := func(s Settings) {
		t.Helper()
		all := routes.Build(s.Routing, s.Rules)
		at := func(id string) routes.Route {
			i := slices.IndexFunc(all, func(r routes.Route) bool { return r.ID == id })
			if i < 0 {
				t.Fatalf("group %s is gone", id)
			}
			return all[i]
		}
		if g := at("g1"); g.Label != "工作" || g.Policy.Mode != routes.ModeAuto || !slices.Equal(g.Policy.Regions, []string{"JP"}) {
			t.Errorf("g1: %q %+v", g.Label, g.Policy)
		}
		if g := at("g2"); g.Label != "游戏" || g.Policy.Mode != routes.ModeOrder || !slices.Equal(g.Policy.Order, []string{"SG 01", "SG 02"}) {
			t.Errorf("g2: %q %+v", g.Label, g.Policy)
		}
		if g := at("ai"); g.Policy.Mode != routes.ModeFixed || g.Policy.Fixed != "HK 01" {
			t.Errorf("ai: %+v", g.Policy)
		}
		has := func(g routes.Route, id string) bool {
			return slices.ContainsFunc(g.Services, func(s routes.Service) bool { return s.ID == id })
		}
		if !has(at("g1"), "github") || !has(at("g2"), "s1") {
			t.Error("services left their groups")
		}
		if len(routes.Active(all, s.Rules)) != 1 {
			t.Error("the rule pointing at g1 no longer counts")
		}
		if len(s.RouteTargets["g1"]) != 1 {
			t.Error("g1's test sites are gone")
		}
		at(routes.Reject) // from after the file was saved
	}
	check(s)

	// Any change saves the whole file; opened again it says the same.
	if _, err := st.Update(func(s *Settings) { s.Routing.RenameGroup("g2", "游戏 2") }); err != nil {
		t.Fatal(err)
	}
	again, err := Open(filepath.Dir(st.path))
	if err != nil {
		t.Fatal(err)
	}
	s = again.Get()
	if i := slices.IndexFunc(s.Routing.Groups, func(g routes.GroupConf) bool { return g.ID == "g2" }); i < 0 || s.Routing.Groups[i].Name != "游戏 2" {
		t.Fatalf("the rename wasn't saved: %+v", s.Routing.Groups)
	}
	s.Routing.RenameGroup("g2", "游戏")
	check(s)
}

// TestUnreadableFileIsKept: a file that can't be read becomes the defaults,
// and what it held is put aside before a save writes over it.
func TestUnreadableFileIsKept(t *testing.T) {
	broken := savedWithGroups[:len(savedWithGroups)/2]
	st := openWith(t, broken)
	if got := st.Get(); len(got.Routing.Groups) != 0 || got.MixedPort != Defaults.MixedPort {
		t.Errorf("half a file was used: %+v", got.Routing)
	}
	if _, err := st.Update(func(s *Settings) { s.AllowLan = true }); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(st.path), "settings.broken.json"))
	if err != nil || string(b) != broken {
		t.Errorf("the unreadable file wasn't kept: %v", err)
	}
	// A good file leaves nothing behind.
	ok := openWith(t, savedWithGroups)
	if _, err := os.Stat(filepath.Join(filepath.Dir(ok.path), "settings.broken.json")); !os.IsNotExist(err) {
		t.Errorf("a readable file was put aside: %v", err)
	}
}
