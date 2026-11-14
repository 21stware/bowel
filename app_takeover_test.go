package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/mcp"
	"github.com/oboo/moho/internal/settings"
)

// A paused entry must reach its setter even when its saved switch is on.
// The missing test core deliberately refuses the apply after the settings
// change: no TUN device, system DNS or system proxy can be changed here.
func TestMCPTakeoverResumesPausedEntry(t *testing.T) {
	for _, entry := range []string{"tun", "systemProxy"} {
		t.Run(entry, func(t *testing.T) {
			a, _, _ := mcpApp(t)
			savedTun, savedDNS := osTun, osDNS
			t.Cleanup(func() { osTun, osDNS = savedTun, savedDNS })
			osTun.installed = func(string) bool { return true }
			osTun.needsRule = func(string) bool { return false }
			osTun.install = func(string, string) error { t.Fatal("attempted TUN installation"); return nil }
			osDNS.supported = false
			osProxy.enable = func(string, int, []string) error { return nil }
			a.coreBin = filepath.Join(a.dataDir, "missing-core")
			a.tunBin = a.coreBin
			a.core.SetBin(a.coreBin)
			if _, err := a.settings.Update(func(s *settings.Settings) {
				s.Paused, s.Tun, s.SystemProxy = true, true, true
				s.IPv6 = core.IPv6Off
			}); err != nil {
				t.Fatal(err)
			}
			var detail string
			askAgent = func(_ context.Context, _, _, d string) (bool, bool, error) {
				detail = d
				return true, false, nil
			}
			out, err := callTool(t, a.mcpServer(), &mcp.Session{}, "set_takeover", map[string]any{entry: true})
			st := a.settings.Get()
			if !a.takeoverOn(st) || st.Tun != (entry == "tun") || st.SystemProxy != (entry == "systemProxy") {
				t.Fatalf("entry %s did not resume alone: paused=%v tun=%v systemProxy=%v, reply=%v err=%v", entry, st.Paused, st.Tun, st.SystemProxy, out, err)
			}
			if !strings.Contains(detail, entry+": false → true") {
				t.Errorf("confirmation did not describe resuming: %q", detail)
			}
			if err == nil {
				t.Error("the missing core's apply failure was hidden")
			}
		})
	}
}

// Turning a saved entry off while paused still clears it, so a later
// `bowel on` cannot bring back an entry the user explicitly disabled.
func TestMCPTakeoverPausedOffClearsEntry(t *testing.T) {
	for _, entry := range []string{"tun", "systemProxy"} {
		t.Run(entry, func(t *testing.T) {
			a, _, _ := mcpApp(t)
			setPolicy(t, a, "allow")
			a.coreBin = filepath.Join(a.dataDir, "missing-core")
			a.core.SetBin(a.coreBin)
			if _, err := a.settings.Update(func(s *settings.Settings) {
				s.Paused, s.Tun, s.SystemProxy = true, true, true
				s.IPv6 = core.IPv6Off
			}); err != nil {
				t.Fatal(err)
			}
			_, _ = callTool(t, a.mcpServer(), &mcp.Session{}, "set_takeover", map[string]any{entry: false})
			st := a.settings.Get()
			if !st.Paused || st.Tun != (entry != "tun") || st.SystemProxy != (entry != "systemProxy") {
				t.Fatalf("paused off: paused=%v tun=%v systemProxy=%v", st.Paused, st.Tun, st.SystemProxy)
			}
			out, err := callTool(t, a.mcpServer(), &mcp.Session{}, "set_takeover", map[string]any{entry: false})
			if err != nil || out["text"] != "Nothing to change." {
				t.Errorf("repeating off is not a no-op: %v %v", out, err)
			}
		})
	}
}
