package devnet

import (
	"testing"
	"time"
)

// Issue 217: Tailscale's CLI on the Mac is its app, and each run brings up
// its menu bar icon, so it must not be run on a timer for nothing.
func TestAskTailscale(t *testing.T) {
	old := tailscaleEvery + time.Second
	for _, c := range []struct {
		name    string
		changed bool
		age     time.Duration
		timed   bool
		want    bool
	}{
		{"address came or went", true, 0, false, true},
		{"old, peers wanted", false, old, true, true},
		{"fresh, peers wanted", false, time.Second, true, false},
		{"old, nothing wants peers", false, old, false, false},
	} {
		if got := askTailscale(c.changed, c.age, c.timed); got != c.want {
			t.Errorf("%s: askTailscale = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestOpensApp(t *testing.T) {
	for bin, want := range map[string]bool{
		"/Applications/Tailscale.app/Contents/MacOS/Tailscale": true,
		"/opt/homebrew/bin/tailscale":                          false,
		`C:\Program Files\Tailscale\tailscale.exe`:             false,
		"": false,
	} {
		if got := opensApp(bin); got != want {
			t.Errorf("opensApp(%q) = %v, want %v", bin, got, want)
		}
	}
}
