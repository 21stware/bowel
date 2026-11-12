package main

// 设置 > 开发者网络: what stays off the proxy, and what was found of this
// machine's networks.

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/devnet"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/settings"
)

func (u *UI) devnetSection(c *ui.Context, p *pal, s settings.Settings) {
	sectionNote(c, p, i18n.T("让本地开发、局域网、容器、Tailscale 和公司 VPN 不受代理影响", "Keeps local development, the LAN, containers, Tailscale and company VPNs clear of the proxy"))
	u.optSwitch(c, p, "devnet", i18n.T("自动放行开发者网络", "Keep developer networks direct"), i18n.T("全局模式下也生效；你自己写的规则优先", "Works in global mode too; your own rules come first"), !s.DevOff,
		func(s *settings.Settings, on bool) { s.DevOff = !on })

	d := u.snap.Dev
	for _, w := range d.Warnings {
		ui.Text(c, w).FontSize(12).TextColor(p.accentLine).Padding(9, 0, 0, 0)
	}

	optHeading(c, p, i18n.T("这台 Mac 上的网络", "Networks on this Mac"))
	devFact(c, p, "Tailscale", i18n.T("每 20 秒看一次，变了自动更新", "Checked every 20 seconds and updated as it changes"), tailscaleWord(d.Env.Tailscale))
	if len(d.Env.VPNs) > 0 {
		var names []string
		for _, t := range d.Env.VPNs {
			names = append(names, fmt.Sprintf(i18n.T("%s（%d 个网段）", "%s (%d networks)"), t.Iface, len(t.Prefixes)))
		}
		devFact(c, p, i18n.T("其他 VPN", "Other VPNs"), i18n.T("这些网段从 VPN 自己的网卡出去", "These networks leave by the VPN's own interface"), strings.Join(names, ", "))
	}
	if len(d.Env.Resolvers) > 0 {
		var names []string
		for _, r := range d.Env.Resolvers {
			names = append(names, r.Domain)
		}
		devFact(c, p, i18n.T("分流 DNS", "Split DNS"), i18n.T("这些域名继续问系统指定的 DNS", "These domains keep asking the DNS the system names for them"), strings.Join(names, ", "))
	}

	optHeading(c, p, i18n.T("场景", "Scenarios"))
	for _, sc := range devnet.Scenarios() {
		id := sc.ID
		u.optSwitch(c, p, "dev-"+id, sc.Name(), sc.Desc(), !slices.Contains(s.DevOffScenarios, id), func(s *settings.Settings, on bool) {
			s.DevOffScenarios = slices.DeleteFunc(s.DevOffScenarios, func(x string) bool { return x == id })
			if !on {
				s.DevOffScenarios = append(s.DevOffScenarios, id)
			}
		}, s.DevOff)
	}

	optHeading(c, p, i18n.T("更多", "More"))
	optWide(c, p, i18n.T("我的内网", "My internal networks"), i18n.T("公司的域名后缀或网段，如 corp.example、10.8.0.0/16，同时用于系统代理、DNS、规则和 TUN", "Your company's domain suffixes or networks, as corp.example or 10.8.0.0/16; used by the system proxy, DNS, rules and TUN alike"), func() {
		u.optList(c, p, "devextra", i18n.T("我的内网", "My internal networks"), s.DevExtra, func(v []string) {
			u.setOpt(func(s *settings.Settings) { s.DevExtra = v })
		})
	})
	if d.Exports != "" {
		u.mcpSnippet(c, p, "devexports", i18n.T("终端代理", "Terminal proxy"), i18n.T("命令行工具不看系统代理。贴到终端里，内网地址会自动跳过代理", "Command-line tools ignore the system proxy. Paste this into a terminal; internal addresses skip the proxy"), d.Exports)
		u.mcpSnippet(c, p, "devunsets", i18n.T("关闭终端代理", "Turn off the terminal proxy"), i18n.T("贴到同一个终端窗口里，恢复不走代理；关掉窗口也一样", "Paste it into the same terminal window to stop using the proxy. Closing the window does the same"), d.Unsets)
	}
}

// devFact is a line of what was found, its value on the right.
func devFact(c *ui.Context, p *pal, title, desc, value string) {
	optRow(c, p, title, desc, func() {
		ui.Text(c, value).FontSize(12).TextColor(p.ink2).MaxWidth(320).Ellipsis("…").NoWrap().Tooltip(value)
	})
}

func tailscaleWord(ts devnet.Tailscale) string {
	switch {
	case !ts.Installed:
		return i18n.T("未发现", "Not found")
	case ts.Iface != "":
		parts := []string{i18n.T("已连接", "Connected"), ts.Iface}
		if ts.Suffix != "" {
			parts = append(parts, ts.Suffix)
		}
		if n := len(ts.Peers); n > 0 {
			parts = append(parts, fmt.Sprintf(i18n.T("%d 台设备", "%d devices"), n))
		}
		return strings.Join(parts, " · ")
	case ts.State == "NeedsLogin":
		return i18n.T("需要登录", "Needs login")
	}
	return i18n.T("未连接", "Not connected")
}

// meshWord is what 直连 says of Tailscale: whether its machines are kept
// direct, and how many there are.
func meshWord(s settings.Settings, ts devnet.Tailscale) string {
	switch {
	case s.DevOff || slices.Contains(s.DevOffScenarios, devnet.Mesh):
		return i18n.T("没有自动直连", "Not kept direct")
	case ts.Iface == "":
		return tailscaleWord(ts)
	}
	if n := len(ts.Peers); n > 0 {
		return fmt.Sprintf(i18n.T("已连接 · %d 台设备", "Connected · %d devices"), n)
	}
	return i18n.T("已连接", "Connected")
}

// openDevnet shows 设置 > 开发者网络.
func (u *UI) openDevnet() {
	u.set.section, u.set.query, u.set.lit = "devnet", "", nil
	u.openSettings()
}

// meshRow is 直连's line about Tailscale, where one would look for a group
// of its own: its machines go direct by 开发者网络, not by a rule here.
// A machine without Tailscale has none.
func (u *UI) meshRow(c *ui.Context, p *pal) {
	ts := u.snap.Dev.Env.Tailscale
	if !ts.Installed {
		return
	}
	tip := i18n.T("在设置的「开发者网络」里查看和调整", "See and change it under Developer networks in Settings")
	b := ui.ButtonBase(c.Key("mesh")).Row().Gap(10).Padding(10, 14).AlignItems(ui.Center).
		Radius(p.r(10)).Background(p.card).Border(1, p.hair).Margin(0, 0, 14, 0)
	b.Label("Tailscale").Tooltip(tip).OnClick(u.openDevnet).Children(func() {
		ui.Text(c, "Tailscale").FontSize(12.5).FontWeight(600).TextColor(p.ink).NoWrap().Shrink(0)
		ui.Text(c, i18n.T("设备、MagicDNS 和子网自动直连，全局模式下也一样", "Machines, MagicDNS and subnets go direct, in global mode too")).
			FontSize(12).TextColor(p.mute).SingleLine().Shrink(1).MinWidth(0)
		ui.Spacer(c)
		ui.Text(c, meshWord(u.shownSettings(), ts)).FontSize(12.5).TextColor(p.ink2).NoWrap().Shrink(0)
		ui.Text(c, "›").FontSize(13).TextColor(p.mute).Shrink(0)
	})
}
