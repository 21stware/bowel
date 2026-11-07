package main

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

// TestLogoIcons writes the app icon to resources/icon.png and the tray's
// two icons to $MOHO_ICONS when MOHO_ICONS is set: go test -run LogoIcons.
func TestLogoIcons(t *testing.T) {
	dir := os.Getenv("MOHO_ICONS")
	if dir == "" {
		t.Skip("MOHO_ICONS not set")
	}
	lime, dark := color.NRGBA{0xDD, 0xFB, 0x4C, 255}, color.NRGBA{0x15, 0x17, 0x1E, 255}
	if err := os.WriteFile("resources/icon.png", appIcon(1024, lime, dark), 0o644); err != nil {
		t.Fatal(err)
	}
	for on, name := range map[bool]string{false: "off", true: "on"} {
		if err := os.WriteFile(filepath.Join(dir, "tray-"+name+".png"), trayIcon(on), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
