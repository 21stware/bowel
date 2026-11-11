package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oboo/moho/internal/mihomo"
)

// Issue #101: a widely used template (anchors and merge keys, its own
// `type: http` subscriptions, include-all groups, mrs rule sets, a logical
// rule) could not be added. testdata/issue101.yaml is the config as it was
// pasted there, with its two secrets replaced.

func issue101Config(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "issue101.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// issue101Filled is the config with one subscription filled in the way its
// comment shows, and its rule sets taken from sets instead of GitHub.
func issue101Filled(t *testing.T, sets string) string {
	t.Helper()
	nodes := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "proxies:\n  - {name: 香港 01, type: direct}\n  - {name: US 02, type: direct}\n  - {name: 日本 03, type: direct}\n  - {name: 官网 example.com, type: direct}\n")
	}))
	t.Cleanup(nodes.Close)
	src := issue101Config(t)
	filled := strings.Replace(src, "# Primary: {<<: *base-provider, url: 'YOUR_PRIMARY_URL'", "Primary: {<<: *base-provider, url: '"+nodes.URL+"/sub'", 1)
	if filled == src {
		t.Fatal("the template's subscription line has changed")
	}
	return strings.ReplaceAll(filled, "https://github.com/666OS/rules/raw/release", sets)
}

// mrsRuleSets serves rule sets in the core's binary format, made by the
// core itself; hang, while set, keeps every download waiting.
func mrsRuleSets(t *testing.T, hang *atomic.Bool) string {
	t.Helper()
	dir := t.TempDir()
	made := map[string][]byte{}
	for behavior, line := range map[string]string{"domain": "+.example.com\n", "ipcidr": "10.1.2.0/24\n"} {
		src, dst := filepath.Join(dir, behavior+".txt"), filepath.Join(dir, behavior+".mrs")
		if err := os.WriteFile(src, []byte(line), 0o600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(bundledCore(), "convert-ruleset", behavior, "text", src, dst).CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		b, err := os.ReadFile(dst)
		if err != nil {
			t.Fatal(err)
		}
		made[behavior] = b
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for hang != nil && hang.Load() {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
		behavior := "domain"
		if strings.Contains(r.URL.Path, "/ip/") {
			behavior = "ipcidr"
		}
		_, _ = w.Write(made[behavior])
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// As pasted, every node line is a comment: there is nothing to route to,
// and the file says so instead of becoming a Provider without nodes.
func TestIssue101AsPastedHasNoNodes(t *testing.T) {
	a, _ := routingApp(t, nil)
	before := len(a.providers.List())
	_, err := a.addProvider(context.Background(), issue101Config(t))
	if err == nil || !strings.Contains(err.Error(), "一个节点都没有") || !strings.Contains(err.Error(), "proxy-providers") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "内核") {
		t.Errorf("blames the core: %v", err)
	}
	if n := len(a.providers.List()); n != before {
		t.Errorf("providers: %d, were %d", n, before)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(issue101Config(t)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.addFileAs(path, "config"); err == nil || !strings.Contains(err.Error(), "一个节点都没有") {
		t.Errorf("as a file: %v", err)
	}
}

// With a subscription filled in, the config is added, and used as it is.
func TestIssue101FilledIn(t *testing.T) {
	a, _ := routingApp(t, nil)
	v, err := a.addProvider(context.Background(), issue101Filled(t, mrsRuleSets(t, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if r := v.Routing; r.Groups != 18 || r.Rules != 21 || r.Remote != 1 || r.Problem != "" {
		t.Errorf("routing = %+v", r)
	}
	// The file has no node of its own, so the core is not given it to read.
	providers := func() map[string]mihomo.ProxyProvider {
		var pp map[string]mihomo.ProxyProvider
		if err := a.coreCall(5*time.Second, func(ctx context.Context, c *mihomo.Client) (err error) {
			pp, err = c.ProxyProviders(ctx)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return pp
	}
	if _, ok := providers()["p-"+v.ID]; ok {
		t.Error("the config was given to the core as a node file")
	}

	if err := a.setImportedRouting(v.ID); err != nil {
		t.Fatal(err)
	}
	if a.core.Loading() {
		t.Error("still loading")
	}
	px := coreProxies(t, a)
	// include-all takes the subscription's nodes, under its prefix and
	// without what its filter leaves out; a region's look-behind filter
	// keeps its own, and one with none falls back as the config says.
	for group, want := range map[string][]string{
		"全球手动": {"[P] 香港 01", "[P] US 02", "[P] 日本 03"},
		"香港策略": {"[P] 香港 01"},
		"美国策略": {"[P] US 02"},
		"台湾策略": {"REJECT"},
		"故障转移": {"美国策略", "日本策略", "狮城策略", "韩国策略", "香港策略", "台湾策略"},
	} {
		if got := px[group].All; !slices.Equal(got, want) {
			t.Errorf("%s = %v, want %v", group, got, want)
		}
	}
	if len(providers()["Primary"].Proxies) != 3 {
		t.Errorf("Primary = %+v", providers()["Primary"])
	}
	var sets map[string]mihomo.RuleProvider
	if err := a.coreCall(5*time.Second, func(ctx context.Context, c *mihomo.Client) (err error) {
		sets, err = c.RuleProviders(ctx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// The rules name 19 of the config's 24 rule sets; the rest stay out.
	if len(sets) != 19 {
		t.Errorf("%d rule sets", len(sets))
	}
	for name, s := range sets {
		if s.RuleCount == 0 {
			t.Errorf("rule set %s is empty", name)
		}
	}
	a.refresh()
	snap := a.snapshot()
	if snap.Imported == nil || len(snap.Imported.Groups) != 18 {
		t.Fatalf("imported = %+v", snap.Imported)
	}
	if is := a.importedIssue(a.settings.Get(), snap); is != nil {
		t.Errorf("issue = %+v", is)
	}
	for _, l := range a.core.Logs() {
		if strings.Contains(l, "must have a `proxies` field") || strings.Contains(l, "level=error") {
			t.Errorf("the core complained: %s", l)
		}
	}
}

// What the reporter saw. The core answers a reload only after it has tried
// to download every rule set and subscription the config names, and bowel
// took a wait that ran out for a refusal: with this config in use and
// GitHub out of reach, adding any Provider ended in "内核无法读取这个
// Provider：Put "http://mihomo/configs?force=true": context deadline
// exceeded", and the Provider was taken away again.
func TestIssue101SlowDownloadsAreNotARefusal(t *testing.T) {
	a, _ := routingApp(t, nil)
	a.core.ReloadWait = 2 * time.Second
	var hang atomic.Bool
	hang.Store(true)
	defer hang.Store(false)
	v, err := a.addProvider(context.Background(), issue101Filled(t, mrsRuleSets(t, &hang)))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.setImportedRouting(v.ID); err != nil {
		t.Fatalf("use: %v", err)
	}
	if a.settings.Get().ImportedRouting != v.ID {
		t.Fatal("not in use")
	}
	if !a.core.Loading() {
		t.Error("the core is not said to be loading")
	}
	other, err := a.addProvider(context.Background(), "proxies:\n  - {name: X, type: direct}\n")
	if err != nil {
		t.Fatalf("adding a Provider meanwhile: %v", err)
	}
	if _, ok := a.providers.Get(other.ID); !ok {
		t.Error("the Provider added meanwhile is gone")
	}

	// What has not arrived is said, with where it comes from.
	deadline := time.Now().Add(10 * time.Second)
	var is *Issue
	for time.Now().Before(deadline) {
		a.mu.Lock()
		a.nodesAt = time.Time{}
		a.mu.Unlock()
		a.refresh()
		if is = a.importedIssue(a.settings.Get(), a.snapshot()); is != nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if is == nil || !strings.Contains(is.Text, "规则集还没下载下来") || !strings.Contains(is.Detail, "127.0.0.1") || !strings.Contains(is.Detail, "Private") {
		t.Fatalf("issue = %+v", is)
	}

	// Once they can be reached, the next load of the config has them.
	hang.Store(false)
	deadline = time.Now().Add(40 * time.Second)
	for {
		if err := a.applyConfig(); err != nil {
			t.Fatal(err)
		}
		a.refresh()
		if !a.core.Loading() && a.importedIssue(a.settings.Get(), a.snapshot()) == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("loading = %v, issue = %+v", a.core.Loading(), a.importedIssue(a.settings.Get(), a.snapshot()))
		}
		time.Sleep(500 * time.Millisecond)
	}
}
