package main

// The user's own rules: "send this to that route". They are made where a
// connection shows the need (日志's 让它走…) and kept in 设置 > 规则, where
// they can be added by hand, moved, removed, and an address tested.

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

// ruleToast says what a change of rule did.
func (u *UI) ruleToast(r routes.UserRule, removed bool, closed int) string {
	var s string
	if removed {
		s = fmt.Sprintf(i18n.T("已删除规则：%s", "Rule removed: %s"), r.Describe())
	} else {
		s = fmt.Sprintf(i18n.T("%s 改走「%s」", "%s now goes by %s"), r.Describe(), routeName(r.Route))
	}
	if closed > 0 {
		s += fmt.Sprintf(i18n.T("；断开了 %d 条旧连接，重连后就按新规则走", "; %d old connections closed, they reconnect the new way"), closed)
	}
	return s
}

// changeRule adds r (or, removed, takes it out) from the main window, and
// says so in a toast.
func (u *UI) changeRule(r routes.UserRule, removed bool) {
	if u.app == nil {
		return
	}
	go func() {
		var n int
		var err error
		if removed {
			n, err = u.app.removeRule(r)
		} else {
			n, err = u.app.addRule(r)
		}
		if err != nil {
			u.toast(err.Error())
			return
		}
		u.toast(u.ruleToast(r, removed, n))
	}()
}

// ruleOf is the user's rule a connection matched, if it did.
func (u *UI) ruleOf(cn Conn) (routes.UserRule, bool) {
	for _, r := range u.snap.Settings.Rules {
		if r.Matched(cn.Rule, cn.Payload) {
			return r, true
		}
	}
	return routes.UserRule{}, false
}

// routeName is a route's name in the interface's language now.
func routeName(id string) string {
	for _, r := range routes.Template() {
		if r.ID == id {
			return routes.Title(r)
		}
	}
	return id
}

// routeExit is where a route's traffic leaves now, in a few words.
func (u *UI) routeExit(rv RouteView) string {
	if rv.Direct {
		return i18n.T("直接连接", "Direct")
	}
	if n, ok := u.node(rv.Node); ok {
		return n.Display
	}
	return "—"
}

// actRouteBtn is 让它走…: a menu of the routes. The first choice covers
// the site (api.openai.com → openai.com and what is under it); the
// submenu, the one name. A connection with no name gets a rule for its IP.
func (u *UI) actRouteBtn(c *ui.Context, p *pal, cn Conn) {
	mine, hasMine := u.ruleOf(cn)
	btn(c.Key("route"), p, i18n.T("让它走…", "Send via…")).PopoverMenuButton(func(m *ui.Menu) {
		m.MaxWidth(340)
		items := func(m *ui.Menu, r routes.UserRule) {
			for _, rv := range u.snap.Routes {
				r := r
				r.Route = rv.ID
				it := m.Item(routeName(rv.ID)).Radio(rv.ID == cn.Route).Trailing(u.routeExit(rv))
				if it.Chosen() {
					u.changeRule(r, false)
				}
			}
		}
		var wide routes.UserRule
		if cn.Host == "" {
			if cn.IP == "" {
				m.Item(i18n.T("这条连接没有域名或 IP", "This connection has no domain or IP")).Disabled(true)
				return
			}
			var err error
			if wide, err = routes.ParseRule(routes.KindIP, cn.IP, ""); err != nil {
				m.Item(err.Error()).Disabled(true)
				return
			}
		} else {
			wide = routes.UserRule{Kind: routes.KindSuffix, Value: routes.Site(cn.Host)}
		}
		m.Item(fmt.Sprintf(i18n.T("让 %s 走：", "Send %s via:"), wide.Describe())).Disabled(true)
		items(m, wide)
		if cn.Host != "" && cn.Host != wide.Value {
			m.Separator()
			only := routes.UserRule{Kind: routes.KindDomain, Value: strings.ToLower(cn.Host)}
			m.Submenu(fmt.Sprintf(i18n.T("只让 %s 走…", "Only %s via…"), cn.Host), func(m *ui.Menu) { items(m, only) })
		}
		if hasMine {
			m.Separator()
			if m.Item(i18n.T("删除这条规则", "Remove this rule")).Subtitle(mine.Describe() + " → " + routeName(mine.Route)).Danger().Chosen() {
				u.changeRule(mine, true)
			}
		}
		m.Separator()
		if m.Item(i18n.T("管理规则…", "Manage rules…")).Chosen() {
			u.openRules()
		}
	})
}

// openRules opens 设置 at 规则.
func (u *UI) openRules() {
	u.set.section = "rules"
	u.openSettings()
	u.settingsRedraw()
}

// mineTag marks what the user made, as against what came with bowel.
func mineTag(c *ui.Context, p *pal) ui.Element {
	return ui.Text(c, i18n.T("你的规则", "Yours")).FontSize(10.5).FontWeight(600).TextColor(p.ink2).
		Padding(1, 6).Radius(5).Background(p.hair2).NoWrap().Shrink(0)
}

// ---- 设置 > 规则 ----

// ruleEdit is 规则's typing: the new rule, and the address tested.
type ruleEdit struct {
	text, kind, route string
	h                 ui.Handle
	test              string
}

// ruleKinds are the choices for a new rule's kind; "" guesses.
var ruleKinds = []string{"", routes.KindDomain, routes.KindKeyword}

func ruleKindLabels() []string {
	return []string{i18n.T("自动识别", "Detect"), routes.KindName(routes.KindDomain), routes.KindName(routes.KindKeyword)}
}

// setRules changes the rules as setOpt changes an option (at once on the
// page, then in the app), and then closes the connections r matches that
// go elsewhere, so that it shows.
func (u *UI) setRules(r routes.UserRule, removed bool) {
	keep := r.Route
	if removed {
		keep = ""
	}
	u.setOptThen(func(s *settings.Settings) {
		if removed {
			s.Rules = routes.Remove(s.Rules, r)
		} else {
			s.Rules = routes.Upsert(s.Rules, r)
		}
	}, func() {
		u.settingsToast(u.ruleToast(r, removed, u.app.reroute(r, keep)))
	})
}

func (u *UI) routeNames() (ids, names []string) {
	for _, r := range routes.Template() {
		ids, names = append(ids, r.ID), append(names, routes.Title(r))
	}
	return
}

func (u *UI) rulesSection(c *ui.Context, p *pal, s settings.Settings) {
	sectionNote(c, p, i18n.T("你的规则排在内置规则前面，先匹配；改动马上生效。最快的加法：在「日志」里点开一条连接，选「让它走…」。",
		"Your rules come before the built-in ones and win; changes apply at once. The quickest way to add one: open a connection in Log and choose Send via…"))
	if !searching() {
		u.ruleAdder(c, p, s)
	}

	optHeading(c, p, fmt.Sprintf(i18n.T("你的规则 · %d", "Your rules · %d"), len(s.Rules)))
	if len(s.Rules) == 0 && !searching() {
		ui.Text(c, i18n.T("还没有。上面输入一个域名，或者在「日志」里从连接生成。", "None yet. Type a domain above, or make one from a connection in Log.")).
			FontSize(12).TextColor(p.faint).Padding(10, 0)
	}
	ids, names := u.routeNames()
	for _, rv := range u.snap.Routes {
		var mine []routes.UserRule
		for _, r := range s.Rules {
			if r.Route == rv.ID {
				mine = append(mine, r)
			}
		}
		if len(mine) == 0 {
			continue
		}
		if searching() { // each rule says its route in its menu
			for _, r := range mine {
				if optMatch(r.Value, routes.KindName(r.Kind), routeName(rv.ID)) {
					u.ruleRow(c, p, r, ids, names)
				}
			}
			continue
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(12, 0, 2, 0).Children(func() {
			swatch(c, p.routeColor(rv.ID), 9)
			ui.Text(c, routeName(rv.ID)).FontSize(12.5).FontWeight(700).TextColor(p.ink)
			ui.Text(c, u.routeExit(rv)).FontSize(11.5).TextColor(p.mute).SingleLine().Shrink(1).MinWidth(0)
		})
		for _, r := range mine {
			u.ruleRow(c, p, r, ids, names)
		}
	}

	optHeading(c, p, i18n.T("测试一个地址", "Test an address"))
	if !searching() {
		u.ruleTester(c, p, s)
	}

	optHeading(c, p, i18n.T("内置规则", "Built-in rules"))
	for _, r := range routes.Template() {
		desc := fmt.Sprintf(i18n.T("%[1]d 条 · %[2]s", "%[1]d · %[2]s"), len(r.Rules), routes.Describe(r))
		if r.ID == routes.Default {
			desc = i18n.T("其他规则都没命中的流量", "Traffic no other rule matched")
		}
		if !optMatch(routes.Title(r), desc) {
			continue
		}
		ui.Row(c).Gap(10).Padding(8, 0).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.hair2).Children(func() {
			swatch(c, p.routeColor(r.ID), 9).Shrink(0)
			ui.Text(c, routes.Title(r)).FontSize(12.5).FontWeight(600).TextColor(p.ink).Width(90).Shrink(0)
			ui.Text(c, desc).FontSize(11.5).TextColor(p.mute).SingleLine().Grow(1).Basis(0).MinWidth(0)
		})
	}
}

// ruleAdder is the new rule's line: what, which way, where to; and under
// it what bowel understood, before it is added.
func (u *UI) ruleAdder(c *ui.Context, p *pal, s settings.Settings) {
	e := &u.set.rule
	ids, names := u.routeNames()
	if len(ids) == 0 {
		return
	}
	if !slices.Contains(ids, e.route) {
		e.route = routes.Default
	}
	r, err := routes.ParseRule(e.kind, e.text, e.route)
	typed := strings.TrimSpace(e.text) != ""
	add := func() {
		if err != nil {
			u.set.toasts = append(u.set.toasts, err.Error())
			return
		}
		u.setRules(r, false)
		e.text = ""
	}
	ui.Column(c).Gap(8).Margin(14, 0, 0, 0).Padding(12, 14).Radius(10).Background(p.card).Border(1, p.hair).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			in := ui.TextInput(c.Key("rule-text"), &e.text).Bind(&e.h).Label(i18n.T("新规则", "New rule")).
				Placeholder(i18n.T("openai.com、*.example.com 或 10.0.0.0/8", "openai.com, *.example.com or 10.0.0.0/8")).
				Padding(6, 10).Radius(7).Background(p.paper).Border(1, p.hair).FontSize(13).Font(actMono).Grow(1).Basis(0).MinWidth(0)
			if e.h.Focused(c) {
				in.BorderColor(p.ink)
			}
			if in.Submitted() && typed {
				add()
			}
			b := btn(c.Key("rule-add"), p, i18n.T("添加", "Add"))
			if typed && err == nil {
				b.OnClick(add)
			} else {
				b.Opacity(0.45)
			}
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, i18n.T("匹配方式", "Match")).FontSize(12).TextColor(p.mute)
			kl := ruleKindLabels()
			cur := kl[max(0, slices.Index(ruleKinds, e.kind))]
			if ui.Select(c.Key("rule-kind"), &cur, kl).Label(i18n.T("匹配方式", "Match")).Width(118).Changed() {
				e.kind = ruleKinds[max(0, slices.Index(kl, cur))]
			}
			ui.Text(c, i18n.T("走", "via")).FontSize(12).TextColor(p.mute).Margin(0, 0, 0, 6)
			rc := names[max(0, slices.Index(ids, e.route))]
			if ui.Select(c.Key("rule-route"), &rc, names).Label(i18n.T("路由", "Route")).Width(112).Changed() {
				e.route = ids[max(0, slices.Index(names, rc))]
			}
		})
		// What it will do, said before it is done.
		ui.Row(c).Gap(6).AlignItems(ui.Center).MinHeight(16).Children(func() {
			switch {
			case !typed:
				ui.Text(c, i18n.T("域名会连同子域名一起匹配；写 IP 或 IP 段就按地址匹配。", "A domain matches its subdomains too; an IP or a range matches by address.")).
					FontSize(11.5).TextColor(p.faint)
			case err != nil:
				ui.Text(c, err.Error()).FontSize(11.5).TextColor(p.signal)
			default:
				swatch(c, p.routeColor(r.Route), 8)
				msg := fmt.Sprintf(i18n.T("%s 将走「%s」", "%s will go by %s"), r.Describe(), routeName(r.Route))
				if i := slices.IndexFunc(s.Rules, r.Same); i >= 0 && s.Rules[i].Route != r.Route {
					msg += fmt.Sprintf(i18n.T("，替换原来的「%s」", ", in place of %s"), routeName(s.Rules[i].Route))
				}
				ui.Text(c, msg).FontSize(11.5).TextColor(p.ink2)
			}
		})
	})
}

// ruleRow is one of the user's rules: what it matches, a choice of route
// to move it, and a button taking it out.
func (u *UI) ruleRow(c *ui.Context, p *pal, r routes.UserRule, ids, names []string) {
	key := r.Kind + ":" + r.Value
	ui.Row(c.Key(key)).Gap(10).Padding(7, 0, 7, 17).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.hair2).Children(func() {
		ui.Column(c).Gap(1).Grow(1).Basis(0).MinWidth(0).Children(func() {
			ui.Text(c, r.Value).Font(actMono).FontSize(12.5).TextColor(p.ink).SingleLine()
			ui.Text(c, routes.KindName(r.Kind)).FontSize(11).TextColor(p.mute)
		})
		cur := names[max(0, slices.Index(ids, r.Route))]
		if ui.Select(c.Key("mv"), &cur, names).Label(i18n.T("改走", "Move to")).Width(108).Changed() {
			if id := ids[max(0, slices.Index(names, cur))]; id != r.Route {
				moved := r
				moved.Route = id
				u.setRules(moved, false)
			}
		}
		iconBtn(c.Key("rm"), p, "×", 16, i18n.T("删除规则", "Remove rule")).OnClick(func() { u.setRules(r, true) })
	})
}

// ruleTester says where an address would go, and how sure that is: the
// user's rules and the built-in domains are certain; for the rest, GeoSite
// and GeoIP decide in the core, so what the address did lately is shown.
func (u *UI) ruleTester(c *ui.Context, p *pal, s settings.Settings) {
	e := &u.set.rule
	ui.Column(c).Gap(8).Padding(10, 0).Children(func() {
		ui.TextInput(c.Key("rule-test"), &e.test).Label(i18n.T("测试地址", "Address to test")).
			Placeholder(i18n.T("输入域名或 IP，如 chat.openai.com", "A domain or an IP, as chat.openai.com")).
			Padding(5, 9).Radius(7).Background(p.card).Border(1, p.hair).FontSize(12.5).Font(actMono)
		host := strings.TrimSpace(e.test)
		if host == "" {
			return
		}
		if t, err := routes.ParseRule(routes.KindDomain, host, ""); err == nil {
			host = t.Value
		} else if t, err := routes.ParseRule(routes.KindIP, host, ""); err == nil {
			host, _, _ = strings.Cut(t.Value, "/")
		} else {
			ui.Text(c, err.Error()).FontSize(11.5).TextColor(p.signal)
			return
		}
		id, rule, mine, sure := routes.Explain(host, s.Rules, routes.Template())
		var seen *Conn
		if !sure {
			for i := len(u.act.conns) - 1; i >= 0; i-- {
				if cn := u.act.conns[i]; (cn.Host == host || cn.IP == host) && cn.Route != "" {
					seen = &u.act.conns[i]
					break
				}
			}
			if seen != nil {
				id, rule = seen.Route, actRule(*seen)
			}
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
			if id == "" {
				ui.Text(c, i18n.T("没有命中域名规则：国内网站和局域网按 GeoSite / GeoIP 走「直连」，其余走「默认」。", "No domain rule matches it: mainland sites and the LAN go Direct by GeoSite / GeoIP, the rest by Default.")).
					FontSize(12).TextColor(p.ink2)
				return
			}
			swatch(c, p.routeColor(id), 9)
			ui.Text(c, fmt.Sprintf(i18n.T("走「%s」", "Goes by %s"), routeName(id))).FontSize(12.5).FontWeight(600).TextColor(p.ink)
			ui.Text(c, rule).Font(actMono).FontSize(11.5).TextColor(p.mute)
			switch {
			case mine:
				mineTag(c, p)
			case seen != nil:
				ui.Text(c, i18n.T("最近的连接实际这样走", "as a recent connection went")).FontSize(11.5).TextColor(p.faint)
			default:
				ui.Text(c, i18n.T("内置规则", "Built-in rule")).FontSize(11.5).TextColor(p.faint)
			}
		})
	})
}
