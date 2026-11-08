package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Gateway is off at first, kept across opening the store again, and saved
// under its own key.
func TestGatewaySaved(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Get().Gateway {
		t.Fatal("gateway on by default")
	}
	if _, err := st.Update(func(s *Settings) { s.Gateway, s.Tun = true, true }); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["gateway"] != true {
		t.Fatalf("settings.json: gateway = %v", raw["gateway"])
	}
	again, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s := again.Get(); !s.Gateway || !s.Tun {
		t.Fatalf("reopened: gateway %v, tun %v", s.Gateway, s.Tun)
	}
	if err := again.Get().Validate(); err != nil {
		t.Fatal(err)
	}
	// Resetting the core's options leaves it: it decides who reaches bowel.
	s := again.Get()
	s.ResetCore()
	if !s.Gateway {
		t.Fatal("ResetCore turned the gateway off")
	}
}
