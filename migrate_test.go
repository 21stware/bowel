package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateData(t *testing.T) {
	root := t.TempDir()
	old, dir := filepath.Join(root, "Moho Dev"), filepath.Join(root, "bowel Dev")
	for _, f := range []string{"settings.json", "providers/a.yaml", "bin/mihomo"} {
		p := filepath.Join(old, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// The window's state, written under the new name before the app's data.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "window-state.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	migrateData(dir)
	for _, f := range []string{"settings.json", "providers/a.yaml", "bin/mihomo", "window-state.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s not in the new directory: %v", f, err)
		}
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("the old directory stays: %v", err)
	}
	// Once there, nothing moves again.
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "settings.json"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	migrateData(dir)
	if b, _ := os.ReadFile(filepath.Join(dir, "settings.json")); string(b) != "settings.json" {
		t.Errorf("settings replaced by %q", b)
	}
}
