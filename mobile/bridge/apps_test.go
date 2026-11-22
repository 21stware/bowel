package bridge

// Rules for an app on the phone. From Android 10 the VPN service says which
// app owns a connection (mobile/android/owner), and a rule names the app's
// package; before, and on iOS, there is no such rule.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

// withAppRules is SetAppRules for one test.
func withAppRules(t *testing.T, on bool) {
	t.Helper()
	was := routes.AppRules
	t.Cleanup(func() { routes.AppRules = was })
	SetAppRules(on)
}

func offers(x *bridge, kind string) bool {
	return slices.ContainsFunc(x.routing().Kinds, func(k kindData) bool { return k.ID == kind })
}

const chatgpt = "com.openai.chatgpt"

// On Android 10 and later the kind is offered, and a rule for a package
// reaches the core ahead of the domain rules, with the core asked to look
// apps up for it.
func TestAppRuleOnThePhone(t *testing.T) {
	withAppRules(t, true)
	x := testBridge(t)
	ctx := context.Background()
	if !offers(x, routes.KindApp) {
		t.Fatal("the app kind is not offered")
	}
	ai := group(t, x, "AI 服务")
	r, err := x.route(ai.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := x.routeOp(ctx, callArgs{Op: "addRule", Kind: routes.KindApp, Text: " " + chatgpt + " ", Route: ai.ID}); err != nil {
		t.Fatal(err)
	}
	cfg, rules := rulesOfConfig(t, x)
	if cfg["find-process-mode"] != "strict" {
		t.Fatalf("find-process-mode = %v, want strict", cfg["find-process-mode"])
	}
	app := slices.Index(rules, "PROCESS-NAME,"+chatgpt+","+routes.Group(r))
	domain := slices.IndexFunc(rules, func(s string) bool { return strings.HasPrefix(s, "DOMAIN") })
	if app < 0 || domain >= 0 && app > domain {
		t.Fatalf("the app's rule at %d, the first domain rule at %d: %q", app, domain, rules)
	}
	found := false
	for _, s := range group(t, x, "AI 服务").Services {
		for _, l := range s.Rules {
			if l.Value == chatgpt {
				found = s.Mine && l.Kind == routes.KindApp && l.KindName == routes.KindName(routes.KindApp)
			}
		}
	}
	if !found {
		t.Fatal("the rule is not in the group's own rules as an app's")
	}
}

// Before Android 10 no rule for an app is offered or taken, and the core
// looks nothing up. One brought over from a desktop stays, and goes.
func TestNoAppRulesBeforeAndroid10(t *testing.T) {
	withAppRules(t, false)
	x := testBridge(t)
	ctx := context.Background()
	if offers(x, routes.KindApp) {
		t.Fatal("the app kind is offered")
	}
	if err := x.routeOp(ctx, callArgs{Op: "addRule", Kind: routes.KindApp, Text: chatgpt, Route: "ai"}); err == nil {
		t.Fatal("a rule for an app was added")
	}
	mine := routes.UserRule{Kind: routes.KindSuffix, Value: "chat.example.com", Route: "ai"}
	if err := x.routeOp(ctx, callArgs{Op: "addRule", Kind: mine.Kind, Text: mine.Value, Route: mine.Route}); err != nil {
		t.Fatal(err)
	}
	if err := x.routeOp(ctx, callArgs{Op: "editRule", Old: mine, Kind: routes.KindApp, Text: chatgpt, Route: "ai"}); err == nil {
		t.Fatal("a rule was edited into an app's")
	}
	if cfg := config(t, x); cfg["find-process-mode"] != "off" {
		t.Fatalf("find-process-mode = %v, want off", cfg["find-process-mode"])
	}

	desktop := routes.UserRule{Kind: routes.KindApp, Value: "Telegram.exe", Route: "ai"}
	if err := x.change(ctx, func(s *settings.Settings) { s.Rules = append(s.Rules, desktop) }); err != nil {
		t.Fatal(err)
	}
	if err := x.routeOp(ctx, callArgs{Op: "removeRule", Kind: desktop.Kind, Text: desktop.Value, Route: desktop.Route}); err != nil {
		t.Fatal(err)
	}
	if got := x.set.Get().Rules; slices.ContainsFunc(got, desktop.Same) || !slices.ContainsFunc(got, mine.Same) {
		t.Fatalf("rules after removing the desktop's = %+v", got)
	}
}

// A rule for an app closes that app's connections that go elsewhere, so
// that it shows at once, and leaves other apps' alone.
func TestAppRuleMovesTheAppsConnections(t *testing.T) {
	withAppRules(t, true)
	x := testBridge(t)
	ctx := context.Background()
	c := withCore(t, x)
	ai, err := x.route("ai")
	if err != nil {
		t.Fatal(err)
	}
	other, err := x.route(routes.Default)
	if err != nil {
		t.Fatal(err)
	}
	conn := func(id, process, group string) map[string]any {
		return map[string]any{"id": id, "chains": []string{"东京 01", group}, "metadata": map[string]any{"network": "tcp", "process": process, "host": "x.example.com"}}
	}
	snap, _ := json.Marshal(map[string]any{"connections": []any{
		conn("chat-elsewhere", chatgpt, routes.Group(other)),
		conn("chat-ai", chatgpt, routes.Group(ai)),
		conn("mail", "com.google.android.gm", routes.Group(other)),
	}})
	c.mu.Lock()
	c.conns = string(snap)
	c.mu.Unlock()

	if err := x.routeOp(ctx, callArgs{Op: "addRule", Kind: routes.KindApp, Text: chatgpt, Route: ai.ID}); err != nil {
		t.Fatal(err)
	}
	if !c.saw("DELETE /connections/chat-elsewhere") || c.saw("DELETE /connections/chat-ai") || c.saw("DELETE /connections/mail") {
		t.Fatalf("adding closed %v", c.asked)
	}
	if err := x.routeOp(ctx, callArgs{Op: "removeRule", Kind: routes.KindApp, Text: chatgpt, Route: ai.ID}); err != nil {
		t.Fatal(err)
	}
	if !c.saw("DELETE /connections/chat-ai") || c.saw("DELETE /connections/mail") {
		t.Fatalf("removing closed %v", c.asked)
	}
}

// Until the first rule for an app the core looks no app up, so a connection
// opened before has none and no rule could move it. That rule closes them,
// those that went direct too (the core closes those by a node itself as it
// reloads), and not bowel's own. With the core looking already, the next
// rule for an app closes only its app's.
func TestFirstAppRuleClosesWhatWasNeverLookedUp(t *testing.T) {
	withAppRules(t, true)
	x := testBridge(t)
	ctx := context.Background()
	c := withCore(t, x)
	conn := func(id, kind, process string, chains ...string) map[string]any {
		return map[string]any{"id": id, "chains": chains, "metadata": map[string]any{"network": "tcp", "type": kind, "process": process, "host": "x.example.com"}}
	}
	snap, _ := json.Marshal(map[string]any{"connections": []any{
		conn("direct", "Tun", "", "DIRECT"),
		conn("by-node", "Tun", "", "东京 01", "默认"),
		conn("probe", "Inner", "", "东京 01"),
		conn("update", "Mixed", "", "东京 01"),
	}})
	c.mu.Lock()
	c.conns = string(snap)
	c.mu.Unlock()

	mine := routes.UserRule{Kind: routes.KindSuffix, Value: "chat.example.com", Route: "ai"}
	if err := x.routeOp(ctx, callArgs{Op: "addRule", Kind: mine.Kind, Text: mine.Value, Route: mine.Route}); err != nil {
		t.Fatal(err)
	}
	if c.saw("DELETE /connections/direct") || c.saw("DELETE /connections/by-node") {
		t.Fatalf("a rule for a domain closed %v", c.asked)
	}
	if err := x.routeOp(ctx, callArgs{Op: "addRule", Kind: routes.KindApp, Text: chatgpt, Route: "ai"}); err != nil {
		t.Fatal(err)
	}
	if !c.saw("DELETE /connections/direct") || !c.saw("DELETE /connections/by-node") {
		t.Fatalf("the first rule for an app left open what it never looked up: %v", c.asked)
	}
	if c.saw("DELETE /connections/probe") || c.saw("DELETE /connections/update") {
		t.Fatalf("the first rule for an app closed bowel's own: %v", c.asked)
	}

	c.mu.Lock()
	c.asked = nil
	c.mu.Unlock()
	if err := x.routeOp(ctx, callArgs{Op: "addRule", Kind: routes.KindApp, Text: "com.android.chrome", Route: "ai"}); err != nil {
		t.Fatal(err)
	}
	if c.saw("DELETE /connections/direct") || c.saw("DELETE /connections/by-node") {
		t.Fatalf("a second rule for an app closed connections not its app's: %v", c.asked)
	}
}

func TestFindsApps(t *testing.T) {
	dir := t.TempDir()
	for name, tc := range map[string]struct {
		cfg  string
		want bool
	}{
		"no file":             {"", false},
		"off":                 {"find-process-mode: off\nrules:\n  - PROCESS-NAME,a.b,x\n", false},
		"always":              {"find-process-mode: always\nrules:\n  - MATCH,x\n", true},
		"strict, no app rule": {"find-process-mode: strict\nrules:\n  - DOMAIN-SUFFIX,a.b,x\n  - MATCH,x\n", false},
		"strict, app rule":    {"find-process-mode: strict\nrules:\n  - PROCESS-NAME,a.b,x\n  - MATCH,x\n", true},
		"strict, uid":         {"find-process-mode: strict\nrules:\n  - UID,10234,x\n", true},
		"strict, in a logic":  {"find-process-mode: strict\nrules:\n  - AND,((NETWORK,udp),(PROCESS-NAME-REGEX,a.*)),x\n", true},
		"strict, in sub-rule": {"find-process-mode: strict\nrules:\n  - SUB-RULE,(NETWORK,tcp),s\nsub-rules:\n  s:\n    - PROCESS-PATH,/a,x\n", true},
	} {
		path := filepath.Join(dir, strings.ReplaceAll(name, " ", "_")+".yaml")
		if tc.cfg != "" {
			if err := os.WriteFile(path, []byte(tc.cfg), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if got := findsApps(path); got != tc.want {
			t.Errorf("%s: findsApps = %v, want %v", name, got, tc.want)
		}
	}
}

// Activity says a connection went by the rule for its app.
func TestActivityNamesTheAppsRule(t *testing.T) {
	got := ruleText("ProcessName", chatgpt)
	if !strings.Contains(got, chatgpt) || strings.HasPrefix(got, "ProcessName") {
		t.Fatalf("ruleText = %q", got)
	}
	all := routes.Build(routes.Plan{}, []routes.UserRule{{Kind: routes.KindApp, Value: chatgpt, Route: "ai"}})
	if sv, ok := routes.FindMatched(all, "ProcessName", chatgpt); !ok || sv.Group != "ai" {
		t.Fatalf("FindMatched = %+v %v, want the rule's home in AI 服务", sv, ok)
	}
}
