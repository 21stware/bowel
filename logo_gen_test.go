package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLogoIcons writes the tray's two icons to $MOHO_ICONS when MOHO_ICONS is
// set: go test -run LogoIcons. The app icon itself is assets/icon/AppIcon.icon.
func TestLogoIcons(t *testing.T) {
	dir := os.Getenv("MOHO_ICONS")
	if dir == "" {
		t.Skip("MOHO_ICONS not set")
	}
	for on, name := range map[bool]string{false: "off", true: "on"} {
		if err := os.WriteFile(filepath.Join(dir, "tray-"+name+".png"), trayIcon(on), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
