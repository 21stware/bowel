package cli

import (
	"strings"
	"testing"
)

// aiGroup is list_route_groups's answer for the ai group, as a bowel gives it.
func aiGroup(order ...string) map[string]any {
	if order == nil {
		order = []string{}
	}
	return map[string]any{"groups": []any{map[string]any{
		"id": "ai", "name": "AI", "position": 1, "builtin": true, "movable": true, "usesNodes": true, "node": "hk1", "nodeLabel": "🇭🇰 香港 01",
		"strategy": map[string]any{"mode": "auto", "regions": []string{"JP", "US"}, "providers": []string{"p1"}, "providerNames": map[string]string{"p1": "Sub"},
			"exceptProviders": true, "nodes": []string{}, "by": "download", "downloadEvery": 12, "order": order, "labels": map[string]string{"jp": "JP-Tokyo-02"}},
		"sites":    []any{map[string]any{"name": "OpenAI", "url": "https://api.openai.com/v1/models"}, map[string]any{"name": "Claude", "url": "https://api.anthropic.com/v1/models"}},
		"services": []string{"OpenAI", "Claude"},
		"rules":    []any{map[string]any{"kind": "suffix", "value": "openai.com", "route": "ai", "matches": "openai.com and its subdomains"}},
	}}, "routing": map[string]any{"source": "bowel", "name": "bowel"}}
}

// TestGroupCommands: each route group command calls its tool with what was
// typed, and says what came of it; what is typed wrong stops at the command
// line.
func TestGroupCommands(t *testing.T) {
	socket, calls := fake(t, map[string]func(map[string]any) (any, error){
		"list_route_groups":  func(map[string]any) (any, error) { return aiGroup("jp"), nil },
		"create_route_group": func(map[string]any) (any, error) { return map[string]any{"created": "g1", "groups": []any{}}, nil },
		"move_route_group": func(map[string]any) (any, error) {
			return map[string]any{"changed": true, "groups": []any{map[string]any{"id": "g1", "name": "Netflix"}, map[string]any{"id": "default", "name": "Other"}}}, nil
		},
		"set_route_strategy": func(map[string]any) (any, error) {
			return map[string]any{"changed": true, "groups": aiGroup()["groups"]}, nil
		},
		"set_route_sites": func(map[string]any) (any, error) {
			return map[string]any{"changed": false, "groups": aiGroup()["groups"]}, nil
		},
		"delete_route_group": func(map[string]any) (any, error) { return map[string]any{"deleted": "g1", "movedTo": "media"}, nil },
		"list_routes":        func(map[string]any) (any, error) { return map[string]any{"routes": []any{}}, nil },
		"set_routing_source": func(map[string]any) (any, error) {
			return map[string]any{"changed": true, "routing": map[string]any{"source": "p1", "name": "Sub"}}, nil
		},
	})
	env := Env{Socket: socket}
	last := func() string { return (*calls)[len(*calls)-1] }
	for _, c := range []struct {
		args       []string
		call, says string
	}{
		{[]string{"route", "new", "Netflix", "--before", "media"}, `create_route_group {"before":"media","name":"Netflix"}`, "Created route group Netflix (id g1). Send traffic to it with `bowel rules add <target> g1`."},
		{[]string{"route", "move", "Netflix", "--at", "1"}, `move_route_group {"position":1,"route":"Netflix"}`, "✓ Order: Netflix → Other"},
		{[]string{"route", "strategy", "ai", "fixed", "--node", "jp"}, `set_route_strategy {"mode":"fixed","node":"jp","route":"ai"}`, "✓ AI: Fastest · JP, US · not Sub · by download speed, every 12 h"},
		{[]string{"route", "strategy", "ai", "--by", "download", "--every", "24"}, `set_route_strategy {"by":"download","downloadEvery":24,"route":"ai"}`, "AI: Fastest"},
		{[]string{"route", "order", "ai", "jp", "🇺🇸 US"}, `set_route_strategy {"mode":"order","order":["jp","🇺🇸 US"],"route":"ai"}`, "AI: "},
		{[]string{"route", "scope", "ai", "--regions", "JP, US", "--nodes", `["a, b","c"]`, "--only"}, `set_route_strategy {"exceptProviders":false,"nodes":["a, b","c"],"regions":["JP","US"],"route":"ai"}`, "AI: "},
		{[]string{"route", "scope", "ai", "--providers", "all"}, `set_route_strategy {"providers":[],"route":"ai"}`, "AI: "},
		{[]string{"route", "direct", "ai", "on"}, `set_route_strategy {"direct":true,"route":"ai"}`, "AI: "},
		{[]string{"route", "sites", "ai", "claude.ai"}, `set_route_sites {"route":"ai","sites":["claude.ai"]}`, "nothing changed"},
		{[]string{"route", "sites", "ai", "--reset"}, `set_route_sites {"reset":true,"route":"ai"}`, "nothing changed"},
		{[]string{"route", "remove", "Netflix", "--to", "media"}, `delete_route_group {"route":"Netflix","to":"media"}`, "Deleted Netflix; its services and rules went to media"},
		{[]string{"routing", "Sub"}, `set_routing_source {"source":"Sub"}`, "Now using Sub's routing"},
		{[]string{"routing"}, `list_routes {}`, "Using bowel's own route groups and rules"},
	} {
		code, out, errOut := run(t, env, c.args...)
		if code != ExitOK || last() != c.call || !strings.Contains(out, c.says) {
			t.Errorf("%v: %d %q %q\ncalled %s\nwant   %s", c.args, code, out, errOut, last(), c.call)
		}
	}

	// Read: the list, one group whole, its sites; JSON as bowel gave it.
	_, out, _ := run(t, env, "route", "show")
	for _, want := range []string{"#  ID  Group  Strategy", "1  ai  AI     Fastest · JP, US · not Sub · by download speed, every 12 h", "OpenAI", "Tried from the top"} {
		if !strings.Contains(out, want) {
			t.Errorf("route show lacks %q:\n%s", want, out)
		}
	}
	_, out, _ = run(t, env, "route", "show", "ai")
	for _, want := range []string{"AI  ai · #1 · built in", "Backup order  JP-Tokyo-02", "Now           🇭🇰 香港 01", "Test sites    OpenAI  https://api.openai.com/v1/models  (main)", "Claude  https://api.anthropic.com", "Services      OpenAI · Claude", "Your rules    suffix openai.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("route show ai lacks %q:\n%s", want, out)
		}
	}
	if _, out, _ = run(t, env, "route", "show", "ai", "--json"); !strings.Contains(out, `"downloadEvery": 12`) {
		t.Errorf("route show --json:\n%s", out)
	}
	_, out, _ = run(t, env, "route", "sites", "ai")
	if lines := strings.Split(out, "\n"); len(lines) < 4 || !strings.HasPrefix(lines[1], "OpenAI  https://api.openai.com/v1/models") || !strings.HasSuffix(lines[1], "main") ||
		!strings.Contains(out, "default test sites") {
		t.Errorf("route sites:\n%s", out)
	}

	// Typed wrong: nothing is called.
	n := len(*calls)
	for _, args := range [][]string{
		{"route", "move", "ai"},
		{"route", "move", "ai", "--at", "0"},
		{"route", "move", "ai", "--before", "dev", "--after", "media"},
		{"route", "new", "X", "--at", "2", "--before", "dev"},
		{"route", "strategy", "ai"},
		{"route", "strategy", "ai", "fastest"},
		{"route", "strategy", "ai", "--every", "5"},
		{"route", "order", "ai"},
		{"route", "scope", "ai"},
		{"route", "scope", "ai", "--except", "--only"},
		{"route", "scope", "ai", "--nodes", "[oops"},
		{"route", "direct", "ai", "yes"},
		{"route", "sites", "ai", "x.com", "--reset"},
	} {
		if code, _, errOut := run(t, env, args...); code != ExitUsage || errOut == "" {
			t.Errorf("%v: %d %q, want a usage error", args, code, errOut)
		}
	}
	if len(*calls) != n {
		t.Errorf("a wrong command line called %v", (*calls)[n:])
	}
}

func TestListArg(t *testing.T) {
	for in, want := range map[string]string{"": "", "all": "", "ALL": "", "JP,US": "JP|US", " JP , ,US ": "JP|US", `["a, b","c"]`: "a, b|c"} {
		got, err := listArg(in)
		if err != nil || got == nil || strings.Join(got, "|") != want {
			t.Errorf("listArg(%q) = %q %v, want %q", in, got, err, want)
		}
	}
	if _, err := listArg("[1,2]"); err == nil {
		t.Error("a list of numbers was taken")
	}
}
