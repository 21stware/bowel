package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestBundledFonts: the faces the skins name that a desktop may not have
// are bundled, each beside its license, and registered under one family
// name whatever its files call themselves; the monospace faces fall to the
// bundled one before the system's.
func TestBundledFonts(t *testing.T) {
	fontsOnce.Do(registerFonts)
	got := map[string]ui.FontFamily{}
	for _, f := range ui.FontFamilies() {
		if f.Registered {
			got[f.Name] = f
		}
	}
	if _, ok := got["Jost"]; !ok {
		t.Errorf("Jost is not registered: %v", got)
	}
	if f, ok := got["IBM Plex Mono"]; !ok || !f.Monospaced {
		t.Errorf("IBM Plex Mono is not registered as a monospace: %v", got)
	}
	for _, f := range bundledFonts {
		dirs := []string{filepath.Join("resources", "fonts")}
		if f.file == "RaiseSans.ttf" {
			// Windows and Linux alone: a Mac draws with PingFang.
			dirs = []string{filepath.Join("resources", "windows", "fonts"), filepath.Join("resources", "linux", "fonts")}
			if _, err := os.Stat(filepath.Join("resources", "darwin", "fonts", f.file)); err == nil {
				t.Errorf("%s ships with the Mac app", f.file)
			}
			if _, ok := got["Raise Sans"]; ok != (runtime.GOOS != "darwin") {
				t.Errorf("Raise Sans registered on %s: %v", runtime.GOOS, ok)
			}
		}
		for _, dir := range dirs {
			if _, err := os.Stat(filepath.Join(dir, f.file)); err != nil {
				t.Error(err)
			}
			if _, err := os.Stat(filepath.Join(dir, f.license)); err != nil {
				t.Errorf("%s: %v", f.file, err)
			}
		}
	}
	if !strings.HasPrefix(sans, "Raise Sans, PingFang SC,") {
		t.Errorf("sans %q", sans)
	}
	for _, face := range []string{faceMono, facePlex} {
		if !strings.Contains(face, "IBM Plex Mono") || !strings.HasSuffix(face, "monospace") {
			t.Errorf("face %q does not fall to the bundled monospace, then the system's", face)
		}
	}
	if !strings.HasPrefix(faceJost, "Jost,") {
		t.Errorf("face %q", faceJost)
	}
}
