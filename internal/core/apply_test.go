package core

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oboo/moho/internal/routes"
	"gopkg.in/yaml.v3"
)

// A running core checks the config it is asked to reload itself: no
// `mihomo -t` for one it accepts, and one it refuses leaves it, and
// config.yaml, as they were, with what the check says (#60).
func TestApplyRunningCoreChecks(t *testing.T) {
	bin := coreBin(t)
	dir, err := os.MkdirTemp("", "moho")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	geo(t, dir)
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte(yamlNodes), 0o600)
	m, err := New(bin, dir)
	if err != nil {
		t.Fatal(err)
	}
	probe, _ := FreePort()
	mixed, _ := FreePort()
	in := Input{
		Providers: []ProviderRef{{ID: "aaa", Path: filepath.Join(dir, "a.txt")}},
		Routes:    routes.Template(), Mode: "rule", MixedPort: mixed, LogLevel: "info",
		TestURL: "https://www.gstatic.com/generate_204", ProbePort: probe, Controller: m.ControllerFields(),
	}
	cfg, _ := BuildConfig(in)
	if err := m.Apply(cfg); err != nil {
		t.Fatalf("start: %v\n%s", err, m.logs.last(10))
	}
	defer m.Stop()
	// No core to ask yet: the check ran before it started.
	if tm := m.Timing(); tm.Check <= 0 || tm.Start <= 0 || tm.Reload != 0 {
		t.Fatalf("start timing = %+v", tm)
	}
	pid := m.Status().PID
	c, _ := m.Client()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	onDisk := func() []byte {
		b, err := os.ReadFile(m.ConfigPath())
		if err != nil {
			t.Fatal(err)
		}
		return b
	}

	in.Mode = "global"
	global, _ := BuildConfig(in)
	if err := m.Apply(global); err != nil {
		t.Fatal(err)
	}
	if tm := m.Timing(); tm.Check != 0 || tm.Reload <= 0 || tm.Start != 0 {
		t.Errorf("reload timing = %+v, want no check", tm)
	}
	if conf, _ := c.Config(ctx); conf.Mode != "global" || m.Status().PID != pid {
		t.Fatalf("after reload: %+v, pid %d → %d", conf, pid, m.Status().PID)
	}
	if !bytes.Equal(onDisk(), global) {
		t.Error("config.yaml is not the config the core accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "config.next.yaml")); !os.IsNotExist(err) {
		t.Errorf("config.next.yaml after an accepted reload: %v", err)
	}

	// One the core refuses: a rule to a group that is not there.
	var doc map[string]any
	in.Mode = "direct"
	direct, _ := BuildConfig(in)
	if err := yaml.Unmarshal(direct, &doc); err != nil {
		t.Fatal(err)
	}
	doc["rules"] = append([]any{"DOMAIN,example.org,nowhere"}, doc["rules"].([]any)...)
	bad, _ := yaml.Marshal(doc)
	m.setListenErr("port 1 is taken")
	err = m.Apply(bad)
	if err == nil || !strings.Contains(err.Error(), "proxy [nowhere] not found") {
		t.Fatalf("refused config: err = %v", err)
	}
	if tm := m.Timing(); tm.Check <= 0 {
		t.Errorf("refused timing = %+v, want the check to say why", tm)
	}
	if conf, _ := c.Config(ctx); conf.Mode != "global" || m.Status().PID != pid {
		t.Errorf("after a refused reload: %+v, pid %d → %d", conf, pid, m.Status().PID)
	}
	if !bytes.Equal(onDisk(), global) {
		t.Error("a refused config replaced config.yaml")
	}
	if got := m.Status().Error; got != "port 1 is taken" {
		t.Errorf("a refused reload changed the port's error to %q", got)
	}
	m.setListenErr("")

	// And the core takes the next good one as before.
	if err := m.Apply(direct); err != nil {
		t.Fatal(err)
	}
	if conf, _ := c.Config(ctx); conf.Mode != "direct" || m.Status().PID != pid {
		t.Fatalf("after the next reload: %+v, pid %d → %d", conf, pid, m.Status().PID)
	}
}
