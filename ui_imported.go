package main

// What the interface shows of a provider's own routing: the row's offer, and
// the 路由组 page in its place. Which member a select group is on is the
// user's to change; everything else about the groups is the config's.

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/i18n"
)

// importedGroup is the group of an imported config behind route r.
func (u *UI) importedGroup(r RouteView) (GroupView, bool) {
	if u.snap.Imported == nil {
		return GroupView{}, false
	}
	for _, g := range u.snap.Imported.Groups {
		if g.Name == r.Group {
			return g, true
		}
	}
	return GroupView{}, false
}

// groupKindWord is what a kind of group does, in words.
func groupKindWord(kind string) string {
	switch kind {
	case "select":
		return i18n.T("手动选择", "Manual")
	case "url-test":
		return i18n.T("自动最快", "Fastest")
	case "fallback":
		return i18n.T("顺序备用", "Fallback")
	case "load-balance":
		return i18n.T("负载均衡", "Load balance")
	case "relay":
		return i18n.T("链式代理", "Relay")
	}
	return kind
}

// provRouting is the row's line about the routing its config brings: an
// offer, what is in use, or why it can't be. It is an offer and never done
// by itself: a subscription's update must not change anyone's routing.
func (u *UI) provRouting(c *ui.Context, p *pal, pv ProviderView) {
	r := pv.Routing
	if r.Problem != "" {
		ui.Text(c, i18n.T("带路由组，但不能使用：", "Has route groups that can't be used: ")+r.Problem).FontSize(11.5).TextColor(p.mute).MaxLines(2)
		if name := r.NeedsFile; name != "" && pv.Enabled {
			// The one problem a file settles: it is added under the name
			// the config has for it.
			ui.Row(c).Children(func() {
				link(c.Key("needs-file"), p, i18n.T("选择文件…", "Choose file…"), func() {
					if u.app != nil && !u.pending("add") {
						u.do(func() error { return u.app.addProviderFileAs(bg(), name) })
					}
				})
			})
		}
		return
	}
	if r.Groups == 0 || !pv.Enabled {
		return
	}
	id := pv.ID
	if r.Remote > 0 && !r.InUse {
		// Otherwise a config whose nodes are all its subscriptions' looks
		// like one that brought none.
		ui.Text(c, fmt.Sprintf(i18n.T("这份配置自带 %d 个订阅，里面的节点在使用之后才由内核下载", "This config brings %d subscriptions of its own; the core downloads their nodes once it is in use"), r.Remote)).FontSize(11.5).TextColor(p.mute).MaxLines(2)
	}
	pr := &u.prov
	if r.InUse {
		if pr.offer == id {
			pr.offer = ""
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf(i18n.T("正在使用它的 %d 个路由组", "Using its %d route groups"), r.Groups)).FontSize(11.5).FontWeight(600).TextColor(p.ink).NoWrap()
			link(c.Key("routing"), p, i18n.T("改回 bowel 路由", "Back to bowel's routing"), func() { u.useRouting("") })
		})
		return
	}
	asking := pr.confirm == "use:"+id
	rules, sets := len(u.shownSettings().Rules), len(u.snap.RuleSets)
	use := func(label string) {
		if asking {
			label = fmt.Sprintf(i18n.T("确认使用（暂停 %d 条规则、%d 个规则集）", "Confirm (pauses %d rules and %d rule sets)"), rules, sets)
		}
		btn(c.Key("routing"), p, label).OnClick(func() {
			if !asking && rules+sets > 0 {
				pr.confirm, pr.confAt = "use:"+id, c.Now()
				return
			}
			pr.confirm, pr.offer = "", ""
			u.useRouting(id)
		})
	}
	if pr.offer == id {
		// Just added: the question is put once, in words that say what
		// either answer does. Left unanswered, nothing changes.
		ui.Column(c.Key("offer")).Gap(6).Margin(4, 0, 0, 0).Children(func() {
			ui.Text(c, fmt.Sprintf(i18n.T("这份配置自带 %d 个路由组、%d 条规则。要按它的路由分流，还是只用它的节点？", "This config has %d route groups and %d rules of its own. Route by them, or only use its nodes?"), r.Groups, r.Rules)).FontSize(12).TextColor(p.ink).MaxLines(2)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				use(i18n.T("按它的路由分流", "Route by this config"))
				btn(c.Key("nodes-only"), p, i18n.T("只用节点", "Nodes only")).OnClick(func() { pr.offer = "" })
			})
		})
		return
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, fmt.Sprintf(i18n.T("这份配置带 %d 个路由组、%d 条规则", "This config has %d route groups and %d rules"), r.Groups, r.Rules)).FontSize(11.5).TextColor(p.mute).NoWrap()
		use(i18n.T("使用它们", "Use them"))
	})
}

func (u *UI) useRouting(id string) {
	if u.app != nil {
		u.do(func() error { return u.app.setImportedRouting(id) })
	}
}

// link is a word to press inside a line of text.
func link(c *ui.Context, p *pal, label string, click func()) {
	b := ui.ButtonBase(c).OnClick(click)
	b.Children(func() {
		t := ui.Text(c, label).FontSize(11.5).FontWeight(600).TextColor(p.ink).NoWrap()
		if b.Hovered() {
			t.TextColor(p.ink2)
		}
	})
}

// importedPage stands in for 路由组: where each group goes and what it can
// point at, and what is out of reach while the config's routing is used.
func (u *UI) importedPage(c *ui.Context, p *pal) {
	im := u.snap.Imported
	st := &u.routes
	at := 0
	for i, g := range im.Groups {
		if g.Name == st.group {
			at = i
		}
	}
	ui.Column(c).Fill().Children(func() {
		pageHead(c, p, i18n.T("路由组 · 来自这份配置", "Routes · From this config"), im.Name,
			fmt.Sprintf(i18n.T("%d 个路由组 · %d 条规则，按配置里的顺序生效", "%d route groups · %d rules, in the config's order"), len(im.Groups), im.Rules), true, func() {
				btn(c.Key("back"), p, i18n.T("改回 bowel 路由", "Back to bowel's routing")).OnClick(func() { u.useRouting("") })
			})
		note := i18n.T("新建路由组、改规则和测试一个地址现在不可用；实际命中了哪条规则，可以在「活动」里看。", "New route groups, rule changes and testing an address aren't available now; Activity shows which rule each connection matched.")
		if im.PausedRules+im.PausedSets > 0 {
			note = fmt.Sprintf(i18n.T("你的 %d 条规则和 %d 个规则集已暂停，改回 bowel 路由后恢复。", "Your %d rules and %d rule sets are paused until you go back to bowel's routing. "), im.PausedRules, im.PausedSets) + note
		}
		if len(im.Unused) > 0 {
			ui.Text(c, note).FontSize(12).TextColor(p.mute).Padding(0, 26, 4, 26)
			// Someone who wrote them expects them to work.
			note = fmt.Sprintf(i18n.T("这份配置里的 %s 不会生效：DNS、TUN 和本机端口用的是 bowel 自己的设置。", "Not used from this config: %s. DNS, TUN and local ports follow bowel's own settings."), strings.Join(im.Unused, i18n.T("、", ", ")))
		}
		if len(im.DNSFill) == 0 {
			st.fillDNS = false
			ui.Text(c, note).FontSize(12).TextColor(p.mute).Padding(0, 26, 12, 26)
		} else {
			ui.Text(c, note).FontSize(12).TextColor(p.mute).Padding(0, 26, 4, 26)
			u.importedDNSFill(c, p, im)
		}
		ui.Row(c).Grow(1).Basis(0).MinHeight(0).Padding(0, 26, 18, 26).Gap(18).AlignItems(ui.Stretch).Children(func() {
			ui.Scroll(c.Key("list")).Width(244).Shrink(0).Gap(2).Children(func() {
				for i, g := range im.Groups {
					u.importedItem(c, p, g, i == at)
				}
			})
			if len(im.Groups) > 0 {
				g := im.Groups[at]
				ui.Scroll(c.Key("main:"+g.Name)).Grow(1).Basis(0).MinWidth(0).Padding(0, 12, 0, 0).Children(func() { u.importedGroupDetail(c, p, g) })
			}
		})
	})
}

// importedDNSFill offers to copy the config's DNS servers, policy, fake-ip
// filter and hosts into the settings: once, on request, and after saying
// what it replaces. They are settings from then on. The config's DNS is
// never followed as it changes: how names resolve goes with how traffic is
// taken over, and an update must not change it unseen.
func (u *UI) importedDNSFill(c *ui.Context, p *pal, im *ImportedView) {
	st := &u.routes
	sep := i18n.T("、", ", ")
	ui.Column(c.Key("dns-fill")).Gap(4).Padding(0, 26, 12, 26).Children(func() {
		if !st.fillDNS {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, i18n.T("其中的 DNS 服务器、域名策略和 hosts 可以一次性填进 bowel 的设置。", "Its DNS servers, nameserver policy and hosts can be copied into bowel's settings, once.")).FontSize(12).TextColor(p.mute)
				link(c.Key("fill"), p, i18n.T("把其中的 DNS 填入设置…", "Copy its DNS into settings…"), func() { st.fillDNS = true })
			})
			return
		}
		ui.Text(c, fmt.Sprintf(i18n.T("将用这份配置的 %s 替换「设置 › DNS」里对应的几项，其余不动；DNS 模式、监听和劫持仍按 bowel 的。之后订阅更新不会再改它们。", "The config's %s will replace the same entries in Settings › DNS; nothing else changes, and the DNS mode, listener and hijack stay bowel's. Later updates of the config won't change them."), strings.Join(im.DNSFill, sep))).FontSize(12).TextColor(p.ink)
		if n := len(im.DNSSkipped); n > 0 {
			shown := im.DNSSkipped
			more := ""
			if n > 4 {
				shown, more = shown[:4], i18n.T(" 等", " and more")
			}
			ui.Text(c, fmt.Sprintf(i18n.T("有 %d 项填不进去，会跳过：%s", "%d entries can't be held and are skipped: %s"), n, strings.Join(shown, sep)+more)).FontSize(12).TextColor(p.mute).MaxLines(2)
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			btn(c.Key("fill-ok"), p, i18n.T("确认填入", "Copy them")).OnClick(func() {
				st.fillDNS = false
				if u.app == nil {
					return
				}
				go func() {
					took, err := u.app.fillImportedDNS()
					if err != nil {
						u.toast(err.Error())
						return
					}
					u.toast(fmt.Sprintf(i18n.T("已填入 %s，可以在「设置 › DNS」里修改", "Copied %s; change them in Settings › DNS"), strings.Join(took, i18n.T("、", ", "))))
				}()
			})
			link(c.Key("fill-no"), p, i18n.T("取消", "Cancel"), func() { st.fillDNS = false })
		})
	})
}

func (u *UI) importedItem(c *ui.Context, p *pal, g GroupView, on bool) {
	b := ui.ButtonBase(c.Key(g.Name)).Column().Gap(2).Padding(8, 10).Radius(p.r(8)).AlignItems(ui.Stretch)
	switch {
	case on:
		b.Background(p.hair2)
	case b.Hovered():
		b.Background(p.hover)
	}
	name := g.Name
	b.OnClick(func() { u.routes.group = name }).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			col := p.hair
			if g.Route != "" {
				col = p.routeColor(g.Route)
			}
			swatch(c, col, 9).Shrink(0)
			ui.Text(c, g.Name).FontSize(13).FontWeight(600).TextColor(p.ink).SingleLine().Grow(1).Basis(0).MinWidth(0)
		})
		ui.Text(c, groupKindWord(g.Kind)+" · "+u.groupNowWord(g)).FontSize(11.5).TextColor(p.mute).SingleLine().Margin(0, 0, 0, 17)
	})
}

// groupNowWord is what a group is on, by the name people read.
func (u *UI) groupNowWord(g GroupView) string {
	if n, ok := u.node(g.Exit); ok {
		return n.Display
	}
	if g.Exit != "" {
		return display(g.Exit)
	}
	if g.Now != "" {
		return g.Now
	}
	return "—"
}

func (u *UI) importedGroupDetail(c *ui.Context, p *pal, g GroupView) {
	ui.Column(c).Gap(3).Padding(2, 0, 12, 0).Children(func() {
		ui.Text(c, g.Name).FontSize(22).FontWeight(700).TextColor(p.ink).MaxLines(2)
		what := groupKindWord(g.Kind)
		if g.Rules > 0 {
			what += fmt.Sprintf(i18n.T(" · %d 条规则指向这里", " · %d rules point here"), g.Rules)
		} else {
			what += i18n.T(" · 没有规则直接指向这里", " · no rule points here directly")
		}
		if !g.Selectable {
			what += i18n.T(" · 由内核自己选择", " · the core chooses by itself")
		}
		ui.Text(c, what).FontSize(12).TextColor(p.mute)
	})
	u.groupMembers(c, p, g)
}

// groupMembers lists what g can point at, in the config's order.
func (u *UI) groupMembers(c *ui.Context, p *pal, g GroupView) {
	group := g.Name
	for _, m := range g.Members {
		member := m.Name
		chosen := g.Now == member
		b := ui.ButtonBase(c.Key(member)).Row().Gap(8).Padding(6, 10).Radius(p.r(6)).AlignItems(ui.Center)
		if g.Selectable && b.Hovered() {
			b.Background(p.hover)
		}
		if g.Selectable && !chosen {
			b.OnClick(func() {
				if u.app != nil {
					u.do(func() error { return u.app.selectMember(group, member) })
				}
			})
		}
		b.Children(func() {
			mark := " "
			if chosen {
				mark = "✓"
			}
			ui.Text(c, mark).FontSize(12).FontWeight(700).TextColor(p.ink).Width(12).Shrink(0)
			n, isNode := u.node(m.Node)
			title := func() {
				t := display(member)
				if isNode {
					t = n.Display
				}
				shimmer(ui.Text(c, t).FontSize(13).FontWeight(ifw(chosen)).TextColor(p.ink).SingleLine().Grow(1).Basis(0).MinWidth(0), chosen && u.pending("group:"+group), p)
			}
			if isNode {
				flagged(c, p, n, 13, title).Grow(1).Basis(0).MinWidth(0)
			} else {
				ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(title)
			}
			if m.Group {
				ui.Text(c, i18n.T("路由组", "Route group")).FontSize(11).TextColor(p.mute).NoWrap().Shrink(0)
			}
			ui.Text(c, fmtMs(m.Delay, m.Tested)).FontSize(11.5).FontFeatures("tnum").TextColor(delayColor(p, m.Delay, m.Tested)).NoWrap().Shrink(0)
		})
	}
}

func ifw(strong bool) int {
	if strong {
		return 600
	}
	return 400
}

// groupPicker takes the place of the node picker for a route that is an
// imported group.
func (u *UI) groupPicker(c *ui.Context, p *pal, r RouteView) {
	ui.Scroll(c).Grow(1).Basis(0).MinHeight(0).Padding(4, 16, 18, 16).Children(func() {
		g, ok := u.importedGroup(r)
		if !ok {
			return
		}
		if !g.Selectable {
			ui.Text(c, fmt.Sprintf(i18n.T("这个组是「%s」，由内核自己选择", "This group is %s: the core chooses by itself"), groupKindWord(g.Kind))).FontSize(12).TextColor(p.mute).Padding(2, 10, 8, 10)
		}
		u.groupMembers(c, p, g)
	})
}
