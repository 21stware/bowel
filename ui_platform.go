package main

// What the interface says differently on each system. Most of it says
// the same everywhere, and i18n turns "Mac" into the computer; these are
// the few lines about something only one system has.

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/oboo/moho/internal/devtools"
	"github.com/oboo/moho/internal/gateway"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/tun"
)

// gatewaySupported says whether this system can be made a gateway, and
// so whether the interface offers it.
func gatewaySupported() bool { return gateway.Supported() }

// tunInstallHint is TUN's status before its one authorization, which
// each system asks for in its own dialog. Windows asks at every start of
// the core instead (tunAsks), and has no such status.
func tunInstallHint() string { return tunInstallHintOn(runtime.GOOS) }

func tunInstallHintOn(goos string) string {
	if goos == "darwin" {
		return i18n.T("未使用 · 第一次开启时，macOS 会请求管理员授权", "Not in use · The first time it's turned on, macOS asks for an administrator's authorization")
	}
	return i18n.T("未使用 · 第一次开启时，系统会请求管理员授权", "Not in use · The first time it's turned on, the system asks for an administrator's authorization")
}

// tunAsks is whether every start of TUN's core asks for the authorization
// (Windows), which TUN's card then says, with why.
const tunAsks = tun.AsksEachStart

// tunAskNote is why Windows asks each time TUN's core starts, for TUN's
// card.
func tunAskNote() string {
	return i18n.T("每次启动 TUN 的内核，Windows 都会弹出用户账户控制请求授权：bowel 还没有代码签名，而 Windows Defender 会把注册计划任务、以管理员权限自动运行的未签名程序当成恶意软件拦截，所以 bowel 不再注册这样的任务",
		"Windows asks in a User Account Control prompt each time TUN's core starts: bowel isn't code-signed yet, and Windows Defender blocks as malware an unsigned program that registers a scheduled task to run elevated without asking, so bowel no longer registers one")
}

// tunAskWait is what TUN's switch and card say while Windows' prompt for
// the elevated start waits (Entry.Asking): Windows cancels one nobody
// answers.
func tunAskWait() string {
	return i18n.T("请在用户账户控制窗口中确认；约 2 分钟内没有回应，Windows 会自动取消", "Confirm in the User Account Control prompt; Windows cancels it if nobody answers within about 2 minutes")
}

// firewallTip is the gateway's warning that the system's firewall is on.
func firewallTip() string { return firewallTipOn(runtime.GOOS) }

func firewallTipOn(goos string) string {
	if goos == "darwin" {
		return i18n.T("macOS 防火墙开着：如果其他设备查不到域名，在「系统设置 → 网络 → 防火墙」里允许 mihomo 接收连接。",
			"The macOS firewall is on: if other devices can't look up names, allow mihomo incoming connections in System Settings → Network → Firewall.")
	}
	return i18n.T("防火墙（ufw 或 firewalld）开着：如果其他设备查不到域名或上不了网，放行 53 端口，并允许转发来自局域网的流量。",
		"A firewall (ufw or firewalld) is on: if other devices can't look up names or reach the network, open port 53 and allow traffic from the LAN to be forwarded.")
}

// shortcutLabel is how a shortcut held with ui.Cmd is written beside what
// it does: ⌘ on a Mac, Ctrl on the other systems, where ui.Cmd is Control.
func shortcutLabel(key string) string { return shortcutLabelOn(runtime.GOOS, key) }

func shortcutLabelOn(goos, key string) string {
	if goos == "darwin" {
		return "⌘" + key
	}
	return "Ctrl+" + key
}

// toolSubOf is the line under a tool's name. The terminal's is its
// shell, which is the one whose file bowel reads.
func toolSubOf(t devtools.Tool) string {
	if t.ID != devtools.Shell {
		return toolSub(t.ID)
	}
	switch filepath.Base(strings.TrimPrefix(t.File, "~/")) {
	case ".bashrc":
		return "bash"
	case ".zshenv":
		return "zsh"
	}
	return ""
}
