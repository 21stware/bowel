package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBinName: on macOS the core's file name, less its last extension, is
// what codesign signs it as: the app's identifier and ".core", which has
// the dots macOS 27 wants before it shows the core who opens connections.
// Elsewhere the name is the one it always had.
func TestBinName(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "mygo.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct{ Identifier string }
	if err := json.Unmarshal(b, &cfg); err != nil || cfg.Identifier == "" {
		t.Fatalf("mygo.json: %v, identifier %q", err, cfg.Identifier)
	}
	name := binName("darwin")
	if id := strings.TrimSuffix(name, filepath.Ext(name)); id != cfg.Identifier+".core" {
		t.Errorf("the macOS core would be signed as %q, not %q", id, cfg.Identifier+".core")
	}
	if binName("linux") != "mihomo" || binName("windows") != "mihomo.exe" {
		t.Errorf("linux %q, windows %q", binName("linux"), binName("windows"))
	}
}
