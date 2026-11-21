package main

import (
	"testing"

	"github.com/oboo/moho/internal/settings"
)

// The system proxy points at the core for takeover by it, and for the exit
// lock while takeover is off; never under a provider's own routing, which
// has none of the groups.
func TestProxyWantedByTheLock(t *testing.T) {
	a := &App{}
	locked := func(s settings.Settings) settings.Settings {
		s = s.Clone()
		s.Routing.SetLocked("ai", true)
		return s
	}
	for _, c := range []struct {
		name        string
		s           settings.Settings
		guard, want bool
	}{
		{"on, system proxy", settings.Settings{SystemProxy: true}, false, true},
		{"on, TUN only", settings.Settings{Tun: true}, false, false},
		{"paused", settings.Settings{SystemProxy: true, Paused: true}, false, false},
		{"paused, locked", locked(settings.Settings{SystemProxy: true, Paused: true}), true, true},
		{"no entry, locked", locked(settings.Settings{}), true, true},
		{"TUN only, locked", locked(settings.Settings{Tun: true}), false, false},
		{"paused, locked, imported", func() settings.Settings {
			s := locked(settings.Settings{Paused: true})
			s.ImportedRouting = "p1"
			return s
		}(), false, false},
	} {
		if got := a.guarding(c.s); got != c.guard {
			t.Errorf("%s: guarding %v, want %v", c.name, got, c.guard)
		}
		if got := a.proxyWanted(c.s); got != c.want {
			t.Errorf("%s: proxyWanted %v, want %v", c.name, got, c.want)
		}
	}
}
