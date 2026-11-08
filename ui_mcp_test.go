package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/settings"
)

// TestMCPSection: 设置 > MCP shows the policy, ask by default, how to
// connect an agent, and what agents did; choosing a policy sets it.
func TestMCPSection(t *testing.T) {
	defer i18n.Set("zh")
	agentLog.list = []agentChange{
		{At: time.Date(2026, 11, 8, 10, 4, 5, 0, time.Local), Client: "claude-code 2.1", What: words{"添加规则", "Add a rule"}, Outcome: "applied"},
		{At: time.Date(2026, 11, 8, 10, 3, 0, 0, time.Local), Client: "claude-code 2.1", What: words{"更改接管", "Change the takeover"}, Outcome: "denied"},
	}
	defer func() { agentLog.list = nil }()
	u := fixtureUI()
	u.snap.Settings = settings.Defaults.Clone()
	tt := ui.NewTesterWith(u.settingsView, 760, 720, ui.TesterOptions{})
	tt.Advance(time.Second)
	if err := tt.Click("MCP"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, w := range []string{"agent 变更时", "询问", "始终允许", "不允许", "Claude Code", "其他客户端", "最近的 agent 变更", "已应用", "被拒绝", "claude-code 2.1 · 添加规则"} {
		if !tt.HasText(w) {
			t.Errorf("lacks %q: %q", w, tt.Texts())
		}
	}
	shot(t, tt, "settings-mcp")
	if err := tt.Click("始终允许"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := u.shownSettings().Policy(); got != "allow" {
		t.Errorf("policy after choosing 始终允许: %s", got)
	}

	i18n.Set("en")
	tt.Frame()
	if !tt.HasText("claude-code 2.1 · Add a rule") {
		t.Errorf("the list should follow the language: %q", tt.Texts())
	}
}
