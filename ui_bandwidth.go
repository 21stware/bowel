package main

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/routes"
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
// how much traffic it may use. Nothing is chosen when it ends, but by a
// round of a group that chooses by download speed (bandwidthPlan.Round).
func (u *UI) bandwidthDialog(c *ui.Context, p *pal, r RouteView) {
	h := &u.home
	plan := h.bwPlan
	ui.Modal(c.Key("bandwidth-dialog"), &h.bwOpen, func() {
		ui.Column(c).Gap(12).Width(440).Children(func() {
			ui.Text(c, fmt.Sprintf(i18n.T("测%s的下载速度", "Test downloads on %s"), r.Name)).FontSize(18).FontWeight(700).TextColor(p.ink).MaxLines(1)
			if len(plan.Nodes) == 0 {
				ui.Text(c, i18n.T("列表里的节点还没有测得通延迟的。先点「全部测速」，再来测下载。", "None of the nodes listed has answered a delay test yet. Click Test all first, then test downloads.")).FontSize(12).TextColor(p.mute)
				ui.Row(c).Justify(ui.End).Children(func() {
					btn(c.Key("bw-close"), p, i18n.T("关闭", "Close")).OnClick(func() { h.bwOpen = false })
				})
				return
			}
			say := fmt.Sprintf(i18n.T("经下面 %d 个节点依次下载测速文件，每个最多 %d 秒，比较真实的下载速度。测完不会自动换节点，由你来选。", "The test file is downloaded through each of these %d nodes in turn, for up to %d seconds each, to compare real download speeds. No node is switched to afterwards: you choose."), len(plan.Nodes), int(speed.DownloadTime/time.Second))
			if plan.Round {
				say = fmt.Sprintf(i18n.T("现在就测一轮：经下面 %d 个节点依次下载测速文件，每个最多 %d 秒。测完换到下载最快的节点（正在用的慢不到五分之一就不换），下一轮从测完时算起。", "A round, now: the test file is downloaded through each of these %d nodes in turn, for up to %d seconds each. The fastest download is then used (the node in use stays if it came within a fifth of it), and the next round is counted from then."), len(plan.Nodes), int(speed.DownloadTime/time.Second))
			}
			ui.Text(c, say).FontSize(12).TextColor(p.mute)
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
			ui.Text(c, fmt.Sprintf(i18n.T("最多会用掉约 %s 流量（每个节点最多 %s）。订阅按流量计费的话请留意；有的机场不允许测速。", "This uses up to about %s of traffic (%s at most per node). Mind it if your subscription is metered; some providers don't allow speed tests."), fmtBytes(plan.MaxBytes), fmtBytes(plan.Cap))).FontSize(12).TextColor(p.ink2)
			ui.Text(c, fmt.Sprintf(i18n.T("测速文件：%s（可以在设置里换）", "Test file: %s (you can change it in Settings)"), plan.URL)).FontSize(11).TextColor(p.mute).MaxLines(2).Ellipsis("…").Tooltip(plan.URL)
			ui.Row(c).Gap(8).Margin(4, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Grow(1)
				btn(c.Key("bw-cancel"), p, i18n.T("取消", "Cancel")).OnClick(func() { h.bwOpen = false })
				btn(c.Key("bw-start"), p, i18n.T("开始测试", "Start")).OnClick(func() {
					h.bwOpen = false
					rid := r.ID
					u.do(func() error { return u.app.testBandwidth(bg(), rid, plan) })
				})
			})
		})
	})
}

// defaultDownloadEvery is the hours between scheduled download tests when
// a group is first set to choose by download speed.
const defaultDownloadEvery = 12

// downloadSay is the note under a group that chooses by download speed:
// what its last test chose, when the next runs, and what the tests cost.
func (u *UI) downloadSay(r RouteView, q routes.Policy) string {
	runs := 24 / max(1, q.DownloadEvery)
	round := int64(bandwidthNodes+1) * scheduledCap
	cost := fmt.Sprintf(i18n.T("每轮测延迟最低的 %d 个节点（加上正在用的），每个最多 %d MB，每天 %d 轮，最多约 %s/月。", "Each round tests the %d nodes with the lowest delay (and the one in use), %d MB at most each, %d rounds a day: about %s a month at most."),
		bandwidthNodes, scheduledCap>>20, runs, fmtBytes(round*int64(runs)*30))
	var state string
	d := r.Download
	switch {
	case u.pending(bandwidthKey(r.ID)):
		state = i18n.T("正在测这一轮…", "This round is being tested…")
	case d == nil || d.At.IsZero():
		state = i18n.T("马上测第一轮；测完前先用延迟最低的。", "The first round runs now; until it's done, the lowest delay is used.")
	case d.Node == "":
		state = fmt.Sprintf(i18n.T("%s测的那轮没选出节点（下载都失败了，或者选中的节点后来不通），先用延迟最低的，下一轮 %s。", "The round %s chose no node (every download failed, or the node chosen stopped answering since): the lowest delay is used, next round %s."),
			ago(d.At), d.Next.Local().Format("15:04"))
	default:
		state = fmt.Sprintf(i18n.T("%s测的：%s 下载最快（%s），一直用它；不通了先换回延迟最低的，下一轮 %s。", "Tested %s: %s downloaded fastest (%s) and is used; if it stops answering, the lowest delay is used. Next round %s."),
			ago(d.At), u.nodeDisplay(d.Node), fmtMbps(&DownloadTest{Mbps: d.Mbps}), d.Next.Local().Format("15:04"))
	}
	return state + cost + i18n.T("按流量计费或不允许测速的机场请留意。", " Mind it if your subscription is metered or forbids speed tests.")
}
