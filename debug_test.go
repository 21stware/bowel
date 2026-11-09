package main

import (
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

// TestDevBuildLeavesReleasePorts: a development build leaves the default
// ports to the released app, keeps a port the user chose, and doesn't take
// over the system on its own.
func TestDevBuildLeavesReleasePorts(t *testing.T) {
	dir, err := os.MkdirTemp("", "mh")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	mygo.App.SetPath(mygo.PathUserData, dir)
	defer mygo.App.SetPath(mygo.PathUserData, "")
	saved := core.LocalDNSPort
	defer func() { core.LocalDNSPort = saved }()

	a, err := newApp()
	if err != nil {
		t.Fatal(err)
	}
	if p := a.settings.Get().MixedPort; p != settings.Defaults.MixedPort {
		t.Fatalf("fresh port %d", p)
	}
	if _, err := a.settings.Update(func(s *settings.Settings) { s.Paused = false }); err != nil {
		t.Fatal(err)
	}
	a.separateFromRelease()
	if s := a.settings.Get(); s.MixedPort != devMixedPort || s.MixedPort == settings.Defaults.MixedPort || !s.Paused {
		t.Errorf("after separating: port %d, paused %v", s.MixedPort, s.Paused)
	}
	cfg, err := core.BuildConfig(core.Input{Routes: routes.Template(), Mode: "rule", MixedPort: devMixedPort, ProbePort: 1, TestURL: "x", Tun: true, Options: core.DefaultOptions})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfg), "127.0.0.1:11053") {
		t.Errorf("DNS isn't on the dev port:\n%s", cfg)
	}

	if _, err := a.settings.Update(func(s *settings.Settings) { s.MixedPort = 20000 }); err != nil {
		t.Fatal(err)
	}
	a.separateFromRelease()
	if p := a.settings.Get().MixedPort; p != 20000 {
		t.Errorf("a chosen port became %d", p)
	}
}
