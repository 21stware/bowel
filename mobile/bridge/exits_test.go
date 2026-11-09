package bridge

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oboo/moho/internal/mihomo"
	"gopkg.in/yaml.v3"
)

func probeConfig() map[string]any {
	return map[string]any{
		"listeners":          []any{map[string]any{"name": "moho-probe", "type": "mixed"}},
		"skip-auth-prefixes": []string{"127.0.0.1/8"},
	}
}

func saveConfig(t *testing.T, dir string, cfg map[string]any) {
	t.Helper()
	out, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), out, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestProbeListenerNeedsPassword(t *testing.T) {
	x := &bridge{dir: t.TempDir()}
	cfg := probeConfig()
	if err := x.lockProbe(cfg); err != nil {
		t.Fatal(err)
	}
	if _, open := cfg["skip-auth-prefixes"]; open {
		t.Fatal("127.0.0.1 would skip the password")
	}
	saveConfig(t, x.dir, cfg)
	login := x.probeLogin()
	first, _ := login.Password()
	if login.Username() != probeUser || len(first) != 32 {
		t.Fatalf("login = %v", login)
	}

	// A new tunnel gets a new password.
	next := probeConfig()
	if err := x.lockProbe(next); err != nil {
		t.Fatal(err)
	}
	saveConfig(t, x.dir, next)
	second, _ := x.probeLogin().Password()
	if second == first {
		t.Fatal("the password did not change for a new tunnel")
	}

	// A reload keeps the one the running core started with.
	x.client = mihomo.NewUnix(filepath.Join(x.dir, socketName))
	again := probeConfig()
	if err := x.lockProbe(again); err != nil {
		t.Fatal(err)
	}
	saveConfig(t, x.dir, again)
	if kept, _ := x.probeLogin().Password(); kept != second {
		t.Fatal("a reload changed the password of the running core")
	}
}
