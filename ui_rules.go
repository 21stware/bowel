package main

// The user's own rules: "send this to that route". They are made where a
// connection shows the need (日志's 让它走…) and kept in 路由组, where they
// can be added by hand, moved, removed, and an address tested.

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/routes"
)

// ruleToast says what a change of rule did.
func (u *UI) ruleToast(r routes.UserRule, removed bool, closed int) string {
	var s string
	if removed {
		s = fmt.Sprintf(i18n.T("已删除规则：%s", "Rule removed: %s"), r.Describe())
	} else {
		s = fmt.Sprintf(i18n.T("%s 改走「%s」", "%s now goes by %s"), r.Describe(), u.routeName(r.Route))
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

// routeName is a route group's name as the interface shows it.
func (u *UI) routeName(id string) string {
	if r := u.route(id); r.ID != "" {
		return r.Name
	}
	return id
}

// noNodeWord is what becomes of the traffic of a route with no node.
func noNodeWord(rv RouteView) string {
	if rv.Reject {
		return i18n.T("已拦截", "Blocked")
	}
	return i18n.T("直接连接", "Direct")
}

// routeExit is where a route's traffic leaves now, in a few words.
func (u *UI) routeExit(rv RouteView) string {
	if rv.Direct {
		return noNodeWord(rv)
	}
	if n, ok := u.node(rv.Node); ok {
		return n.Display
	}
	return "—"
}

// actRouteBtn is 让它走…: a menu of the routes. The first choice covers
// the site (api.openai.com → openai.com and what is under it); the
// submenu, the one name. A connection with no name gets a rule for its IP.
// Where the core knows who connects (routes.AppRules), a second submenu
// sends everything of the connection's application.
func (u *UI) actRouteBtn(c *ui.Context, p *pal, cn Conn) {
	mine, hasMine := u.ruleOf(cn)
	btn(c.Key("route"), p, i18n.T("让它走…", "Send via…")).PopoverMenuButton(func(m *ui.Menu) {
		m.MaxWidth(340)
		items := func(m *ui.Menu, r routes.UserRule) {
			for _, rv := range u.snap.Routes {
				r := r
				r.Route = rv.ID
				it := m.Item(u.routeName(rv.ID)).Radio(rv.ID == cn.Route).Trailing(u.routeExit(rv))
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
		if app, err := routes.ParseRule(routes.KindApp, cn.Process, ""); err == nil && routes.AppRules {
			m.Separator()
			m.Submenu(fmt.Sprintf(i18n.T("让 %s 的所有连接走…", "Everything from %s via…"), app.Value), func(m *ui.Menu) { items(m, app) })
			if u.tick.AppsUnknown { // the name may be bowel's own lookup's
				m.Item(appsUnknownNote()).Disabled(true)
			}
		}
		if hasMine {
			m.Separator()
			if m.Item(i18n.T("删除这条规则", "Remove this rule")).Subtitle(mine.Describe() + " → " + u.routeName(mine.Route)).Danger().Chosen() {
				u.changeRule(mine, true)
			}
		}
		m.Separator()
		if m.Item(i18n.T("管理规则…", "Manage rules…")).Chosen() {
			u.openRules(cmp.Or(cn.Route, routes.Default))
		}
	})
}

// appsUnknownNote says what Tick.AppsUnknown means for the user.
func appsUnknownNote() string {
	return i18n.T("内核现在认不出这台电脑上的连接来自哪个应用，按应用的规则不会生效。", "The core can't tell which app on this computer opened a connection, so rules for apps won't match for now.")
}

// openRules opens 路由组 at the service holding r's traffic, or the group's
// 我添加的.
func (u *UI) openRules(group string) {
	u.openRoutes(group, routes.MineID(group))
}

// mineTag marks what the user made, as against what came with bowel.
func mineTag(c *ui.Context, p *pal) ui.Element {
	return ui.Text(c, i18n.T("你的规则", "Yours")).FontSize(10.5).FontWeight(600).TextColor(p.ink2).
		Padding(1, 6).Radius(p.r(5)).Background(p.hair2).NoWrap().Shrink(0)
}
