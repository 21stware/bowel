package bridge

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oboo/moho/internal/locate"
	"github.com/oboo/moho/internal/mihomo"
	"github.com/oboo/moho/internal/monitor"
	"github.com/oboo/moho/internal/provider"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/ruleset"
	"gopkg.in/yaml.v3"
)

// chinaDNS is the list of the report this work answers: four classical rules
// in a YAML payload, and no DNS settings.
const chinaDNS = `# NAME: ChinaDNS
# TOTAL: 4
payload:
  - DOMAIN-SUFFIX,dns.alidns.com
  - DOMAIN-SUFFIX,doh.360.cn
  - DOMAIN-SUFFIX,doh.pub
  - IP-CIDR,1.12.12.12/32
`

var chinaDNSRules = []string{"DOMAIN-SUFFIX,dns.alidns.com", "DOMAIN-SUFFIX,doh.360.cn", "DOMAIN-SUFFIX,doh.pub", "IP-CIDR,1.12.12.12/32"}

// page is what the list's server answers, which a test changes as it goes.
type page struct {
	mu     sync.Mutex
	status int
	body   string
}

func (p *page) set(status int, body string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status, p.body = status, body
}

func serve(t *testing.T, body string) (*page, string) {
	t.Helper()
	p := &page{status: http.StatusOK, body: body}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		w.WriteHeader(p.status)
		_, _ = io.WriteString(w, p.body)
	}))
	t.Cleanup(srv.Close)
	return p, srv.URL
}

func rulesOfConfig(t *testing.T, x *bridge) (map[string]any, []string) {
	t.Helper()
	cfg := config(t, x)
	var rules []string
	for _, r := range cfg["rules"].([]any) {
		rules = append(rules, r.(string))
	}
	return cfg, rules
}

func payloadOf(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var y struct {
		Payload []string `yaml:"payload"`
	}
	if err := yaml.Unmarshal(raw, &y); err != nil {
		t.Fatal(err)
	}
	return y.Payload
}

func newGroup(t *testing.T, x *bridge, name string) groupData {
	t.Helper()
	if err := x.routeOp(context.Background(), callArgs{Op: "newGroup", Name: name}); err != nil {
		t.Fatal(err)
	}
	return group(t, x, name)
}

func service(t *testing.T, x *bridge, id string) (serviceData, bool) {
	t.Helper()
	for _, g := range x.routing().Groups {
		for _, s := range g.Services {
			if s.ID == id {
				return s, true
			}
		}
	}
	return serviceData{}, false
}

func importChinaDNS(t *testing.T, x *bridge, route string) (id, link string, p *page) {
	t.Helper()
	p, base := serve(t, chinaDNS)
	link = base + "/rule/ChinaDNS.yaml"
	id, err := x.importRuleSet(context.Background(), callArgs{Input: link, Route: route})
	if err != nil {
		t.Fatal(err)
	}
	return id, link, p
}

// A list checked and imported on the phone reaches the core: as a file the
// core reads, and a rule sending what it matches to the chosen group, after
// the user's own rules and before the built-in ones and the last rule.
func TestRuleSetReachesConfig(t *testing.T) {
	x := testBridge(t)
	ctx := context.Background()
	g := newGroup(t, x, "国内 DNS")
	if err := x.routeOp(ctx, callArgs{Op: "addRule", Kind: routes.KindSuffix, Text: "mine.example.com", Route: g.ID}); err != nil {
		t.Fatal(err)
	}
	_, base := serve(t, chinaDNS)
	link := base + "/rule/ChinaDNS.yaml"

	seen, err := x.checkRuleSet(ctx, callArgs{Input: link})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(base)
	if seen.Count != 4 || seen.Skipped != 0 || seen.Name != "ChinaDNS" || seen.Where != u.Host || len(seen.Rules) != 4 || seen.Dup != nil || len(seen.Targets) != 0 {
		t.Fatalf("check = %+v", seen)
	}
	if _, rules := rulesOfConfig(t, x); slices.ContainsFunc(rules, func(r string) bool { return strings.HasPrefix(r, "RULE-SET") }) {
		t.Fatal("looking at a list already changed the config")
	}

	id, err := x.importRuleSet(ctx, callArgs{Input: link, Route: g.ID})
	if err != nil {
		t.Fatal(err)
	}
	cfg, rules := rulesOfConfig(t, x)
	path := filepath.Join(x.dir, "rulesets", id+".yaml")
	prov, _ := cfg["rule-providers"].(map[string]any)["rs-"+id].(map[string]any)
	if prov["type"] != "file" || prov["behavior"] != "classical" || prov["format"] != "yaml" || prov["path"] != path {
		t.Fatalf("rule-provider = %v, want a classical yaml file at %s", prov, path)
	}
	if got := payloadOf(t, path); !slices.Equal(got, chinaDNSRules) {
		t.Fatalf("the file holds %v", got)
	}
	mine := slices.IndexFunc(rules, func(r string) bool { return strings.HasPrefix(r, "DOMAIN-SUFFIX,mine.example.com,") })
	if mine < 0 {
		t.Fatalf("the user's rule is gone: %v", rules)
	}
	target := strings.TrimPrefix(rules[mine], "DOMAIN-SUFFIX,mine.example.com,")
	set := slices.Index(rules, "RULE-SET,rs-"+id+","+target+",no-resolve")
	builtin := slices.IndexFunc(rules, func(r string) bool { return strings.HasPrefix(r, "GEOSITE,") || strings.HasPrefix(r, "GEOIP,") })
	last := slices.IndexFunc(rules, func(r string) bool { return strings.HasPrefix(r, "MATCH,") })
	if set < 0 || !(mine < set && set < builtin && builtin < last) || last != len(rules)-1 {
		t.Fatalf("order: mine %d, set %d, built-in %d, MATCH %d of %d\n%v", mine, set, builtin, last, len(rules), rules)
	}

	s, ok := service(t, x, id)
	if !ok || s.Set == nil || s.Name != "ChinaDNS" || !s.Set.Have || s.Set.Count != 4 || s.Set.Where != u.Host || s.Set.Local || s.Set.UpdatedAt == 0 || len(s.Set.Preview) != 4 || len(s.Rules) != 0 {
		t.Fatalf("the page reads %+v %+v", s, s.Set)
	}
	if got := group(t, x, "国内 DNS"); !slices.ContainsFunc(got.Services, func(o serviceData) bool { return o.ID == id }) {
		t.Fatalf("the set is not in its group: %+v", got.Services)
	}
}

// What is no list is refused with its reason, and nothing is added: no
// service, no rule for the link's own domain.
func TestRuleSetRefusesWhatIsNoList(t *testing.T) {
	x := testBridge(t)
	ctx := context.Background()
	before := x.set.Get()
	p, base := serve(t, "")
	link := base + "/rule/ChinaDNS.yaml"
	for name, c := range map[string]struct {
		status int
		body   string
		want   string
	}{
		"not found": {http.StatusNotFound, "404: Not Found", "HTTP 404"},
		"html":      {http.StatusOK, "<html><body>Sign in</body></html>\n", "没有找到可用的规则"},
		"mrs":       {http.StatusOK, "MRS\x01\x00\xff\xfe", ".mrs"},
		"empty":     {http.StatusOK, "payload: []\n", "没有找到可用的规则"},
	} {
		p.set(c.status, c.body)
		if got, err := x.checkRuleSet(ctx, callArgs{Input: link}); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: check = %+v, %v; want an error with %q", name, got, err, c.want)
		}
		if id, err := x.importRuleSet(ctx, callArgs{Input: link, Route: routes.Default}); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: import = %q, %v; want an error with %q", name, id, err, c.want)
		}
	}
	for _, bad := range []string{"", "ChinaDNS.yaml", "ftp://x.example/a.yaml"} {
		if _, err := x.importRuleSet(ctx, callArgs{Input: bad, Route: routes.Default}); err == nil {
			t.Errorf("%q was imported", bad)
		}
	}
	after := x.set.Get()
	if len(after.Rules) != len(before.Rules) || len(after.Routing.Services) != len(before.Routing.Services) {
		t.Fatalf("a refused list left something behind: rules %v, services %+v", after.Rules, after.Routing.Services)
	}
	if entries, _ := os.ReadDir(filepath.Join(x.dir, "rulesets")); slices.ContainsFunc(entries, func(e os.DirEntry) bool { return strings.HasSuffix(e.Name(), ".yaml") }) {
		t.Fatalf("a refused list left a file: %v", entries)
	}
	if _, err := x.importRuleSet(ctx, callArgs{Input: link, Route: "nowhere"}); err == nil {
		t.Error("imported to a group that isn't there")
	}
}

// An update that fails keeps the last good rules in use, and says so.
func TestRuleSetUpdateFailureKeepsRules(t *testing.T) {
	x := testBridge(t)
	ctx := context.Background()
	id, _, p := importChinaDNS(t, x, routes.Default)
	path := filepath.Join(x.dir, "rulesets", id+".yaml")
	for _, bad := range []struct {
		status int
		body   string
	}{{http.StatusInternalServerError, "oops"}, {http.StatusOK, "<html></html>"}, {http.StatusOK, "mixed-port: 7890\nrules:\n  - MATCH,DIRECT\n"}} {
		p.set(bad.status, bad.body)
		if err := x.updateRuleSet(ctx, id); err == nil {
			t.Fatalf("update of %q succeeded", bad.body)
		}
		s, _ := service(t, x, id)
		if s.Set == nil || s.Set.Error == "" || s.Set.Count != 4 {
			t.Fatalf("after %q the page reads %+v", bad.body, s.Set)
		}
		if got := payloadOf(t, path); !slices.Equal(got, chinaDNSRules) {
			t.Fatalf("after %q the file holds %v", bad.body, got)
		}
		if cfg, _ := rulesOfConfig(t, x); cfg["rule-providers"].(map[string]any)["rs-"+id] == nil {
			t.Fatalf("after %q the set left the config", bad.body)
		}
	}
	p.set(http.StatusOK, "payload:\n  - DOMAIN-SUFFIX,doh.pub\n  - DOMAIN-SUFFIX,dns.google\n")
	if err := x.updateRuleSet(ctx, id); err != nil {
		t.Fatal(err)
	}
	s, _ := service(t, x, id)
	if s.Set.Error != "" || s.Set.Count != 2 || !slices.Equal(payloadOf(t, path), []string{"DOMAIN-SUFFIX,doh.pub", "DOMAIN-SUFFIX,dns.google"}) {
		t.Fatalf("a good update: %+v", s.Set)
	}
}

// Deleting a set takes its service, its lines in the config and its file.
func TestRuleSetRemoved(t *testing.T) {
	x := testBridge(t)
	ctx := context.Background()
	id, _, _ := importChinaDNS(t, x, routes.Default)
	path := filepath.Join(x.dir, "rulesets", id+".yaml")
	if err := x.routeOp(ctx, callArgs{Op: "removeService", ID: id}); err != nil {
		t.Fatal(err)
	}
	if _, ok := service(t, x, id); ok {
		t.Fatal("the service stayed")
	}
	cfg, rules := rulesOfConfig(t, x)
	if cfg["rule-providers"] != nil || slices.ContainsFunc(rules, func(r string) bool { return strings.HasPrefix(r, "RULE-SET") }) {
		t.Fatalf("the config still names it: %v", cfg["rule-providers"])
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the file stayed: %v", err)
	}
	if err := x.updateRuleSet(ctx, id); err == nil {
		t.Fatal("a deleted set was updated")
	}
}

// A set switched off leaves the config, and comes back as it was.
func TestRuleSetSwitchedOff(t *testing.T) {
	x := testBridge(t)
	ctx := context.Background()
	id, _, _ := importChinaDNS(t, x, routes.Default)
	has := func() bool {
		_, rules := rulesOfConfig(t, x)
		return slices.ContainsFunc(rules, func(r string) bool { return strings.HasPrefix(r, "RULE-SET,rs-"+id+",") })
	}
	if err := x.routeOp(ctx, callArgs{Op: "toggleService", ID: id}); err != nil {
		t.Fatal(err)
	}
	if has() {
		t.Fatal("a set that is off is still in the config")
	}
	if err := x.routeOp(ctx, callArgs{Op: "toggleService", ID: id}); err != nil {
		t.Fatal(err)
	}
	if !has() {
		t.Fatal("the set did not come back")
	}
}

// 测试一个地址 looks inside the sets, and leads to the set.
func TestExplainLooksInsideRuleSets(t *testing.T) {
	x := testBridge(t)
	g := newGroup(t, x, "国内 DNS")
	id, _, _ := importChinaDNS(t, x, g.ID)
	for _, host := range []string{"doh.pub", "a.dns.alidns.com", "1.12.12.12"} {
		if got := x.explain(host); got.Route != g.ID || got.ServiceID != id || got.Service != "ChinaDNS" {
			t.Errorf("explain(%s) = %+v, want the set in %s", host, got, g.ID)
		}
	}
	if got := x.ruleHome("RuleSet", "rs-"+id); got.Group != g.ID || got.Service != id {
		t.Errorf("home of the set = %+v", got)
	}
	// The activity page names the set, not the file the core knows it by.
	if got := ruleWords(x.routes(), "RuleSet", "rs-"+id); got != "规则集 ChinaDNS" {
		t.Errorf("a connection the set matched reads %q", got)
	}
}

// The same link twice is refused, and the check says where it already is.
func TestRuleSetTwice(t *testing.T) {
	x := testBridge(t)
	ctx := context.Background()
	g := newGroup(t, x, "国内 DNS")
	id, link, _ := importChinaDNS(t, x, g.ID)
	seen, err := x.checkRuleSet(ctx, callArgs{Input: link})
	if err != nil || seen.Dup == nil || seen.Dup.Group != g.ID || seen.Dup.Service != id || seen.Dup.GroupName != "国内 DNS" {
		t.Fatalf("check = %+v, %v", seen.Dup, err)
	}
	if again, err := x.importRuleSet(ctx, callArgs{Input: link, Route: routes.Default}); err == nil {
		t.Fatalf("imported again as %s", again)
	}
	if n := len(x.set.Get().Routing.Services); n != 1 {
		t.Fatalf("%d services", n)
	}
}

// A link's set is downloaded again when it falls due; one that is off, or
// came from a file, is left alone.
func TestRuleSetsUpdateWhenDue(t *testing.T) {
	x := testBridge(t)
	ctx := context.Background()
	id, _, p := importChinaDNS(t, x, routes.Default)
	off, _, pOff := importChinaDNS(t, x, routes.Default)
	if err := x.routeOp(ctx, callArgs{Op: "toggleService", ID: off}); err != nil {
		t.Fatal(err)
	}
	next := "payload:\n  - DOMAIN-SUFFIX,doh.pub\n"
	p.set(http.StatusOK, next)
	pOff.set(http.StatusOK, next)
	count := func(id string) int {
		s, _ := service(t, x, id)
		return s.Set.Count
	}
	x.updateRuleSets(time.Now().Add(time.Hour))
	if count(id) != 4 {
		t.Fatal("updated before it was due")
	}
	x.updateRuleSets(time.Now().Add(ruleset.UpdateEvery + time.Minute))
	if count(id) != 1 || count(off) != 4 {
		t.Fatalf("due: %d rules, and %d in the one that is off", count(id), count(off))
	}
}

// A set is still there when the app is opened again, and a file no service
// names is cleared.
func TestRuleSetSurvivesRestart(t *testing.T) {
	x := testBridge(t)
	id, _, _ := importChinaDNS(t, x, routes.Default)
	orphan := filepath.Join(x.dir, "rulesets", "orphan.yaml")
	if err := os.WriteFile(orphan, []byte("payload:\n  - x.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := provider.Open(filepath.Join(x.dir, "providers"))
	if err != nil {
		t.Fatal(err)
	}
	set, err := openSettings(x.dir)
	if err != nil {
		t.Fatal(err)
	}
	y := &bridge{dir: x.dir, store: store, set: set, loc: locate.New(""), sink: discard{}, vpn: "off", lastNode: map[string]string{}}
	if err := y.openRuleSets(); err != nil {
		t.Fatal(err)
	}
	s, ok := service(t, y, id)
	if !ok || s.Set == nil || !s.Set.Have || s.Set.Count != 4 || len(s.Set.Preview) != 4 {
		t.Fatalf("after a restart the page reads %+v %+v", s, s.Set)
	}
	if cfg, _ := rulesOfConfig(t, y); cfg["rule-providers"].(map[string]any)["rs-"+id] == nil {
		t.Fatal("after a restart the config lost the set")
	}
	if got := y.explain("doh.pub"); got.ServiceID != id {
		t.Fatalf("after a restart explain = %+v", got)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatal("a file no service names stayed")
	}
}

// A file picked on the phone is copied once: the set keeps its name, not the
// path of a copy that goes with the next pick, and has nothing to update from.
func TestRuleSetFromFile(t *testing.T) {
	x := testBridge(t)
	ctx := context.Background()
	picked := filepath.Join(t.TempDir(), "ChinaDNS.list")
	if err := os.WriteFile(picked, []byte("DOMAIN-SUFFIX,doh.pub,DIRECT\nUSER-AGENT,x\n.dns.alidns.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	seen, err := x.checkRuleSet(ctx, callArgs{Path: picked})
	if err != nil || seen.Count != 1 || seen.Skipped != 2 || seen.Where != "ChinaDNS.list" || seen.Name != "ChinaDNS" {
		t.Fatalf("check = %+v, %v", seen, err)
	}
	if err := os.Remove(picked); err != nil { // the next pick clears the copy
		t.Fatal(err)
	}
	id, err := x.importRuleSet(ctx, callArgs{Path: picked, Route: routes.Default})
	if err != nil {
		t.Fatal(err)
	}
	s, _ := service(t, x, id)
	if s.Set == nil || !s.Set.Local || s.Set.Where != "ChinaDNS.list" || s.Set.Count != 1 || s.Set.Skipped != 2 {
		t.Fatalf("the page reads %+v", s.Set)
	}
	for _, sv := range x.set.Get().Routing.Services {
		if sv.ID == id && sv.Source != "ChinaDNS.list" {
			t.Fatalf("source = %q, want the file's name only", sv.Source)
		}
	}
	if err := x.updateRuleSet(ctx, id); err == nil {
		t.Fatal("a set from a file was updated")
	}
	if got := payloadOf(t, filepath.Join(x.dir, "rulesets", id+".yaml")); !slices.Equal(got, []string{"DOMAIN-SUFFIX,doh.pub"}) {
		t.Fatalf("a refused update changed the file: %v", got)
	}
	x.updateRuleSets(time.Now().Add(ruleset.UpdateEvery + time.Minute))
	if s, _ := service(t, x, id); s.Set.Error != "" {
		t.Fatalf("a file's set was tried when due: %s", s.Set.Error)
	}
}

const wholeConfig = `mixed-port: 7890
dns:
  enable: true
  nameserver: [203.0.113.53]
proxies:
  - {name: a, type: ss, server: 192.0.2.1, port: 1, cipher: aes-128-gcm, password: x}
proxy-groups:
  - {name: 节点选择, type: select, proxies: [a]}
rule-providers:
  ads: {type: http, behavior: domain, url: https://example.com/ads.yaml, path: ./ads.yaml}
rules:
  - DOMAIN-SUFFIX,elsewhere.example,节点选择
  - DOMAIN-SUFFIX,doh.pub,DIRECT
  - IP-CIDR,1.12.12.12/32,DIRECT,no-resolve
  - RULE-SET,ads,REJECT
  - MATCH,节点选择
`

// A whole config is recognised, not read as a broken list: the check lists
// what it sends where, one target's rules are imported to a group chosen
// here, and nothing else of the config comes along.
func TestRuleSetFromWholeConfig(t *testing.T) {
	x := testBridge(t)
	ctx := context.Background()
	g := newGroup(t, x, "国内 DNS")
	p, base := serve(t, wholeConfig)
	link := base + "/sub?token=1"
	before := x.set.Get()

	seen, err := x.checkRuleSet(ctx, callArgs{Input: link})
	if err != nil {
		t.Fatal(err)
	}
	want := []targetData{{Name: "节点选择", Count: 1, Skipped: 1}, {Name: "DIRECT", Count: 2}}
	if !slices.Equal(seen.Targets, want) || seen.Left != 2 || seen.Count != 0 {
		t.Fatalf("check = %+v, want the targets %+v and 2 left", seen, want)
	}
	if _, err := x.importRuleSet(ctx, callArgs{Input: link, Route: g.ID}); err == nil {
		t.Fatal("a whole config was imported without choosing a target")
	}

	one, err := x.checkRuleSet(ctx, callArgs{Input: link, Text: "DIRECT"})
	if err != nil || one.Count != 2 || one.Target != "DIRECT" || one.Name != "DIRECT" || len(one.Targets) != 0 {
		t.Fatalf("check of DIRECT = %+v, %v", one, err)
	}
	id, err := x.importRuleSet(ctx, callArgs{Input: link, Text: "DIRECT", Route: g.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got := payloadOf(t, filepath.Join(x.dir, "rulesets", id+".yaml")); !slices.Equal(got, []string{"DOMAIN-SUFFIX,doh.pub", "IP-CIDR,1.12.12.12/32,no-resolve"}) {
		t.Fatalf("the file holds %v", got)
	}
	cfg, rules := rulesOfConfig(t, x)
	raw, _ := json.Marshal(cfg)
	for _, foreign := range []string{"节点选择", "elsewhere.example", "example.com/ads.yaml", "203.0.113.53", "7890"} {
		if strings.Contains(string(raw), foreign) {
			t.Errorf("%q of the source config reached ours", foreign)
		}
	}
	if len(cfg["rule-providers"].(map[string]any)) != 1 || !slices.ContainsFunc(rules, func(r string) bool { return strings.HasPrefix(r, "RULE-SET,rs-"+id+",") }) {
		t.Fatalf("rule-providers = %v", cfg["rule-providers"])
	}
	after := x.set.Get()
	if after.Options.DNS != before.Options.DNS || after.Mode != before.Mode || len(after.Routing.Groups) != len(before.Routing.Groups) {
		t.Fatal("importing rules changed DNS, the mode or the groups")
	}
	if got := x.explain("doh.pub"); got.Route != g.ID || got.ServiceID != id {
		t.Fatalf("explain = %+v", got)
	}
	s, _ := service(t, x, id)
	if s.Set.Target != "DIRECT" || s.Set.Skipped != 0 {
		t.Fatalf("the page reads %+v", s.Set)
	}

	// The source dropping the target is a failed update, not an empty set.
	p.set(http.StatusOK, "proxies: []\nrules:\n  - MATCH,DIRECT\n")
	if err := x.updateRuleSet(ctx, id); err == nil {
		t.Fatal("an update with nothing for the target succeeded")
	}
	if s, _ := service(t, x, id); s.Set.Count != 2 || s.Set.Error == "" {
		t.Fatalf("after the failed update: %+v", s.Set)
	}
}

// controller is a core that writes down what it is asked.
type controller struct {
	mu     sync.Mutex
	asked  []string
	refuse string // a request answered with an error
}

func (c *controller) saw(what string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Contains(c.asked, what)
}

func withCore(t *testing.T, x *bridge) *controller {
	t.Helper()
	dir, err := os.MkdirTemp("", "b")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, socketName)
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	c := &controller{}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		what := r.Method + " " + r.URL.Path
		c.mu.Lock()
		c.asked = append(c.asked, what)
		refuse := c.refuse == what
		c.mu.Unlock()
		if refuse {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"message":"refused"}`)
			return
		}
		_, _ = io.WriteString(w, "{}")
	})}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	x.client, x.mon = mihomo.NewUnix(sock), monitor.New(filepath.Join(dir, "stats.json"))
	return c
}

// With the core running, an import gives it a new config, and an update only
// has it read the one file again, so no connection closes.
func TestRuleSetAndTheRunningCore(t *testing.T) {
	x := testBridge(t)
	ctx := context.Background()
	c := withCore(t, x)
	id, _, p := importChinaDNS(t, x, routes.Default)
	if !c.saw("PUT /configs") {
		t.Fatalf("the import did not reload the core: %v", c.asked)
	}
	c.mu.Lock()
	c.asked = nil
	c.mu.Unlock()
	p.set(http.StatusOK, "payload:\n  - DOMAIN-SUFFIX,doh.pub\n")
	if err := x.updateRuleSet(ctx, id); err != nil {
		t.Fatal(err)
	}
	if !c.saw("PUT /providers/rules/rs-"+id) || c.saw("PUT /configs") {
		t.Fatalf("an update asked the core for %v", c.asked)
	}
	// A list that turns from rules into bare addresses changes what the
	// config says of it, which only a new config carries.
	c.mu.Lock()
	c.asked = nil
	c.mu.Unlock()
	p.set(http.StatusOK, "payload:\n  - 10.0.0.0/8\n")
	if err := x.updateRuleSet(ctx, id); err != nil {
		t.Fatal(err)
	}
	if !c.saw("PUT /configs") {
		t.Fatalf("a changed behavior asked the core for %v", c.asked)
	}
}

// Two picks with the same filename are still different sources.
func TestRuleSetSameFilenameUsesChosenFile(t *testing.T) {
	x := testBridge(t)
	ctx := context.Background()
	paths := []string{filepath.Join(t.TempDir(), "list.yaml"), filepath.Join(t.TempDir(), "list.yaml")}
	for i, path := range paths {
		if err := os.WriteFile(path, []byte("payload:\n  - DOMAIN,"+[]string{"first.example", "second.example"}[i]+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := x.checkRuleSet(ctx, callArgs{Path: path}); err != nil {
			t.Fatal(err)
		}
	}
	id, err := x.importRuleSet(ctx, callArgs{Path: paths[0], Route: routes.Default})
	if err != nil {
		t.Fatal(err)
	}
	if got := payloadOf(t, x.ruleSets.Path(id)); !slices.Equal(got, []string{"DOMAIN,first.example"}) {
		t.Fatalf("imported the other pick: %v", got)
	}
}

// The home card and the group page must count the same active rules.
func TestRuleSetCountsOnHome(t *testing.T) {
	x := testBridge(t)
	before := x.view().RuleCount
	id, _, _ := importChinaDNS(t, x, routes.Default)
	if got := x.view().RuleCount; got != before+4 {
		t.Fatalf("home counts %d, want %d", got, before+4)
	}
	if err := x.routeOp(context.Background(), callArgs{Op: "toggleService", ID: id}); err != nil {
		t.Fatal(err)
	}
	if got := x.view().RuleCount; got != before {
		t.Fatalf("disabled rules counted on home: %d, want %d", got, before)
	}
}

// A core refusing a changed provider must leave the last good on-disk copy.
func TestRuleSetRefusedUpdateKeepsRules(t *testing.T) {
	for _, body := range []string{"payload:\n  - DOMAIN,changed.example\n", "payload:\n  - +.changed.example\n"} {
		t.Run(body, func(t *testing.T) {
			x := testBridge(t)
			c := withCore(t, x)
			id, _, p := importChinaDNS(t, x, routes.Default)
			before, _ := x.ruleSets.Get(id)
			c.mu.Lock()
			c.refuse = "PUT /providers/rules/rs-" + id
			if strings.Contains(body, "+.") {
				c.refuse = "PUT /configs"
			}
			c.mu.Unlock()
			p.set(http.StatusOK, body)
			if err := x.updateRuleSet(context.Background(), id); err == nil {
				t.Fatal("the refused update succeeded")
			}
			if got := payloadOf(t, x.ruleSets.Path(id)); !slices.Equal(got, chinaDNSRules) {
				t.Fatalf("refused rules replaced the last good copy: %v", got)
			}
			after, _ := x.ruleSets.Get(id)
			if after.Behavior != before.Behavior || after.Count != 4 || after.Error == "" || !after.UpdatedAt.Equal(before.UpdatedAt) {
				t.Fatalf("after refused update: %+v, before %+v", after, before)
			}
			cfg, _ := rulesOfConfig(t, x)
			if got := cfg["rule-providers"].(map[string]any)["rs-"+id].(map[string]any)["behavior"]; got != "classical" {
				t.Fatalf("refused behavior remained in config: %v", got)
			}
		})
	}
}

// A config the core refuses takes the set back out: no service, no file.
func TestRuleSetTheCoreRefuses(t *testing.T) {
	x := testBridge(t)
	c := withCore(t, x)
	c.refuse = "PUT /configs"
	_, base := serve(t, chinaDNS)
	id, err := x.importRuleSet(context.Background(), callArgs{Input: base + "/ChinaDNS.yaml", Route: routes.Default})
	if err == nil {
		t.Fatalf("imported as %s", id)
	}
	if n := len(x.set.Get().Routing.Services); n != 0 {
		t.Fatalf("%d services stayed", n)
	}
	if entries, _ := os.ReadDir(filepath.Join(x.dir, "rulesets")); slices.ContainsFunc(entries, func(e os.DirEntry) bool { return strings.HasSuffix(e.Name(), ".yaml") }) {
		t.Fatalf("a file stayed: %v", entries)
	}
}
