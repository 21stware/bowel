package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/mihomo"
	"github.com/oboo/moho/internal/settings"
)

// TestExtraPortsOnCore: an extra port set as the CLI sets it (a settings
// patch) is open on this machine, bound to its group. A port another program
// holds, bowel's own, and a group that doesn't exist are refused, with the
// settings left as they were. A port whose group is deleted afterwards
// closes, and deleting the group is not held up by it.
func TestExtraPortsOnCore(t *testing.T) {
	a, _ := routingApp(t, nil)
	port := freePortFor(t)
	set := func(v string) error {
		patch, err := a.settingsPatch(map[string]json.RawMessage{"extraPorts": json.RawMessage(v)}, true)
		if err != nil {
			return err
		}
		return a.updateSettings(func(s *settings.Settings) { _ = json.Unmarshal(patch, s) })
	}
	if !strings.Contains(strings.Join(editable(a.settings.Get()), " "), "extraPorts") {
		t.Fatal("extraPorts is not a setting the CLI can change")
	}
	if err := set(fmt.Sprintf(`[{"port":%d,"route":"ai"}]`, port)); err != nil {
		t.Fatal(err)
	}
	listener := fmt.Sprintf("name: %s", core.ExtraPortListener(port))
	if cfg := coreConfig(t, a); !strings.Contains(cfg, listener) || !strings.Contains(cfg, "proxy: AI 服务") {
		t.Fatalf("the listener is not at the core:\n%s", cfg)
	}
	c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("nothing listens on the extra port: %v", err)
	}
	c.Close()

	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	for name, v := range map[string]string{
		"held by another program": fmt.Sprintf(`[{"port":%d,"route":"ai"},{"port":%d,"route":"media"}]`, port, held.Addr().(*net.TCPAddr).Port),
		"bowel's own":             fmt.Sprintf(`[{"port":%d,"route":"ai"},{"port":%d,"route":"media"}]`, port, a.probePort),
		"the mixed port":          fmt.Sprintf(`[{"port":%d,"route":"ai"}]`, a.settings.Get().MixedPort),
		"no such group":           fmt.Sprintf(`[{"port":%d,"route":"nope"}]`, port),
		"not a list of ports":     `["7901"]`,
	} {
		if err := set(v); err == nil {
			t.Errorf("%s: should be refused", name)
		}
		if got := a.settings.Get().ExtraPorts; len(got) != 1 || got[0] != (core.ExtraPort{Port: port, Route: "ai"}) {
			t.Fatalf("%s: settings changed to %+v", name, got)
		}
	}

	// A group of the user's, a port for it, and the group deleted.
	var gid string
	if err := a.updateSettings(func(s *settings.Settings) { gid = s.Routing.AddGroup(s.Rules, "工作") }); err != nil {
		t.Fatal(err)
	}
	if err := set(fmt.Sprintf(`[{"port":%d,"route":%q}]`, port, gid)); err != nil {
		t.Fatal(err)
	}
	if cfg := coreConfig(t, a); !strings.Contains(cfg, "proxy: "+gid) {
		t.Fatalf("the port did not follow to the new group:\n%s", cfg)
	}
	if err := a.updateSettings(func(s *settings.Settings) { s.Routing.DeleteGroup(s.Rules, gid, "media") }); err != nil {
		t.Fatalf("deleting a group with a port: %v", err)
	}
	if cfg := coreConfig(t, a); strings.Contains(cfg, listener) {
		t.Errorf("the port of a deleted group is still open:\n%s", cfg)
	}
	if err := set(`[]`); err != nil {
		t.Fatal(err)
	}
}

func freePortFor(t *testing.T) int {
	t.Helper()
	p, err := core.FreePort()
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// TestExtraPortsOfImportedRouting: while a provider's own routing is in use,
// an extra port is given one of that config's groups by name, and what comes
// in by it leaves by that group whatever the rules say. A name the config
// doesn't have is refused. A port whose group an update takes away closes
// and opens again when the group is back; with bowel's routing again, which
// has no such group, it is closed, and the settings keep it all along.
func TestExtraPortsOfImportedRouting(t *testing.T) {
	a, id, serve := importedApp(t)
	ctx := context.Background()
	port := freePortFor(t)
	set := func(v string) error {
		patch, err := a.settingsPatch(map[string]json.RawMessage{"extraPorts": json.RawMessage(v)}, true)
		if err != nil {
			return err
		}
		return a.updateSettings(func(s *settings.Settings) { _ = json.Unmarshal(patch, s) })
	}
	streaming := fmt.Sprintf(`[{"port":%d,"route":"Streaming"}]`, port)
	listener := fmt.Sprintf("name: %s", core.ExtraPortListener(port))
	open := func() bool {
		c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			return false
		}
		c.Close()
		return true
	}
	kept := func(when string) {
		t.Helper()
		if got := a.settings.Get().ExtraPorts; len(got) != 1 || got[0] != (core.ExtraPort{Port: port, Route: "Streaming"}) {
			t.Fatalf("%s: settings changed to %+v", when, got)
		}
	}

	// The config's groups are no one's until its routing is in use.
	if err := set(streaming); err == nil {
		t.Fatal("a group of a config that is not in use should be refused")
	}
	if err := a.setImportedRouting(id); err != nil {
		t.Fatal(err)
	}
	if err := set(streaming); err != nil {
		t.Fatal(err)
	}
	if cfg := coreConfig(t, a); !strings.Contains(cfg, listener) || !strings.Contains(cfg, "proxy: Streaming") {
		t.Fatalf("the listener is not at the core:\n%s", cfg)
	}
	if !open() {
		t.Fatal("nothing listens on the extra port")
	}
	for name, v := range map[string]string{
		"no such group": fmt.Sprintf(`[{"port":%d,"route":"Nope"}]`, port),
		"a node":        fmt.Sprintf(`[{"port":%d,"route":"JP-01"}]`, port),
	} {
		if err := set(v); err == nil {
			t.Errorf("%s: should be refused", name)
		}
		kept(name)
	}

	// A host no rule names would leave by MATCH's Japan; by this port it
	// leaves by Streaming.
	if err := a.selectMember("Streaming", "DIRECT"); err != nil {
		t.Fatal(err)
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var snap mihomo.Snapshot
		_ = a.coreCall(5e9, func(ctx context.Context, c *mihomo.Client) (err error) { snap, err = c.Connections(ctx); return err })
		b, _ := json.Marshal(snap)
		_, _ = w.Write(b)
	}))
	defer target.Close()
	pu, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	hc := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(pu), DisableKeepAlives: true}}
	resp, err := hc.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	var got mihomo.Snapshot
	err = json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(got.Connections, func(c mihomo.Connection) bool {
		return slices.Contains(c.Chains, "Streaming") && !slices.Contains(c.Chains, "Japan")
	}) {
		t.Errorf("no connection through the port's group: %+v", got.Connections)
	}

	// An update without the group, and one with it again.
	serve(strings.Replace(strings.Replace(issue33Sub, "name: Streaming", "name: Video", 1), "netflix.com,Streaming", "netflix.com,Video", 1))
	if err := a.updateProvider(ctx, id); err != nil {
		t.Fatal(err)
	}
	if cfg := coreConfig(t, a); strings.Contains(cfg, listener) || open() {
		t.Errorf("the port of a group the update took away is still open:\n%s", cfg)
	}
	kept("group gone")
	serve(issue33Sub)
	if err := a.updateProvider(ctx, id); err != nil {
		t.Fatal(err)
	}
	if cfg := coreConfig(t, a); !strings.Contains(cfg, listener) || !open() {
		t.Errorf("the port did not open again with its group:\n%s", cfg)
	}

	// bowel's routing has no group of that name.
	if err := a.setImportedRouting(""); err != nil {
		t.Fatal(err)
	}
	if cfg := coreConfig(t, a); strings.Contains(cfg, listener) || open() {
		t.Errorf("the port is open with bowel's routing:\n%s", cfg)
	}
	kept("bowel's routing")
	if err := set(`[]`); err != nil {
		t.Fatal(err)
	}
}

// TestExtraPortsLookUpNewEntries: saving the settings reads the config whose
// routing is in use only for an extra port that was not there before, whose
// group it has to find. Another change, with the ports as they were, leaves
// the config unread.
func TestExtraPortsLookUpNewEntries(t *testing.T) {
	dir, err := os.MkdirTemp("", "mh")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	mygo.App.SetPath(mygo.PathUserData, dir)
	defer mygo.App.SetPath(mygo.PathUserData, "")
	a, err := newApp()
	if err != nil {
		t.Fatal(err)
	}
	read := func() bool {
		a.routesMu.Lock()
		defer a.routesMu.Unlock()
		return a.imp.loaded
	}
	had := core.ExtraPort{Port: freePortFor(t), Route: "ai"}
	if _, err := a.settings.Update(func(s *settings.Settings) { s.ExtraPorts = []core.ExtraPort{had} }); err != nil {
		t.Fatal(err)
	}
	a.dropImported()
	if err := a.updateSettings(func(s *settings.Settings) { s.StartHidden = !s.StartHidden }); err != nil {
		t.Fatal(err)
	}
	if read() {
		t.Error("the config was read for a change that adds no port")
	}
	// A new entry is looked up before anything is saved: here it is refused.
	added := core.ExtraPort{Port: freePortFor(t), Route: "nope"}
	if err := a.updateSettings(func(s *settings.Settings) { s.ExtraPorts = append(s.ExtraPorts, added) }); err == nil {
		t.Error("a port for a group that doesn't exist should be refused")
	}
	if !read() {
		t.Error("the config was not read for a new port")
	}
	if got := a.settings.Get().ExtraPorts; len(got) != 1 || got[0] != had {
		t.Errorf("settings changed to %+v", got)
	}
}
