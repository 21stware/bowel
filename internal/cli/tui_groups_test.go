package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/oboo/moho/internal/i18n"
)

// groupScreen is a tui on a fake bowel with an AI group above the default
// route and direct, in English, 100 by 30.
func groupScreen(t *testing.T) (*tui, *[]string) {
	t.Helper()
	i18n.Set("en")
	t.Cleanup(func() { i18n.Set("zh") })
	status := map[string]any{}
	for k, v := range statusJSON {
		status[k] = v
	}
	status["routes"] = []any{
		map[string]any{"id": "ai", "name": "AI", "node": "hk1", "nodeLabel": "🇭🇰 香港 01", "auto": true},
		map[string]any{"id": "direct", "name": "Direct", "direct": true},
		map[string]any{"id": "default", "name": "Other", "node": "hk1", "nodeLabel": "🇭🇰 香港 01", "auto": true},
	}
	order := []string{"jp"}
	ok := func(map[string]any) (any, error) { return map[string]any{"changed": true}, nil }
	socket, calls := fake(t, map[string]func(map[string]any) (any, error){
		"get_status": func(map[string]any) (any, error) { return status, nil },
		"list_nodes": func(map[string]any) (any, error) {
			return map[string]any{"current": "hk1", "providers": map[string]string{"p1": "Sub"}, "nodes": []any{
				map[string]any{"name": "hk1", "label": "🇭🇰 香港 01", "provider": "p1", "type": "ss", "delayMs": 45},
				map[string]any{"name": "jp", "label": "JP-Tokyo-02", "provider": "p1", "type": "vmess", "delayMs": 800},
			}}, nil
		},
		"list_route_groups": func(a map[string]any) (any, error) {
			if a["route"] == "direct" {
				return map[string]any{"groups": []any{map[string]any{"id": "direct", "name": "Direct", "direct": true}}}, nil
			}
			return aiGroup(order...), nil
		},
		"set_route_strategy": func(a map[string]any) (any, error) {
			if l, ok := a["order"].([]any); ok {
				order = nil
				for _, n := range l {
					order = append(order, n.(string))
				}
			}
			return map[string]any{"changed": true}, nil
		},
		"move_route_group": func(a map[string]any) (any, error) { // as bowel would
			rs := status["routes"].([]any)
			for i, r := range rs {
				if r.(map[string]any)["id"] == a["route"] {
					rs = slices.Insert(slices.Delete(slices.Clone(rs), i, i+1), int(a["position"].(float64))-1, r)
					break
				}
			}
			status["routes"] = rs
			return map[string]any{"changed": true}, nil
		},
		"create_route_group": ok, "set_route_sites": ok, "rename_route_group": ok, "delete_route_group": ok,
		"list_connections": func(map[string]any) (any, error) { return map[string]any{"connections": []any{}}, nil },
		"tail_logs":        func(map[string]any) (any, error) { return map[string]any{"logs": []any{}}, nil },
		"list_rules":       func(map[string]any) (any, error) { return map[string]any{"rules": []any{}}, nil },
		"list_providers":   func(map[string]any) (any, error) { return map[string]any{"providers": []any{}}, nil },
		"get_settings":     func(map[string]any) (any, error) { return map[string]any{"settings": map[string]any{}}, nil },
	})
	c := &Ctx{env: Env{Socket: socket, Getenv: func(string) string { return "" }}, flags: map[string]string{}}
	t.Cleanup(func() {
		if c.cl != nil {
			c.cl.close()
		}
	})
	ui := &tui{c: c, w: 100, h: 30}
	ui.poll(true)
	return ui, calls
}

// TestTUIGroups: on a route's row the keys set its strategy, move it, set
// its test sites, rename and delete it, and make a new group; in the node
// list, o puts a node in the backup order.
func TestTUIGroups(t *testing.T) {
	ui, calls := groupScreen(t)
	called := func(want string) bool {
		t.Helper()
		for _, c := range *calls {
			if c == want {
				return true
			}
		}
		t.Errorf("not called: %s\nin %v", want, *calls)
		return false
	}
	keys(ui, "s") // on a switch, not a route
	if ui.choose != nil || !strings.Contains(text(ui), "Choose a route group first") {
		t.Errorf("s on a switch:\n%s", text(ui))
	}
	keys(ui, "jjjjj")                                        // AI
	if s := text(ui); !strings.Contains(s, "[s] Strategy") { // and the rest, as wide as the bar is
		t.Errorf("the route's keys are not shown:\n%s", s)
	}
	if ui.w = 200; !strings.Contains(text(ui), "[J K] Move down/up   [e] Test sites") {
		t.Errorf("a wide bar lacks the route's keys:\n%s", text(ui))
	}
	ui.w = 100

	// The strategy: from a list, the one in use marked.
	keys(ui, "s")
	if ui.choose == nil || !strings.Contains(text(ui), "AI's strategy") || !strings.Contains(text(ui), "●  Fastest") || !strings.Contains(text(ui), "Direct  no node") {
		t.Fatalf("s did not open the strategies:\n%s", text(ui))
	}
	keys(ui, "jj\r")
	called(`set_route_strategy {"mode":"fixed","route":"ai"}`)
	keys(ui, "sjjj\r")
	called(`set_route_strategy {"direct":true,"route":"ai"}`)

	// Moved down, the row going with it; the default route stays last.
	keys(ui, "J")
	called(`move_route_group {"position":2,"route":"ai"}`)
	if ui.sel[tabHome] != 6 {
		t.Errorf("the selection stayed behind: %d", ui.sel[tabHome])
	}
	keys(ui, "K")
	called(`move_route_group {"position":1,"route":"ai"}`)
	keys(ui, "jjJ")
	if !strings.Contains(text(ui), "always last") {
		t.Errorf("the default route moved:\n%s", text(ui))
	}
	keys(ui, "kk")

	// Test sites, typed over the ones there are.
	keys(ui, "e")
	if ui.input == nil || string(ui.input.buf) != "https://api.openai.com/v1/models https://api.anthropic.com/v1/models" {
		t.Fatalf("e: %+v", ui.input)
	}
	keys(ui, " gemini.google.com\r")
	called(`set_route_sites {"route":"ai","sites":["https://api.openai.com/v1/models","https://api.anthropic.com/v1/models","gemini.google.com"]}`)
	keys(ui, "e")
	ui.input.buf = []rune("reset")
	keys(ui, "\r")
	called(`set_route_sites {"reset":true,"route":"ai"}`)

	// Named, made and deleted.
	keys(ui, "R\x7f\x7fBots\r")
	called(`rename_route_group {"name":"Bots","route":"ai"}`)
	keys(ui, "nWork\r")
	called(`create_route_group {"name":"Work"}`)
	keys(ui, "d")
	if ui.confirm == nil || !strings.Contains(text(ui), "Delete route group AI?") {
		t.Fatalf("d did not ask:\n%s", text(ui))
	}
	keys(ui, "y")
	called(`delete_route_group {"route":"ai"}`)

	// The backup order, in the node list: its places shown, o adds.
	keys(ui, "\r")
	if ui.pick == nil || !strings.Contains(text(ui), "Backup") || !strings.Contains(text(ui), "[o] Backup order") {
		t.Fatalf("the picker:\n%s", text(ui))
	}
	keys(ui, "o")
	called(`set_route_strategy {"order":["jp","hk1"],"route":"ai"}`)
	if p := ui.pick; p == nil || len(p.order) != 2 {
		t.Errorf("the order shown is not the new one: %+v", ui.pick)
	}
	keys(ui, "jo")
	called(`set_route_strategy {"order":["hk1"],"route":"ai"}`)
	keys(ui, "\x1b")

	// Direct never uses a node: no strategy to choose.
	keys(ui, "js")
	if ui.choose != nil || !strings.Contains(text(ui), "uses no node") {
		t.Errorf("a strategy for direct:\n%s", text(ui))
	}
	// Every key is in the help.
	keys(ui, "?")
	for _, want := range []string{"[n] New group", "[R] Rename", "[d] Delete group", "[e] Test sites", "[o] Backup order"} {
		if !strings.Contains(text(ui), want) {
			t.Errorf("the help lacks %q:\n%s", want, text(ui))
		}
	}
}
