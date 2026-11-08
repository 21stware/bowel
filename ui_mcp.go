package main

// 设置 > MCP: what agents may do, how to connect one, and what they did.

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/settings"
)

// shellQuote quotes s for a shell when it needs it.
func shellQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t'\"\\$`!*?[](){};&|<>#~") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// mcpClaudeCommand is the command that registers bowel with Claude Code.
func mcpClaudeCommand(exe string) string {
	return "claude mcp add --scope user bowel -- " + shellQuote(exe) + " mcp"
}

// mcpJSON is bowel's entry for clients configured with a JSON file
// (Claude Desktop, Cursor, …).
func mcpJSON(exe string) string {
	type server struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	b, _ := json.MarshalIndent(map[string]any{"mcpServers": map[string]server{"bowel": {exe, []string{"mcp"}}}}, "", "  ")
	return string(b)
}

func (u *UI) mcpSection(c *ui.Context, p *pal, s settings.Settings) {
	sectionNote(c, p, i18n.T("让 Claude Code 等 agent 通过 MCP 读取并修改 bowel 的配置", "Let agents such as Claude Code read and change bowel's configuration over MCP"))
	optRow(c, p, i18n.T("agent 变更时", "When an agent makes a change"), i18n.T("读取总是允许；修改时先问你、直接允许，或一律拒绝", "Reading is always allowed; a change asks you first, goes through, or is refused"), func() {
		names := []string{i18n.T("询问", "Ask"), i18n.T("始终允许", "Always allow"), i18n.T("不允许", "Don't allow")}
		sel := max(0, slices.Index(settings.MCPPolicies, s.Policy()))
		if actSeg(c.Key("mcpPolicy"), p, &sel, names).Changed() && settings.MCPPolicies[sel] != s.Policy() {
			v := settings.MCPPolicies[sel]
			u.setOpt(func(s *settings.Settings) { s.MCPPolicy = v })
		}
	})

	optHeading(c, p, i18n.T("接入 agent", "Connect an agent"))
	exe := mcpCommand()
	u.mcpSnippet(c, p, "mcpClaude", "Claude Code", i18n.T("在终端运行这条命令", "Run this command in a terminal"), mcpClaudeCommand(exe))
	u.mcpSnippet(c, p, "mcpJSON", i18n.T("其他客户端", "Other clients"), i18n.T("加到客户端的 MCP 配置里（stdio）；bowel 没开时会自己打开", "Add this to the client's MCP config (stdio); bowel opens itself when it isn't running"), mcpJSON(exe))
	if searching() {
		return
	}
	state := i18n.T("运行中", "Running")
	if e := mcpErr.Load(); e == nil {
		state = "—"
	} else if *e != "" {
		state = i18n.T("未运行", "Stopped") + " · " + firstLine(*e)
	}
	ui.Column(c).Padding(11, 0).BorderWidth(0, 0, 1, 0).BorderColor(p.hair2).Children(func() {
		sock := "—"
		if u.app != nil {
			sock = mcpSocket(u.app.dataDir)
		}
		kv(c, p, [][2]string{{i18n.T("服务", "Server"), state}, {i18n.T("套接字", "Socket"), sock}})
	})

	optHeading(c, p, i18n.T("最近的 agent 变更", "Recent agent changes"))
	list := agentChanges()
	if len(list) == 0 {
		ui.Text(c, i18n.T("还没有", "None yet")).FontSize(12).TextColor(p.mute).Padding(8, 0)
		return
	}
	ui.Column(c).Children(func() {
		for i, ch := range list {
			ui.Row(c.Key("chg"+strconv.Itoa(i))).Gap(10).Padding(7, 0).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.hair2).Children(func() {
				ui.Text(c, ch.At.Format("15:04:05")).FontSize(11.5).TextColor(p.mute).Font("SF Mono, Menlo").Shrink(0)
				word, col := i18n.T("已应用", "Applied"), p.good
				switch ch.Outcome {
				case "denied":
					word, col = i18n.T("被拒绝", "Denied"), p.warn
				case "failed":
					word, col = i18n.T("失败", "Failed"), p.signal
				}
				ui.Text(c, word).FontSize(11.5).FontWeight(600).TextColor(col).Shrink(0)
				line := ch.Client + " · " + ch.What.String()
				if ch.Error != "" {
					line += " · " + firstLine(ch.Error)
				}
				ui.Text(c, line).FontSize(12).TextColor(p.ink).NoWrap().Ellipsis("…").Shrink(1).MinWidth(0).Tooltip(line)
			})
		}
	})
}

// mcpSnippet is a block of text to copy, with its button.
func (u *UI) mcpSnippet(c *ui.Context, p *pal, key, title, desc, text string) {
	optWide(c, p, title, desc, func() {
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(8).AlignItems(ui.Start).Children(func() {
			ui.Text(c, text).FontSize(11.5).Font("SF Mono, Menlo").TextColor(p.ink2).Selectable().
				Padding(7, 10).Radius(p.r(7)).Background(p.card).Border(1, p.hair).Grow(1).Basis(0).MinWidth(0)
			btn(c.Key(key), p, i18n.T("复制", "Copy")).OnClick(func() {
				c.WriteClipboard(text)
				u.settingsToast(i18n.T("已复制", "Copied ") + title)
			})
		})
	})
}
