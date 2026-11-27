package cli

// The TUI's keys for route groups, on the overview's route rows: n makes
// one, s sets its strategy, J and K move it down and up, e sets its test
// sites, R renames and d deletes it; in the node picker, o puts a node in
// the backup order or takes it out. The tools are groups.go's.

import (
	"fmt"
	"slices"
	"strings"

	"github.com/oboo/moho/internal/i18n"
)

// groupHints are the overview's keys for the route in front; all of them
// with all set, for the help.
func (t *tui) groupHints(all bool) []hint {
	if _, _, ok := t.selectedRoute(); !ok && !all {
		return nil // on a switch: n works, and the help says so
	}
	return []hint{{"s", i18n.T("节点策略", "Strategy")}, {"J K", i18n.T("下移/上移", "Move down/up")}, {"e", i18n.T("测速站点", "Test sites")},
		{"n", i18n.T("新路由组", "New group")}, {"R", i18n.T("重命名", "Rename")}, {"d", i18n.T("删除路由组", "Delete group")}}
}

// selectedRoute is the route in front on the overview, and its row among
// the routes, when a route is in front.
func (t *tui) selectedRoute() (route, int, bool) {
	i := t.sel[tabHome] - len(t.toggles())
	if i < 0 || i >= len(t.st.Routes) {
		return route{}, i, false
	}
	return t.st.Routes[i], i, true
}

// groupOf reads route r as a route group, saying why when it can't.
func (t *tui) groupOf(r route) (groupView, bool) {
	var out groupsAnswer
	if _, err := t.c.get("list_route_groups", map[string]any{"route": r.ID}, &out); err != nil {
		t.say(err.Error(), true)
		return groupView{}, false
	}
	g, ok := out.group(r.ID)
	if !ok {
		t.say(i18n.T("正在使用导入配置自带的路由组，这里不能管理路由组", "An imported config's own groups are in use; route groups can't be managed here"), true)
	}
	return g, ok
}

// groupKey takes a key of the overview's that is about route groups.
func (t *tui) groupKey(k rune) {
	applying := i18n.T("正在应用…", "Applying…")
	if k == 'n' {
		t.input = &prompt{label: i18n.T("新建路由组（名字）", "New route group (its name)"), submit: func(name string) {
			t.call(applying, fmt.Sprintf(i18n.T("已新建路由组 %s", "Created route group %s"), name), "create_route_group", map[string]any{"name": name})
		}}
		return
	}
	if !strings.ContainsRune("sJKeRd", k) {
		return
	}
	r, i, ok := t.selectedRoute()
	if !ok {
		t.say(i18n.T("先选中一个路由组", "Choose a route group first"), true)
		return
	}
	switch k {
	case 's':
		t.chooseStrategy(r)
	case 'J', 'K':
		d := 1
		if k == 'K' {
			d = -1
		}
		// Positions count from 1; the default route is last and stays there.
		at := i + 1 + d
		if r.ID == "default" || at < 1 || at > len(t.st.Routes)-1 {
			t.say(i18n.T("不能再移了：其他流量总在最后", "It can't go further: the default route is always last"), true)
			return
		}
		t.act(applying, fmt.Sprintf(i18n.T("%s 现在排第 %d", "%s is number %d now"), r.Name, at), func() error {
			_, err := t.c.call("move_route_group", map[string]any{"route": r.ID, "position": at})
			if err == nil {
				t.sel[tabHome] += d // the row goes with it
			}
			return err
		})
	case 'e':
		g, ok := t.groupOf(r)
		if !ok {
			return
		}
		if len(g.Sites) == 0 || g.Direct {
			t.say(i18n.T("这条路由不使用节点", "This route uses no node"), true)
			return
		}
		var urls []string
		for _, s := range g.Sites {
			urls = append(urls, s.URL)
		}
		t.input = &prompt{label: i18n.T("测速站点（空格隔开，第一个是主目标；reset 恢复默认）", "Test sites (separated by spaces, the main one first; reset for the defaults)"), buf: []rune(strings.Join(urls, " ")),
			submit: func(s string) {
				args := map[string]any{"route": r.ID}
				if strings.EqualFold(s, "reset") {
					args["reset"] = true
				} else {
					args["sites"] = strings.Fields(s)
				}
				t.call(i18n.T("正在试主目标…", "Trying the main site…"), fmt.Sprintf(i18n.T("%s 的测速站点已更新", "%s's test sites are set"), r.Name), "set_route_sites", args)
			}}
	case 'R':
		t.input = &prompt{label: fmt.Sprintf(i18n.T("把 %s 改名为", "Rename %s to"), r.Name), buf: []rune(r.Name), submit: func(name string) {
			t.call(applying, fmt.Sprintf(i18n.T("已改名为 %s", "Renamed to %s"), name), "rename_route_group", map[string]any{"route": r.ID, "name": name})
		}}
	case 'd':
		t.confirm = &ask{fmt.Sprintf(i18n.T("删除路由组 %s？它的服务和规则交给其他流量", "Delete route group %s? Its services and rules go to the default route"), r.Name), func() {
			t.call(applying, fmt.Sprintf(i18n.T("已删除 %s", "Removed %s"), r.Name), "delete_route_group", map[string]any{"route": r.ID})
		}}
	}
}

// chooseStrategy opens the list of route r's strategies: the three, and
// going without a node.
func (t *tui) chooseStrategy(r route) {
	g, ok := t.groupOf(r)
	if !ok {
		return
	}
	if !g.UsesNodes || g.Strategy == nil {
		t.say(i18n.T("这条路由不使用节点", "This route uses no node"), true)
		return
	}
	options := append(slices.Clone(strategies), "direct")
	labels := []string{
		strategyName("auto") + "  " + i18n.T("范围里最快的节点，不通就换", "the fastest node of its range, moving on when it fails"),
		strategyName("order") + "  " + i18n.T("按顺序用第一个能用的节点（在选节点里按 o 排）", "the first node that works of a list (o in the node list sets it)"),
		strategyName("fixed") + "  " + i18n.T("一个节点，不自动切换", "one node, nothing automatic"),
		i18n.T("直接连接", "Direct") + "  " + i18n.T("不经过节点，节点策略保留", "no node; the strategy is kept"),
	}
	cur := slices.Index(options, g.Strategy.Mode)
	if g.Direct {
		cur = len(options) - 1
	}
	t.choose = &chooser{title: fmt.Sprintf(i18n.T("%s的节点策略", "%s's strategy"), g.Name), options: options, labels: labels, current: cur, sel: max(cur, 0), done: func(o string) {
		args := map[string]any{"route": r.ID}
		word := strategyName(o)
		if o == "direct" {
			args["direct"], word = true, i18n.T("直接连接", "Direct")
		} else {
			args["mode"] = o
			if g.Direct {
				args["direct"] = false
			}
		}
		t.call(i18n.T("正在应用…", "Applying…"), g.Name+": "+word, "set_route_strategy", args)
	}}
}

// loadOrder reads the picker's route's backup order.
func (t *tui) loadOrder() {
	p := t.pick
	var out groupsAnswer
	if _, err := t.c.get("list_route_groups", map[string]any{"route": p.route.ID}, &out); err != nil {
		return
	}
	if g, ok := out.group(p.route.ID); ok && g.Strategy != nil {
		p.order = g.Strategy.Order
	}
}

// toggleOrder puts node n in the picker's route's backup order, last, or
// takes it out, and makes the route use that order.
func (t *tui) toggleOrder(n node) {
	p := t.pick
	t.loadOrder()
	order := slices.Clone(p.order)
	if i := slices.Index(order, n.Name); i >= 0 {
		order = slices.Delete(order, i, i+1)
	} else {
		order = append(order, n.Name)
	}
	t.act(i18n.T("正在应用…", "Applying…"), fmt.Sprintf(i18n.T("%s 的备用顺序有 %d 个节点", "%s's backup order has %d nodes"), p.route.Name, len(order)), func() error {
		_, err := t.c.call("set_route_strategy", map[string]any{"route": p.route.ID, "order": order})
		return err
	})
	if t.pick != nil {
		t.loadOrder()
	}
}
