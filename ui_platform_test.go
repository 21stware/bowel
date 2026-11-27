package main

import (
	"runtime"
	"strings"
	"testing"

	"github.com/oboo/moho/internal/devtools"
	"github.com/oboo/moho/internal/i18n"
)

// Each system's lines name that system's own things, and no other's.
func TestPlatformLines(t *testing.T) {
	saved := i18n.Lang()
	defer i18n.Set(saved)
	i18n.Set("en")
	if s := tunInstallHintOn("darwin"); !strings.Contains(s, "macOS") {
		t.Errorf("macOS: %q", s)
	}
	if s := tunInstallHintOn("linux"); strings.Contains(s, "macOS") || strings.Contains(s, "Windows") {
		t.Errorf("Linux: %q", s)
	}
	// Windows asks at every start of TUN's core, and says why.
	if tunAsks != (runtime.GOOS == "windows") {
		t.Errorf("TUN asks each start on %s: %v", runtime.GOOS, tunAsks)
	}
	if s := tunAskNote(); !strings.Contains(s, "User Account Control") || !strings.Contains(s, "each time") || !strings.Contains(s, "code-signed") || !strings.Contains(s, "Defender") {
		t.Errorf("why Windows asks: %q", s)
	}
	if s := tunAskWait(); !strings.Contains(s, "User Account Control") {
		t.Errorf("waiting for the prompt: %q", s)
	}
	if s := firewallTipOn("darwin"); !strings.Contains(s, "System Settings") {
		t.Errorf("macOS firewall: %q", s)
	}
	if s := firewallTipOn("linux"); !strings.Contains(s, "ufw") || strings.Contains(s, "macOS") {
		t.Errorf("Linux firewall: %q", s)
	}
}

// A shortcut is written with the key the system's keyboard has: ⌘ on a
// Mac, Ctrl elsewhere, which is the key ui.Cmd binds there.
func TestShortcutLabel(t *testing.T) {
	for _, c := range []struct{ goos, key, want string }{
		{"darwin", "O", "⌘O"},
		{"darwin", ",", "⌘,"},
		{"windows", "O", "Ctrl+O"},
		{"windows", ",", "Ctrl+,"},
		{"linux", "Q", "Ctrl+Q"},
	} {
		if got := shortcutLabelOn(c.goos, c.key); got != c.want {
			t.Errorf("%s %q: %q, want %q", c.goos, c.key, got, c.want)
		}
	}
}

func TestToolSubOf(t *testing.T) {
	for file, want := range map[string]string{"~/.zshenv": "zsh", "~/.bashrc": "bash", "": ""} {
		if got := toolSubOf(devtools.Tool{ID: devtools.Shell, File: file}); got != want {
			t.Errorf("shell at %q: %q", file, got)
		}
	}
	if got := toolSubOf(devtools.Tool{ID: devtools.GitSSH, File: "~/.ssh/config"}); got != "git@github.com" {
		t.Errorf("Git SSH: %q", got)
	}
}
