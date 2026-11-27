package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/devnet"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

// The list on the 路由组 page is the order rules are tried in, and all of
// it: the developer networks at its top, then the groups as listed, the
// first with a rule that matches taking the connection. What that order
// makes of a rule is said where the rule is: a rule a group above takes
// all of never matches, and one of the user's inside a developer network
// goes before it.

// routeOrder is what the order makes of the rules, for the page.
type routeOrder struct {
	dev     devnet.Overlay
	shadows map[string]routes.Shadow // routes.Shadowed
	// ahead are the user's rules inside a developer network, which go
	// before it; hidden those a group above takes all of.
	ahead, hidden []orderedRule
}

type orderedRule struct {
	group, service string
	rule           routes.Rule
}

// orderOf works out what the order makes of the rules in all.
func (u *UI) orderOf(s settings.Settings, all []routes.Route) routeOrder {
	o := routeOrder{dev: overlayOf(s, u.snap.Dev.Env)}
	var covers func(string) bool
	if !o.dev.Empty() {
		covers = o.dev.Covers
	}
	o.shadows = routes.Shadowed(all, covers, u.matchRuleSet())
	for _, r := range routes.Listed(all) {
		for _, sv := range r.Services {
			for _, l := range sv.Live() {
				if l.Builtin {
					continue
				}
				switch _, hid := o.shadows[routes.ShadowKey(sv.ID, l.Text)]; {
				case covers != nil && covers(l.Text):
					o.ahead = append(o.ahead, orderedRule{r.ID, sv.ID, l})
				case hid:
					o.hidden = append(o.hidden, orderedRule{r.ID, sv.ID, l})
				}
			}
		}
	}
	return o
}

// aheadOfDev says whether rule l of the user's goes before the developer
// networks, being inside one.
func (o routeOrder) aheadOfDev(l routes.Rule) bool {
	return !l.Builtin && !o.dev.Empty() && o.dev.Covers(l.Text)
}

// shadowWord says what takes all of a rule's traffic before it.
func (u *UI) shadowWord(all []routes.Route, sh routes.Shadow) string { return sh.Word(all) }

// ruleOrderNote is what the order makes of rule l of service s, under its
// row: nothing for most.
func (u *UI) ruleOrderNote(c *ui.Context, p *pal, all []routes.Route, s routes.Service, l routes.Rule) {
	o := u.routes.order
	switch sh, hid := o.shadows[routes.ShadowKey(s.ID, l.Text)]; {
	case l.Off || s.Off:
	case o.aheadOfDev(l):
		ui.Text(c, i18n.T("在开发者网络里，所以排在「开发者网络」前面", "Inside Developer networks, so it goes before them")).FontSize(11).TextColor(p.mute).Padding(0, 0, 5, 0)
	case hid:
		ui.Text(c, u.shadowWord(all, sh)).FontSize(11).TextColor(p.signal).Padding(0, 0, 5, 0)
	}
}

// addedWord is what the order makes of the rule just added, under the
// field it was typed in: "" unless a group above takes all of its traffic
// now, which a drag can change at any time.
func (u *UI) addedWord(all []routes.Route) string {
	st := &u.routes
	sh, ok := st.order.shadows[st.added]
	if st.added == "" || !ok {
		return ""
	}
	return u.shadowWord(all, sh) + i18n.T("。要它生效，把这个路由组拖到上面去。", ". For it to, drag this group up.")
}

// devItem is the developer networks at the top of the list: before every
// group, and with the user's rules that go before them.
func (u *UI) devItem(c *ui.Context, p *pal) {
	o := u.routes.order
	if o.dev.Empty() {
		return
	}
	b := ui.ButtonBase(c.Key("devnets:")).Column().Gap(2).Padding(8, 10).Radius(p.r(8)).AlignItems(ui.Stretch)
	if b.Hovered() {
		b.Background(p.hover)
	}
	b.Tooltip(i18n.T("在设置的「开发者网络」里查看和调整", "See and change it under Developer networks in Settings")).OnClick(u.openDevnet).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			swatch(c, p.faint, 9).Shrink(0)
			ui.Text(c, i18n.T("开发者网络", "Developer networks")).FontSize(13).FontWeight(600).TextColor(p.ink).SingleLine().Grow(1).Basis(0).MinWidth(0)
			ui.Text(c, "›").FontSize(13).TextColor(p.mute).Shrink(0)
		})
		line := i18n.T("Tailscale、VPN 和开发环境 · 总在最前，不能拖动", "Tailscale, VPNs, dev setups · always first, fixed")
		if ts := u.snap.Dev.Env.Tailscale; ts.Installed {
			line = "Tailscale " + meshWord(u.shownSettings(), ts) + " · " + i18n.T("总在最前", "always first")
		}
		ui.Text(c, line).FontSize(11.5).TextColor(p.mute).SingleLine().Margin(0, 0, 0, 17)
		if n := len(o.ahead); n > 0 {
			ui.Text(c, fmt.Sprintf(i18n.T("你的 %d 条规则在这些网络里，排在它前面", "%d of your rules are inside them and go before it"), n)).
				FontSize(11.5).TextColor(p.ink2).SingleLine().Margin(0, 0, 0, 17)
		}
	})
}

// hiddenBanner tells of the user's rules a group above takes all of: they
// never match, and the list is where to change that.
func (u *UI) hiddenBanner(c *ui.Context, p *pal, all []routes.Route) {
	hidden := u.routes.order.hidden
	if len(hidden) == 0 {
		return
	}
	ui.Column(c.Key("hidden")).Margin(0, 26, 12, 26).Padding(9, 14).Gap(4).Radius(p.r(10)).Background(p.hair2).Children(func() {
		ui.Text(c, fmt.Sprintf(i18n.T("你加的 %d 条规则不会生效：上面的路由组先匹配了它们。把它们的路由组拖到上面去，或者改掉挡住它们的规则。", "%d of your rules never match: a group above takes them first. Drag their group up, or change the rule in the way."), len(hidden))).
			FontSize(12).TextColor(p.ink)
		for i, h := range hidden[:min(3, len(hidden))] {
			sh := u.routes.order.shadows[routes.ShadowKey(h.service, h.rule.Text)]
			b := ui.ButtonBase(c.Key(fmt.Sprint("h", i))).Row().Gap(8).AlignItems(ui.Center).AlignSelf(ui.Start)
			b.OnClick(func() { u.openRoutes(h.group, h.service) }).Children(func() {
				ui.Text(c, h.rule.Shown()).Font(actMono).FontSize(12).TextColor(p.ink).NoWrap().Shrink(0)
				ui.Text(c, "· "+u.route(h.group).Name+" · "+strings.TrimPrefix(u.shadowWord(all, sh), i18n.T("不会生效：", "Never matches: "))).
					FontSize(11.5).TextColor(p.mute).SingleLine().Shrink(1).MinWidth(0)
			})
		}
		if len(hidden) > 3 {
			ui.Text(c, fmt.Sprintf(i18n.T("还有 %d 条", "%d more"), len(hidden)-3)).FontSize(11.5).TextColor(p.mute)
		}
	})
}

// exceptNote says which hosts a service's rules leave to the default
// route (国内网站's Google Play hosts).
func exceptNote(c *ui.Context, p *pal, s routes.Service) {
	if len(s.Except) == 0 {
		return
	}
	shown := make([]string, len(s.Except))
	for i, d := range s.Except {
		shown[i] = "*." + d
	}
	ui.Text(c, fmt.Sprintf(i18n.T("例外：%s 不走这里，交给「其他流量」（Google Play 下载应用用到它们）。", "Except %s, which go to Other instead (Google Play downloads apps from them)."), strings.Join(shown, i18n.T("、", ", ")))).
		FontSize(12).TextColor(p.mute)
}

// takenByDev is the developer network that takes a connection to host
// before every group, unless a rule of the user's inside it comes first.
func (u *UI) takenByDev(all []routes.Route, users []routes.UserRule, host string) (devnet.Rule, bool) {
	dev := u.routes.order.dev
	r, ok := dev.Match(host)
	if !ok {
		return r, false
	}
	for _, l := range routes.Lines(all, users) {
		if l.Mine && dev.Covers(l.Text) && l.Takes(host) {
			return r, false
		}
	}
	return r, true
}

// pointless says why rules of the default route change nothing.
func pointlessNote() string {
	return i18n.T("没命中上面任何规则的流量走这里。这里的规则排在最后，不会改变任何流量的去向，所以不能在这里加规则或服务；想让一个网站走这里的出口，就让上面的路由组都不认它。",
		"Traffic no rule above matched goes here. Rules here come last and change nothing, so none can be added here; to send a site out this way, leave it to no group above.")
}

// groupChoices are the groups a rule or service can be put in, in the
// list's order: the default route's rules would change nothing, so it is
// offered only to what is there already.
func groupChoices(all []routes.Route, cur string) (labels, ids []string) {
	for _, g := range routes.Listed(all) {
		if g.ID != routes.Default || cur == routes.Default {
			labels, ids = append(labels, routes.Title(g)), append(ids, g.ID)
		}
	}
	return labels, ids
}
