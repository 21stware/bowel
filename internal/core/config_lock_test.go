package core

import (
	"slices"
	"strings"
	"testing"

	"github.com/oboo/moho/internal/routes"
	"gopkg.in/yaml.v3"
)

// lockedAI is the template with the AI group's exit lock on.
func lockedAI(t *testing.T, change ...func(*routes.Plan)) []routes.Route {
	t.Helper()
	var p routes.Plan
	if !p.SetLocked("ai", true) {
		t.Fatal("the AI group can't be locked")
	}
	for _, fn := range change {
		fn(&p)
	}
	return routes.Build(p, nil)
}

// lockConfig is the config for in, read back.
func lockConfig(t *testing.T, in Input) (mode string, rules []string, groups map[string][]string) {
	t.Helper()
	in.MixedPort, in.ProbePort, in.TestURL = 7899, 7900, "x"
	if len(in.Providers) == 0 {
		in.Providers = []ProviderRef{{ID: "a", Path: "a.yaml"}}
	}
	out, err := BuildConfig(in)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Mode   string   `yaml:"mode"`
		Rules  []string `yaml:"rules"`
		Groups []struct {
			Name    string   `yaml:"name"`
			Proxies []string `yaml:"proxies"`
		} `yaml:"proxy-groups"`
	}
	if err := yaml.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}
	groups = map[string][]string{}
	for _, g := range cfg.Groups {
		groups[g.Name] = g.Proxies
	}
	return cfg.Mode, cfg.Rules, groups
}

func has(rules []string, rule string) bool { return slices.Contains(rules, rule) }

// TestLockRuleMode: in rule mode with takeover on, a locked group is
// routed as any other.
func TestLockRuleMode(t *testing.T) {
	mode, rules, _ := lockConfig(t, Input{Routes: lockedAI(t), Mode: "rule"})
	if mode != "rule" {
		t.Errorf("mode %q", mode)
	}
	for _, want := range []string{"DOMAIN-SUFFIX,openai.com,AI 服务", "DOMAIN-SUFFIX,netflix.com,流媒体", "MATCH,默认"} {
		if !has(rules, want) {
			t.Errorf("rules lack %q: %v", want, rules)
		}
	}
}

// TestLockGuard: with takeover off, the core sends everything direct but
// the locked group's traffic, which it refuses.
func TestLockGuard(t *testing.T) {
	for _, mode := range []string{"rule", "global", "direct"} {
		got, rules, _ := lockConfig(t, Input{Routes: lockedAI(t), Mode: mode, Guard: true})
		if got != "rule" {
			t.Errorf("%s: core mode %q", mode, got)
		}
		for _, want := range []string{"DOMAIN-SUFFIX,openai.com,REJECT", "DOMAIN-SUFFIX,netflix.com,DIRECT", "MATCH,DIRECT"} {
			if !has(rules, want) {
				t.Errorf("%s: rules lack %q: %v", mode, want, rules)
			}
		}
		for _, r := range rules {
			if strings.HasSuffix(r, ",默认") || strings.HasSuffix(r, ",流媒体") || strings.HasSuffix(r, ",AI 服务") {
				t.Errorf("%s: %q still goes by a node while takeover is off", mode, r)
			}
		}
	}
}

// TestLockGuardNeedsALock: Guard alone changes nothing.
func TestLockGuardNeedsALock(t *testing.T) {
	all := routes.Build(routes.Plan{}, nil)
	_, plain, _ := lockConfig(t, Input{Routes: all, Mode: "rule"})
	_, guarded, _ := lockConfig(t, Input{Routes: all, Mode: "rule", Guard: true})
	if !slices.Equal(plain, guarded) {
		t.Errorf("Guard without a lock changed the rules:\n%v\n%v", plain, guarded)
	}
	mode, _, _ := lockConfig(t, Input{Routes: all, Mode: "global"})
	if mode != "global" {
		t.Errorf("global mode without a lock became %q", mode)
	}
}

// TestLockOtherModes: outside rule mode the locked group keeps its rules
// and its nodes; the rest goes as the mode says.
func TestLockOtherModes(t *testing.T) {
	for mode, other := range map[string]string{"global": "GLOBAL", "direct": "DIRECT"} {
		got, rules, _ := lockConfig(t, Input{Routes: lockedAI(t), Mode: mode})
		if got != "rule" {
			t.Errorf("%s: core mode %q", mode, got)
		}
		for _, want := range []string{"DOMAIN-SUFFIX,openai.com,AI 服务", "DOMAIN-SUFFIX,netflix.com," + other, "MATCH," + other} {
			if !has(rules, want) {
				t.Errorf("%s: rules lack %q: %v", mode, want, rules)
			}
		}
	}
}

// TestLockKeepsTheListsOrder: while guarding, the rules of a group above a
// locked one still come first: a rule of the user's in 流媒体, dragged
// above AI 服务, sends its site there, direct; below it, the lock has it.
func TestLockKeepsTheListsOrder(t *testing.T) {
	users := []routes.UserRule{{Kind: routes.KindDomain, Value: "status.openai.com", Route: "media"}}
	for above, want := range map[bool]bool{true: true, false: false} {
		var p routes.Plan
		p.SetLocked("ai", true)
		if above {
			p.MoveGroup("media", "ai")
		}
		_, rules, _ := lockConfig(t, Input{Routes: routes.Build(p, users), UserRules: users, Mode: "rule", Guard: true})
		at := slices.Index(rules, "DOMAIN,status.openai.com,DIRECT")
		lock := slices.Index(rules, "DOMAIN-SUFFIX,openai.com,REJECT")
		if at < 0 || lock < 0 || (at < lock) != want {
			t.Errorf("流媒体 above: %v: the user's rule at %d, the lock's at %d: %v", above, at, lock, rules)
		}
	}
}

// TestLockNoNodes: a locked group with no node at all refuses its traffic
// rather than let it out DIRECT, as an unlocked one does.
func TestLockNoNodes(t *testing.T) {
	in := Input{Routes: lockedAI(t), Mode: "rule", MixedPort: 7899, ProbePort: 7900, TestURL: "x"}
	out, err := BuildConfig(in)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Groups []struct {
			Name    string   `yaml:"name"`
			Proxies []string `yaml:"proxies"`
		} `yaml:"proxy-groups"`
	}
	if err := yaml.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}
	for _, g := range cfg.Groups {
		switch g.Name {
		case "AI 服务·自动":
			if !slices.Equal(g.Proxies, []string{"REJECT"}) {
				t.Errorf("locked auto group holds %v", g.Proxies)
			}
		case "流媒体·自动":
			if !slices.Equal(g.Proxies, []string{"DIRECT"}) {
				t.Errorf("unlocked auto group holds %v", g.Proxies)
			}
		}
	}
}

// TestLockDirectByChoice: a locked group set to go direct (by a file, or
// another client) refuses its traffic.
func TestLockDirectByChoice(t *testing.T) {
	all := lockedAI(t, func(p *routes.Plan) { p.ChangePolicy("ai", func(q *routes.Policy) { q.Direct = true }) })
	_, rules, _ := lockConfig(t, Input{Routes: all, Mode: "rule"})
	if !has(rules, "DOMAIN-SUFFIX,openai.com,REJECT") {
		t.Errorf("a locked group going direct should be refused: %v", rules)
	}
}

// TestLockable: the default group and the two without nodes take no lock;
// locking one that goes direct puts it back on its nodes.
func TestLockable(t *testing.T) {
	var p routes.Plan
	for _, id := range []string{routes.Default, "direct", routes.Reject} {
		if p.SetLocked(id, true) {
			t.Errorf("%s took the lock", id)
		}
	}
	p.ChangePolicy("dev", func(q *routes.Policy) { q.Direct = true })
	if !p.SetLocked("dev", true) || p.Policy("dev").Direct || !p.Locked("dev") {
		t.Errorf("locking dev: direct %v, locked %v", p.Policy("dev").Direct, p.Locked("dev"))
	}
	for _, r := range routes.Build(p, nil) {
		if r.Locked != (r.ID == "dev") {
			t.Errorf("%s locked %v", r.ID, r.Locked)
		}
	}
	if !p.AnyLocked() || !routes.AnyLocked(routes.Build(p, nil)) {
		t.Error("AnyLocked missed dev's lock")
	}
	// A lock a file gives a group that can't have one counts for nothing.
	q := routes.Plan{Groups: []routes.GroupConf{{ID: routes.Default, Locked: true}, {ID: "direct", Locked: true}}}
	if q.AnyLocked() || routes.AnyLocked(routes.Build(q, nil)) {
		t.Error("a lock on the default or direct group counted")
	}
	if p.SetLocked("dev", false); p.AnyLocked() {
		t.Error("unlocking left a lock")
	}
}

// Through the real core, takeover off: a locked group's site is refused,
// another group's goes DIRECT. The direct one is asked first, so that the
// core's resolver is up by the time the refused one is asked.
func TestLockGuardRuns(t *testing.T) {
	users := []routes.UserRule{
		{Kind: routes.KindSuffix, Value: "free.bowel-test.dev", Route: "media"},
		{Kind: routes.KindSuffix, Value: "held.bowel-test.dev", Route: "ai"},
	}
	var p routes.Plan
	p.SetLocked("ai", true)
	all := routes.Build(p, users)
	guard := func(in *Input) { in.Guard = true }
	goesByIn(t, all, users, map[string]string{"free.bowel-test.dev": "DIRECT"}, guard)
	goesByIn(t, all, users, map[string]string{"held.bowel-test.dev": "REJECT"}, guard)
}

// TestLockImported: a provider's own routing has no groups of bowel's, so
// a lock leaves its mode alone.
func TestLockImported(t *testing.T) {
	in := Input{Routes: lockedAI(t), Mode: "global", Guard: true, Imported: &Imported{}}
	if in.locks() || in.guarding() {
		t.Error("imported routing counted the lock")
	}
}

// TestLockNoNodeInProviders: a subscription that is there but holds no node
// (it failed to come down, or the node a group is kept to is gone) leaves
// the core's group empty, and the core fills an empty group with
// COMPATIBLE, which goes direct. A locked group's automatic group and its
// backup steps hold REJECT for then; an unlocked one's don't.
func TestLockNoNodeInProviders(t *testing.T) {
	_, _, groups := lockConfig(t, Input{Routes: lockedAI(t), Mode: "rule"})
	if !has(groups["AI 服务·自动"], "REJECT") {
		t.Errorf("locked auto group holds %v", groups["AI 服务·自动"])
	}
	if has(groups["流媒体·自动"], "REJECT") {
		t.Errorf("unlocked auto group holds %v", groups["流媒体·自动"])
	}
	scoped := Input{Routes: lockedAI(t), Mode: "rule", Scopes: map[string][]string{"ai": {"n" + NodeSep + "a"}}}
	if _, _, groups := lockConfig(t, scoped); !has(groups["AI 服务·自动"], "REJECT") {
		t.Errorf("locked auto group kept to a node holds %v", groups["AI 服务·自动"])
	}
	ordered := Input{Routes: lockedAI(t), Mode: "rule", Orders: map[string][]string{"ai": {"n" + NodeSep + "a"}}}
	_, _, groups = lockConfig(t, ordered)
	steps := 0
	for name, members := range groups {
		if strings.HasPrefix(name, "AI 服务·备用·") {
			steps++
			if !has(members, "REJECT") {
				t.Errorf("locked group's %s holds %v", name, members)
			}
		}
	}
	if steps == 0 {
		t.Errorf("no backup step in %v", groups)
	}
}

// Through the real core: a locked group whose subscription file isn't there
// refuses its traffic, whether it takes any node, is kept to one, or goes
// by a backup order.
func TestLockNoNodeInProvidersRuns(t *testing.T) {
	users := []routes.UserRule{{Kind: routes.KindSuffix, Value: "held.bowel-test.dev", Route: "ai"}}
	var p routes.Plan
	p.SetLocked("ai", true)
	all := routes.Build(p, users)
	node := "n" + NodeSep + "a"
	for name, change := range map[string]func(*Input){
		"any node": func(in *Input) {},
		"kept to":  func(in *Input) { in.Scopes = map[string][]string{"ai": {node}} },
		"by order": func(in *Input) { in.Orders = map[string][]string{"ai": {node}} },
	} {
		t.Run(name, func(t *testing.T) {
			goesByIn(t, all, users, map[string]string{"held.bowel-test.dev": "REJECT"}, func(in *Input) {
				in.Providers = []ProviderRef{{ID: "a", Path: "missing.yaml"}}
				change(in)
			})
		})
	}
}
