package settings

import (
	"testing"

	"github.com/oboo/moho/internal/routes"
)

func TestTargetsOf(t *testing.T) {
	s := Defaults.Clone()
	s.TestURL = "https://probe.example/204"
	if got := s.TargetsOf(routes.Default); got[0].URL != s.TestURL || got[0].Name != "probe.example" {
		t.Errorf("default route: %+v", got)
	}
	if got := s.TargetsOf("media"); got[0].Name != "YouTube" {
		t.Errorf("media: %+v", got)
	}
	mine := []routes.Target{{Name: "x", URL: "https://x.example"}}
	s.RouteTargets = map[string][]routes.Target{"media": mine, routes.Default: mine}
	if got := s.TargetsOf("media"); len(got) != 1 || got[0] != mine[0] {
		t.Errorf("override: %+v", got)
	}
	if got := s.TargetsOf(routes.Default); len(got) != 1 {
		t.Errorf("the user's list for the default route stands alone: %+v", got)
	}
	// The result is the caller's to change.
	s.TargetsOf("media")[0].URL = "changed"
	if s.RouteTargets["media"][0].URL == "changed" {
		t.Error("TargetsOf shares its list")
	}
	c := s.Clone()
	c.RouteTargets["media"][0].URL = "changed"
	if s.RouteTargets["media"][0].URL == "changed" {
		t.Error("Clone shares the lists")
	}
	if err := s.Validate(); err != nil {
		t.Error(err)
	}
	s.RouteTargets["direct"] = mine
	if s.Validate() == nil {
		t.Error("the direct route accepted test sites")
	}
	s.ResetCore()
	if s.RouteTargets != nil {
		t.Errorf("after reset: %+v", s.RouteTargets)
	}
}
