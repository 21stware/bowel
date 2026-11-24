package main

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/speed"
)

// fmtMbps is a download rate as the node list shows it.
func fmtMbps(t *DownloadTest) string {
	switch {
	case t == nil:
		return "—"
	case t.Failed:
		return i18n.T("失败", "Failed")
	case t.Mbps >= 1000:
		return fmt.Sprintf("%.1f Gbps", t.Mbps/1000)
	case t.Mbps < 10:
		return fmt.Sprintf("%.1f Mbps", t.Mbps)
	}
	return fmt.Sprintf("%.0f Mbps", t.Mbps)
}

// downloadCell is a node row's download column.
func downloadCell(c *ui.Context, p *pal, t *DownloadTest, testing bool) {
	col := p.ink2
	if t == nil || t.Failed {
		col = p.mute
	}
	cell := ui.Text(c, fmtMbps(t)).FontSize(12).FontFeatures("tnum").Width(nodeColW).Shrink(0).NoWrap().TextAlign(ui.End).TextColor(col)
	if t != nil {
		cell.Tooltip(fmt.Sprintf(i18n.T("%s测的", "Tested %s"), ago(t.At)))
	}
	if testing && t == nil {
		cell.TextColor(p.mute.Alpha(.5))
	}
}

// downloadsAlike says whether the recent download tests came out within a
// fifth of each other: when every node runs about as fast, what holds them
// back is likely the user's own line, not the nodes.
func downloadsAlike(nodes []NodeView) bool {
	var lo, hi float64
	n := 0
	for _, v := range nodes {
		t := v.Download
		if t == nil || t.Failed || t.Mbps <= 0 || time.Since(t.At) > time.Hour {
			continue
		}
		if n == 0 || t.Mbps < lo {
			lo = t.Mbps
		}
		if t.Mbps > hi {
			hi = t.Mbps
		}
		n++
	}
	return n >= 3 && hi <= lo*1.2
}

// bandwidthDialog asks before a download test of route r: which nodes, and
// how much traffic it may use. Nothing is chosen when it ends.
func (u *UI) bandwidthDialog(c *ui.Context, p *pal, r RouteView) {
	h := &u.home
	plan := h.bwPlan
	ui.Modal(c.Key("bandwidth-dialog"), &h.bwOpen, func() {
		ui.Column(c).Gap(12).Width(440).Children(func() {
			ui.Text(c, fmt.Sprintf(i18n.T("测%s的下载速度", "Test downloads on %s"), r.Name)).FontSize(18).FontWeight(700).TextColor(p.ink).MaxLines(1)
			if len(plan.Nodes) == 0 {
				ui.Text(c, i18n.T("还没有测得通延迟的节点。先点「全部测速」，再来测下载。", "No node has answered a delay test yet. Click Test all first, then test downloads.")).FontSize(12).TextColor(p.mute)
				ui.Row(c).Justify(ui.End).Children(func() {
					btn(c.Key("bw-close"), p, i18n.T("关闭", "Close")).OnClick(func() { h.bwOpen = false })
				})
				return
			}
			ui.Text(c, fmt.Sprintf(i18n.T("经下面 %d 个节点依次下载测速文件，每个最多 %d 秒，比较真实的下载速度。测完不会自动换节点，由你来选。", "The test file is downloaded through each of these %d nodes in turn, for up to %d seconds each, to compare real download speeds. No node is switched to afterwards: you choose."), len(plan.Nodes), int(speed.DownloadTime/time.Second))).FontSize(12).TextColor(p.mute)
			ui.Column(c).Children(func() {
				for _, name := range plan.Nodes {
					ui.Row(c.Key(name)).Gap(10).Padding(6, 0).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.hair2).Children(func() {
						n, _ := u.node(name)
						ui.Box(c).Width(22).Shrink(0).Children(func() { flagBadge(c, p, nodeFlag(n), 12) })
						ui.Text(c, u.nodeDisplay(name)).FontSize(13).FontWeight(600).TextColor(p.ink).MaxLines(1).Ellipsis("…").Grow(1).Shrink(1).Basis(0).MinWidth(0)
						if name == r.Node {
							ui.Text(c, i18n.T("当前", "Current")).FontSize(11).FontWeight(600).TextColor(p.ink2).NoWrap().Shrink(0)
						}
					})
				}
			})
			ui.Text(c, fmt.Sprintf(i18n.T("最多会用掉约 %s 流量（每个节点最多 %s）。订阅按流量计费的话请留意；有的机场不允许测速。", "This uses up to about %s of traffic (%s at most per node). Mind it if your subscription is metered; some providers don't allow speed tests."), fmtBytes(plan.MaxBytes), fmtBytes(speed.DownloadCap))).FontSize(12).TextColor(p.ink2)
			ui.Text(c, fmt.Sprintf(i18n.T("测速文件：%s（可以在设置里换）", "Test file: %s (you can change it in Settings)"), plan.URL)).FontSize(11).TextColor(p.mute).MaxLines(2).Ellipsis("…").Tooltip(plan.URL)
			ui.Row(c).Gap(8).Margin(4, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Grow(1)
				btn(c.Key("bw-cancel"), p, i18n.T("取消", "Cancel")).OnClick(func() { h.bwOpen = false })
				btn(c.Key("bw-start"), p, i18n.T("开始测试", "Start")).OnClick(func() {
					h.bwOpen = false
					rid := r.ID
					u.do(func() error { return u.app.testBandwidth(bg(), rid) })
				})
			})
		})
	})
}
