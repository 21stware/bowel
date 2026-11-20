package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/provider"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

// TestExpiredPlanIsSaidFirst: when a plan has ended, its nodes are down and
// so are the groups on them; the reminder says why, before the groups do,
// and offers its website. A plan only ending soon waits its turn, and a
// disabled provider says nothing.
func TestExpiredPlanIsSaidFirst(t *testing.T) {
	store, err := provider.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "a.yaml")
	os.WriteFile(src, []byte("proxies:\n  - {name: a, type: direct}\n  - {name: '官网：renew.example.com', type: direct}\n"), 0o600)
	p, err := store.AddFile(src)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{providers: store}
	ended := &Usage{Expire: time.Now().Add(-time.Hour).Unix()}
	pv := ProviderView{ID: p.ID, Name: "机场 A", Enabled: true, Usage: ended, Warn: provider.Expired}
	snap := Snapshot{Nodes: []NodeView{{Name: "a"}}, Routes: []RouteView{{ID: routes.Default, Name: "默认", Broken: "scope"}}, Providers: []ProviderView{pv}}
	cs := core.Status{State: core.Running}
	st, is := a.judge(true, settings.Settings{Mode: "rule"}, cs, Takeover{}, snap, nil)
	if st != StateDegraded || is == nil || !strings.Contains(is.Text, "机场 A") || is.Action != "site:https://renew.example.com" {
		t.Fatalf("expired: %v %+v", st, is)
	}

	pv.Warn, pv.Usage = provider.Expire, &Usage{Expire: time.Now().Add(72 * time.Hour).Unix()}
	snap.Providers = []ProviderView{pv}
	if _, is := a.judge(true, settings.Settings{Mode: "rule"}, cs, Takeover{}, snap, nil); is == nil || strings.Contains(is.Text, "机场 A") {
		t.Fatalf("a broken group goes before a plan ending soon: %+v", is)
	}
	snap.Routes = nil
	if _, is := a.judge(true, settings.Settings{Mode: "rule"}, cs, Takeover{}, snap, nil); is == nil || !strings.Contains(is.Text, "3") || !strings.HasPrefix(is.Action, "site:") {
		t.Fatalf("ending soon: %+v", is)
	}

	pv.Enabled = false
	snap.Providers = []ProviderView{pv}
	if _, is := a.judge(true, settings.Settings{Mode: "rule"}, cs, Takeover{}, snap, nil); is != nil {
		t.Fatalf("disabled: %+v", is)
	}
}
