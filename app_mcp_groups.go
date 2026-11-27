package main

// Route groups for what has no window: the command line, the TUI and
// agents. These tools make, name, order and remove route groups, set each
// one's node strategy and test sites, and choose whose routing is used, as
// 路由组 (ui_routes.go) and a route's page (ui_home.go) do in the window.
// They are on both sockets, and a change follows the user's policy for
// agents as every other change does (app_mcp.go's change).
//
// While a provider's own config routes the traffic (app_imported.go), the
// groups in use are that config's: bowel's are kept for coming back, and a
// change of them here is refused with the way back, rather than made where
// it would not show.

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"slices"
	"strings"

	"github.com/oboo/moho/internal/geo"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/mcp"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

// groupView is a route group as agents and the command line read it.
type groupView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Position is its place in the order rules apply, from 1; the default
	// route, which takes what no group's rules match, is last.
	Position int  `json:"position"`
	Builtin  bool `json:"builtin"`         // shipped with bowel; false for one the user made
	Fixed    bool `json:"fixed,omitempty"` // always there and keeps its name: default, direct, reject
	Movable  bool `json:"movable"`
	// UsesNodes: it can leave by a node. direct and reject never do.
	UsesNodes bool `json:"usesNodes"`
	// Direct: its traffic goes out without a node now, by its nature or by
	// the user's choice (strategy direct), which keeps the strategy.
	Direct    bool   `json:"direct"`
	Reject    bool   `json:"reject,omitempty"`
	Locked    bool   `json:"locked,omitempty"`
	Node      string `json:"node,omitempty"` // the node in use now
	NodeLabel string `json:"nodeLabel,omitempty"`
	// Broken says why the strategy leaves it no way out now: scope, order,
	// fixed or all.
	Broken   string          `json:"broken,omitempty"`
	Strategy *groupStrategy  `json:"strategy,omitempty"`
	Sites    []routes.Target `json:"sites,omitempty"` // the main one first
	// SitesChanged: the sites are the user's, not the group's defaults.
	SitesChanged bool      `json:"sitesChanged,omitempty"`
	Services     []string  `json:"services"`
	Rules        []mcpRule `json:"rules"` // the user's rules sending traffic here
}

// groupStrategy is how a group chooses its node. Each mode keeps its
// settings while another is used.
type groupStrategy struct {
	Mode string `json:"mode"` // auto, order, fixed
	// auto's range, all three at once; empty is every one.
	Regions         []string `json:"regions"`
	Providers       []string `json:"providers"`
	ExceptProviders bool     `json:"exceptProviders,omitempty"`
	Nodes           []string `json:"nodes"`
	// By is how auto chooses: delay, or download (speed, tested every
	// DownloadEvery hours).
	By            string   `json:"by"`
	DownloadEvery int      `json:"downloadEvery,omitempty"`
	Order         []string `json:"order"`           // order: the first that works
	Fixed         string   `json:"fixed,omitempty"` // fixed: the node
	// Labels are the nodes named above as the window shows them.
	Labels        map[string]string `json:"labels,omitempty"`
	ProviderNames map[string]string `json:"providerNames,omitempty"`
}

func (a *App) strategyView(q routes.Policy) *groupStrategy {
	v := &groupStrategy{Mode: cmp.Or(q.Mode, routes.ModeAuto), Regions: nonNil(q.Regions), Providers: nonNil(q.Providers),
		ExceptProviders: q.ExceptProviders, Nodes: nonNil(q.Nodes), By: "delay", DownloadEvery: q.DownloadEvery, Order: nonNil(q.Order), Fixed: q.Fixed,
		Labels: map[string]string{}, ProviderNames: map[string]string{}}
	if q.DownloadEvery > 0 {
		v.By = "download"
	}
	for _, n := range append(append(slices.Clone(q.Nodes), q.Order...), q.Fixed) {
		if n != "" {
			v.Labels[n] = a.display(n)
		}
	}
	for _, id := range q.Providers {
		v.ProviderNames[id] = a.providerTitle(id)
	}
	return v
}

func nonNil(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

// orNil is l, or nil when it is empty: a policy keeps no empty lists, so
// that clearing one that was not set changes nothing.
func orNil(l []string) []string {
	if len(l) == 0 {
		return nil
	}
	return l
}

// groupViews are the route groups in their order; only one, when only is
// its id.
func (a *App) groupViews(only string) []groupView {
	snap, s := a.snapshot(), a.settings.Get()
	out := []groupView{}
	for i, r := range routes.Listed(a.routes()) {
		if only != "" && r.ID != only {
			continue
		}
		v := groupView{ID: r.ID, Name: routes.Title(r), Position: i + 1, Builtin: !r.Custom, Fixed: routes.Fixed(r.ID), Movable: routes.Movable(r.ID),
			UsesNodes: routes.ChoosesExit(r.ID), Direct: r.Direct, Reject: r.Reject, Locked: r.Locked, Services: []string{}}
		if v.UsesNodes {
			v.Strategy = a.strategyView(r.Policy)
			v.Sites = s.TargetsOf(r.ID)
			_, v.SitesChanged = s.RouteTargets[r.ID]
		}
		for _, rv := range snap.Routes {
			if rv.ID == r.ID {
				v.Node, v.Broken = rv.Node, rv.Broken
				if rv.Node != "" {
					v.NodeLabel = a.display(rv.Node)
				}
			}
		}
		for _, sv := range r.Services {
			if sv.Mine {
				continue
			}
			name := routes.ServiceTitle(sv)
			if sv.Off {
				name += i18n.T("（已关闭）", " (off)")
			}
			v.Services = append(v.Services, name)
		}
		var mine []routes.UserRule
		for _, u := range s.Rules {
			if u.Route == r.ID {
				mine = append(mine, u)
			}
		}
		v.Rules = mcpRules(mine)
		out = append(out, v)
	}
	return out
}

// ---- Changing them ----

// ownRouting refuses a change of bowel's route groups while an imported
// config's own are in use: it would not show.
func (a *App) ownRouting() error {
	if a.importedNow() != nil {
		return errors.New(i18n.T("正在使用导入配置自带的路由组：路由组、它们的顺序、节点策略和测速站点都由那份配置决定。select 类型的组仍可选成员（bowel route use、select_node）。先改回 bowel 自己的路由（bowel routing bowel、set_routing_source），才能管理路由组。",
			"An imported config's own route groups are in use: the groups, their order, strategies and test sites are that config's. A select group can still be given a member (bowel route use, select_node). Switch back to bowel's own routing first (bowel routing bowel, set_routing_source) to manage route groups."))
	}
	return nil
}

// groupOf is the route group s names (its id, or its name), for a change.
func (a *App) groupOf(s string) (routes.Route, error) {
	if err := a.ownRouting(); err != nil {
		return routes.Route{}, err
	}
	return a.findRoute(s)
}

// sameGroups says whether x and y set up the route groups alike: the
// groups, their order, names and strategies, the test sites and the
// user's rules.
func sameGroups(x, y settings.Settings) bool {
	if !reflect.DeepEqual(routes.Build(x.Routing, x.Rules), routes.Build(y.Routing, y.Rules)) || !reflect.DeepEqual(x.Rules, y.Rules) {
		return false
	}
	for _, m := range []map[string][]routes.Target{x.RouteTargets, y.RouteTargets} {
		for id := range m {
			if !slices.Equal(x.RouteTargets[id], y.RouteTargets[id]) {
				return false
			}
		}
	}
	return true
}

// editGroups changes the route groups by edit, as the user's policy says.
// edit runs on a copy first, so that what is wrong is said before anyone is
// asked and a change that changes nothing asks nothing; say words what it
// changes for the user's dialog. Then edit runs again on the settings as
// they are, and that is saved. It reports whether anything changed.
func (a *App) editGroups(ctx context.Context, s *mcp.Session, what words, say func(old, next settings.Settings) string, edit func(*settings.Settings) error) (bool, error) {
	if err := a.ownRouting(); err != nil {
		return false, err
	}
	old := a.settings.Get()
	next := old.Clone()
	if err := edit(&next); err != nil {
		return false, err
	}
	if err := next.Validate(); err != nil {
		return false, err
	}
	if sameGroups(old, next) {
		return false, nil
	}
	err := a.change(ctx, s, what, say(old, next), func() error {
		var editErr error
		var saved *settings.Settings
		err := a.updateSettings(func(st *settings.Settings) {
			keep := st.Clone()
			if editErr = edit(st); editErr != nil {
				*st = keep // nothing of a change that failed half way
			}
			saved = st
		})
		if editErr != nil {
			return editErr
		}
		if err != nil && saved != nil && sameGroups(a.settings.Get(), *saved) {
			// Saved; only the core could not take it up now, as when it
			// is not running. It does when it starts.
			log.Println("route group:", err)
			return nil
		}
		return err
	})
	return err == nil, err
}

// title is group id's name in settings s.
func title(s settings.Settings, id string) string {
	for _, r := range routes.Build(s.Routing, s.Rules) {
		if r.ID == id {
			return routes.Title(r)
		}
	}
	return id
}

// orderWords are the groups of s in their order, for people.
func orderWords(s settings.Settings) string {
	var names []string
	for _, r := range routes.Listed(routes.Build(s.Routing, s.Rules)) {
		names = append(names, routes.Title(r))
	}
	return strings.Join(names, " → ")
}

// place is where a group is to go: just before or after group to, or at
// at (from 0) among Plan.MovableOrder.
type place struct {
	to    string
	after bool
	at    int
}

// placeArgs are where agents say a group goes.
type placeArgs struct {
	Before, After string
	Position      int
}

// placeOf reads where p says group id goes; ok is false when it says
// nothing. id may be a group still to be made.
func (a *App) placeOf(id string, p placeArgs) (pl place, ok bool, err error) {
	n := 0
	for _, given := range []bool{p.Before != "", p.After != "", p.Position > 0} {
		if given {
			n++
		}
	}
	switch {
	case n > 1:
		return place{}, false, errors.New(i18n.T("before、after 和 position 只能给一个", "Give only one of before, after and position"))
	case p.Position > 0:
		return place{at: p.Position - 1}, true, nil
	case n == 0:
		return place{}, false, nil
	}
	r, err := a.findRoute(cmp.Or(p.Before, p.After))
	if err != nil {
		return place{}, false, err
	}
	switch {
	case r.ID == id:
		return place{}, false, errors.New(i18n.T("路由组不能排在自己前后", "A group can't go before or after itself"))
	case r.ID == routes.Default && p.After != "":
		return place{}, false, errors.New(i18n.T("其他流量总在最后，不能排在它后面", "The default route is always last: nothing goes after it"))
	case r.ID == routes.Default:
		return place{at: 1 << 20}, true, nil // last, which is before it
	}
	return place{to: r.ID, after: p.After != ""}, true, nil
}

// put moves group id of plan to pl.
func (pl place) put(plan *routes.Plan, id string) {
	at := pl.at
	if pl.to != "" {
		at = plan.PlaceAt(id, pl.to, pl.after)
	}
	plan.PlaceGroup(id, at)
}

// strategyArgs are set_route_strategy's: what is not given stays.
type strategyArgs struct {
	Route, Mode, Node, By string
	Order, Regions        *[]string
	Providers, Nodes      *[]string
	ExceptProviders       *bool
	DownloadEvery         *int
	Direct                *bool
}

// nodesOf are the nodes names name, in their order, each once.
func (a *App) nodesOf(names []string) ([]string, error) {
	var out []string
	for _, s := range names {
		n, err := a.findNode(strings.TrimSpace(s))
		if err != nil {
			return nil, err
		}
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out, nil
}

// strategyEdit reads p into what it changes of group r's policy.
func (a *App) strategyEdit(r routes.Route, p strategyArgs) (func(*settings.Settings) error, error) {
	if !routes.ChoosesExit(r.ID) {
		return nil, fmt.Errorf(i18n.T("路由组 %s 从不使用节点，没有节点策略", "Route group %s never uses a node, so it has no strategy"), r.ID)
	}
	mode := strings.ToLower(strings.TrimSpace(p.Mode))
	if mode != "" && !slices.Contains(routes.Modes, mode) {
		return nil, fmt.Errorf(i18n.T("未知的节点策略: %s", "Unknown node strategy: %s"), p.Mode)
	}
	var fixed string
	if p.Node != "" {
		n, err := a.findNode(p.Node)
		if err != nil {
			return nil, err
		}
		fixed = n
		mode = cmp.Or(mode, routes.ModeFixed)
	}
	var order, nodes, regions, providers []string
	var err error
	if p.Order != nil {
		if order, err = a.nodesOf(*p.Order); err != nil {
			return nil, err
		}
		mode = cmp.Or(mode, routes.ModeOrder)
	}
	if p.Nodes != nil {
		if nodes, err = a.nodesOf(*p.Nodes); err != nil {
			return nil, err
		}
	}
	if p.Regions != nil {
		for _, c := range *p.Regions {
			c = strings.ToUpper(strings.TrimSpace(c))
			if _, ok := geo.Country(c); !ok || len(c) != 2 {
				return nil, fmt.Errorf(i18n.T("不是国家或地区代码: %s（要写成 JP、US 这样的两个字母）", "Not a country code: %s (write two letters, as JP or US)"), c)
			}
			if !slices.Contains(regions, c) {
				regions = append(regions, c)
			}
		}
	}
	if p.Providers != nil {
		for _, s := range *p.Providers {
			pv, err := a.findProvider(strings.TrimSpace(s))
			if err != nil {
				return nil, err
			}
			if !slices.Contains(providers, pv.ID) {
				providers = append(providers, pv.ID)
			}
		}
	}
	every := -1 // as it is
	switch strings.ToLower(p.By) {
	case "":
	case "delay":
		every = 0
	case "download":
		every = cmp.Or(r.Policy.DownloadEvery, defaultDownloadEvery)
	default:
		return nil, fmt.Errorf(i18n.T("选法只能是 delay 或 download，不是 %s", "by is delay or download, not %s"), p.By)
	}
	if p.DownloadEvery != nil {
		if !slices.Contains(routes.DownloadEveries, *p.DownloadEvery) {
			return nil, fmt.Errorf(i18n.T("下载测速的间隔只能是 %v 小时", "Download tests are every %v hours"), routes.DownloadEveries)
		}
		if strings.EqualFold(p.By, "delay") {
			return nil, errors.New(i18n.T("按延迟选时没有下载测速的间隔", "Choosing by delay has no download test interval"))
		}
		every = *p.DownloadEvery
	}
	if p.Direct != nil && *p.Direct && a.settings.Get().Routing.Locked(r.ID) {
		return nil, errors.New(i18n.T("已锁定出口，先关掉出口锁定", "The exit is locked; turn the lock off first"))
	}
	cur := ""
	for _, rv := range a.snapshot().Routes {
		if rv.ID == r.ID {
			cur = rv.Node
		}
	}
	if providerOf(cur) == "" {
		cur = "" // DIRECT, or a group: no node to start a strategy from
	}
	return func(st *settings.Settings) error {
		st.Routing.ChangePolicy(r.ID, func(q *routes.Policy) {
			if fixed != "" {
				q.Fixed = fixed
			}
			if p.Order != nil {
				q.Order = orNil(order)
			}
			if p.Nodes != nil {
				q.Nodes = orNil(nodes)
			}
			if p.Regions != nil {
				q.Regions = orNil(regions)
			}
			if p.Providers != nil {
				q.Providers = orNil(providers)
			}
			if p.ExceptProviders != nil {
				q.ExceptProviders = *p.ExceptProviders
			}
			if len(q.Providers) == 0 {
				q.ExceptProviders = false // all of them: there is nothing to leave out
			}
			if every >= 0 {
				q.DownloadEvery = every
			}
			if p.Direct != nil {
				q.Direct = *p.Direct
			}
		})
		if mode != "" {
			st.Routing.SetStrategy(r.ID, mode, cur)
		}
		switch q := st.Routing.Policy(r.ID); {
		case p.ExceptProviders != nil && *p.ExceptProviders && len(q.Providers) == 0:
			return errors.New(i18n.T("要排除的来源还没选：同时给出 providers", "There are no providers to leave out: give providers too"))
		case q.Mode == routes.ModeFixed && q.Fixed == "":
			return errors.New(i18n.T("固定节点要先选一个节点：给出 node（命令行里用 --node）", "A fixed node needs a node: give node (--node on the command line)"))
		case q.Mode == routes.ModeOrder && len(q.Order) == 0:
			return errors.New(i18n.T("顺序备用要有节点：给出 order（命令行里用 bowel route order）", "A backup order needs nodes: give order (bowel route order on the command line)"))
		}
		return nil
	}, nil
}

// policyWords say what changes from old to next, a line each.
func (a *App) policyWords(old, next routes.Policy) []string {
	var out []string
	add := func(what, o, n string) {
		if o != n {
			out = append(out, fmt.Sprintf("%s: %s → %s", what, o, n))
		}
	}
	all := i18n.T("全部", "All")
	list := func(l []string, label func(string) string) string {
		if len(l) == 0 {
			return all
		}
		var w []string
		for _, s := range l {
			w = append(w, label(s))
		}
		return strings.Join(w, i18n.T("、", ", "))
	}
	same := func(s string) string { return s }
	exit := func(q routes.Policy) string {
		if q.Direct {
			return i18n.T("直接连接", "Direct")
		}
		return i18n.T("走节点", "Via a node")
	}
	by := func(q routes.Policy) string {
		if q.DownloadEvery > 0 {
			return fmt.Sprintf(i18n.T("下载速度，每 %d 小时", "download speed, every %d h"), q.DownloadEvery)
		}
		return i18n.T("延迟", "Delay")
	}
	providers := func(q routes.Policy) string {
		w := list(q.Providers, a.providerTitle)
		if q.ExceptProviders {
			w = i18n.T("排除 ", "all but ") + w
		}
		return w
	}
	fixed := func(q routes.Policy) string {
		if q.Fixed == "" {
			return "—"
		}
		return a.display(q.Fixed)
	}
	old.Mode, next.Mode = cmp.Or(old.Mode, routes.ModeAuto), cmp.Or(next.Mode, routes.ModeAuto)
	add(i18n.T("出口", "Exit"), exit(old), exit(next))
	add(i18n.T("节点策略", "Strategy"), modeName(old.Mode), modeName(next.Mode))
	add(i18n.T("固定节点", "Fixed node"), fixed(old), fixed(next))
	add(i18n.T("备用顺序", "Backup order"), list(old.Order, a.display), list(next.Order, a.display))
	add(i18n.T("地区", "Regions"), list(old.Regions, same), list(next.Regions, same))
	add(i18n.T("来源", "Providers"), providers(old), providers(next))
	add(i18n.T("挑选的节点", "Picked nodes"), list(old.Nodes, a.display), list(next.Nodes, a.display))
	add(i18n.T("选法", "By"), by(old), by(next))
	return out
}

// siteList makes targets of what was typed for group id: a URL each,
// whose name is the one it has already, or its host.
func siteList(s settings.Settings, id string, typed []string) ([]routes.Target, error) {
	had := s.TargetsOf(id)
	var out []routes.Target
	for _, u := range typed {
		t, err := routes.ParseTarget(u)
		if err != nil {
			return nil, err
		}
		if i := slices.IndexFunc(had, func(h routes.Target) bool { return h.URL == t.URL }); i >= 0 {
			t = had[i]
		}
		out = append(out, t)
	}
	if err := routes.ValidTargets(id, out); err != nil {
		return nil, err
	}
	return out, nil
}

func siteWords(list []routes.Target) string {
	var w []string
	for _, t := range list {
		w = append(w, t.Name+" ("+t.URL+")")
	}
	return strings.Join(w, i18n.T("、", ", "))
}

// routingView says whose routing is used.
func (a *App) routingView() map[string]any {
	if im := a.importedNow(); im != nil {
		id := a.settings.Get().ImportedRouting
		return map[string]any{"source": id, "name": a.providerTitle(id)}
	}
	return map[string]any{"source": "bowel", "name": "bowel"}
}

// ---- The tools ----

// routeGroupTools are the route groups' tools, for agents and the command
// line alike.
func (a *App) routeGroupTools() []mcp.Tool {
	routeArg := str("A route group's id, as list_routes gives them, or its name.")
	placeProps := func(props map[string]any) map[string]any {
		props["before"] = str("Put it just before this group (id or name).")
		props["after"] = str("Put it just after this group (id or name).")
		props["position"] = map[string]any{"type": "integer", "minimum": 1, "description": "Its place in the list, 1 being tried first. The default route is always last."}
		return props
	}
	list := func(desc string) map[string]any {
		return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
	}
	groups := func(only string) map[string]any {
		return map[string]any{"groups": a.groupViews(only)}
	}
	return []mcp.Tool{
		{
			Name: "list_route_groups", Title: "Route groups and their strategies", ReadOnly: true,
			Description: "Every route group as it is set up, in the order rules apply: the first group with a rule that matches takes a connection, and default, which takes what no group's rules match, is last. For each: id, name and position; builtin (shipped with bowel) or the user's; fixed groups (default, direct, reject) always exist and keep their names. Its strategy: mode auto (the fastest node of a range of regions, providers and hand-picked nodes, by delay or by download speed), order (the first node that works of a backup list) or fixed (one node); direct says its traffic goes out without a node now, the strategy being kept. Its exit lock, the node it uses now, its test sites (the main one first: its nodes' delays and the automatic choice go by it), its services and the user's rules sending traffic to it. With route, that group only. Under an imported config's own routing the groups are that config's: see imported, and list_routes.",
			Input:       obj(map[string]any{"route": routeArg}),
			Run: func(ctx context.Context, s *mcp.Session, raw json.RawMessage) (any, error) {
				var p struct{ Route string }
				if err := mcp.Args(raw, &p); err != nil {
					return nil, err
				}
				if a.importedNow() != nil {
					out := map[string]any{"groups": []groupView{}, "routing": a.routingView(),
						"note": "An imported config's own route groups are in use; bowel's own are kept for when its routing is used again (set_routing_source)."}
					if v := a.snapshot().Imported; v != nil {
						out["imported"] = mcpImported(v)
					}
					return out, nil
				}
				only := ""
				if p.Route != "" {
					r, err := a.findRoute(p.Route)
					if err != nil {
						return nil, err
					}
					only = r.ID
				}
				out := groups(only)
				out["routing"] = a.routingView()
				return out, nil
			},
		},
		{
			Name: "create_route_group", Title: "Create a route group",
			Description: "Makes a route group of the user's, on the fastest of every node and with no rules yet: add_rule sends traffic to it, by the id this returns. It goes after the other groups, before direct and reject while they are at the end, unless before, after or position says where. Its name must be new.",
			Input:       obj(placeProps(map[string]any{"name": str("What to call it.")}), "name"),
			Run: func(ctx context.Context, s *mcp.Session, raw json.RawMessage) (any, error) {
				var p struct {
					Name string
					placeArgs
				}
				if err := mcp.Args(raw, &p); err != nil {
					return nil, err
				}
				if err := a.ownRouting(); err != nil {
					return nil, err
				}
				name := strings.TrimSpace(p.Name)
				if msg := routes.GroupNameError(a.routes(), "", name); msg != "" {
					return nil, errors.New(msg)
				}
				pl, placed, err := a.placeOf("", p.placeArgs)
				if err != nil {
					return nil, err
				}
				var id string
				_, err = a.editGroups(ctx, s, words{"创建路由组", "Create a route group"}, func(old, next settings.Settings) string {
					return fmt.Sprintf(i18n.T("新建路由组「%s」", "New route group %s"), name) + "\n" + fmt.Sprintf(i18n.T("顺序：%s", "Order: %s"), orderWords(next))
				}, func(st *settings.Settings) error {
					id = st.Routing.AddGroup(st.Rules, name)
					if placed {
						pl.put(&st.Routing, id)
					}
					return nil
				})
				if err != nil {
					return nil, err
				}
				return map[string]any{"created": id, "groups": a.groupViews("")}, nil
			},
		},
		{
			Name: "rename_route_group", Title: "Rename a route group",
			Description: "Renames a route group. default, direct and reject keep their names.",
			Input:       obj(map[string]any{"route": routeArg, "name": str("Its new name.")}, "route", "name"),
			Run: func(ctx context.Context, s *mcp.Session, raw json.RawMessage) (any, error) {
				var p struct{ Route, Name string }
				if err := mcp.Args(raw, &p); err != nil {
					return nil, err
				}
				r, err := a.groupOf(p.Route)
				if err != nil {
					return nil, err
				}
				if routes.Fixed(r.ID) {
					return nil, fmt.Errorf(i18n.T("路由组 %s 的名字不能改", "Route group %s keeps its name"), r.ID)
				}
				name := strings.TrimSpace(p.Name)
				if msg := routes.GroupNameError(a.routes(), r.ID, name); msg != "" {
					return nil, errors.New(msg)
				}
				changed, err := a.editGroups(ctx, s, words{"给路由组改名", "Rename a route group"}, func(old, next settings.Settings) string {
					return title(old, r.ID) + " → " + title(next, r.ID)
				}, func(st *settings.Settings) error {
					st.Routing.RenameGroup(r.ID, name)
					return nil
				})
				if err != nil {
					return nil, err
				}
				return map[string]any{"changed": changed, "groups": a.groupViews(r.ID)}, nil
			},
		},
		{
			Name: "delete_route_group", Title: "Delete a route group", Destructive: true,
			Description: "Deletes a route group. Its services and the user's rules in it move to group to (default unless given), and its test sites go. default, direct and reject can't be deleted. An extra port pointing at it stays in the settings, closed.",
			Input:       obj(map[string]any{"route": routeArg, "to": str("The group that takes its services and rules; default unless given.")}, "route"),
			Run: func(ctx context.Context, s *mcp.Session, raw json.RawMessage) (any, error) {
				var p struct{ Route, To string }
				if err := mcp.Args(raw, &p); err != nil {
					return nil, err
				}
				r, err := a.groupOf(p.Route)
				if err != nil {
					return nil, err
				}
				if routes.Fixed(r.ID) {
					return nil, fmt.Errorf(i18n.T("路由组 %s 不能删除", "Route group %s can't be deleted"), r.ID)
				}
				to, err := a.findRoute(cmp.Or(p.To, routes.Default))
				if err != nil {
					return nil, err
				}
				if to.ID == r.ID {
					return nil, errors.New(i18n.T("服务和规则要交给另一个路由组", "Its services and rules must go to another group"))
				}
				_, err = a.editGroups(ctx, s, words{"删除路由组", "Delete a route group"}, func(old, next settings.Settings) string {
					return fmt.Sprintf(i18n.T("删除路由组「%s」，它的服务和规则交给「%s」", "Delete route group %s; its services and rules go to %s"), routes.Title(r), routes.Title(to))
				}, func(st *settings.Settings) error {
					if !st.Routing.DeleteGroup(st.Rules, r.ID, to.ID) {
						return fmt.Errorf(i18n.T("路由组 %s 不能删除", "Route group %s can't be deleted"), r.ID)
					}
					delete(st.RouteTargets, r.ID)
					return nil
				})
				if err != nil {
					return nil, err
				}
				return map[string]any{"deleted": r.ID, "movedTo": to.ID, "groups": a.groupViews("")}, nil
			},
		},
		{
			Name: "move_route_group", Title: "Move a route group",
			Description: "Moves a route group in the list. Rules apply group by group from the top, so of two groups with rules for the same traffic the one above takes it, however specific the rule below. Give one of before, after or position. The default route is always last and can't move.",
			Input:       obj(placeProps(map[string]any{"route": routeArg}), "route"),
			Run: func(ctx context.Context, s *mcp.Session, raw json.RawMessage) (any, error) {
				var p struct {
					Route string
					placeArgs
				}
				if err := mcp.Args(raw, &p); err != nil {
					return nil, err
				}
				r, err := a.groupOf(p.Route)
				if err != nil {
					return nil, err
				}
				if !routes.Movable(r.ID) {
					return nil, errors.New(i18n.T("其他流量接住所有没命中规则的流量，总在最后，不能移动", "The default route takes what no group's rules match, so it is always last and can't move"))
				}
				pl, placed, err := a.placeOf(r.ID, p.placeArgs)
				if err != nil {
					return nil, err
				}
				if !placed {
					return nil, errors.New(i18n.T("要给出 before、after 或 position", "Give before, after or position"))
				}
				changed, err := a.editGroups(ctx, s, words{"调整路由组顺序", "Reorder the route groups"}, func(old, next settings.Settings) string {
					return fmt.Sprintf(i18n.T("顺序：%s", "Order: %s"), orderWords(next))
				}, func(st *settings.Settings) error {
					pl.put(&st.Routing, r.ID)
					return nil
				})
				if err != nil {
					return nil, err
				}
				return map[string]any{"changed": changed, "groups": a.groupViews("")}, nil
			},
		},
		{
			Name: "set_route_strategy", Title: "Set a route group's strategy",
			Description: "Sets how a route group chooses its node; what is not given stays as it is, and each mode keeps its settings while another is used. mode: auto (the fastest node of its range, moving on when it fails), order (the first node that works of a backup list) or fixed (one node, nothing automatic). node sets the fixed node and, without mode, switches to fixed; order sets the backup list, first preferred, and without mode switches to order; with mode alone, fixed and order start from the node in use. auto's range is regions, providers and nodes at once, each empty for every one: regions are country codes (JP, US), providers ids or names (exceptProviders leaves those out and uses every other, one added later too), nodes the nodes picked by hand. by: delay, or download (every downloadEvery hours the fastest-answering nodes of the range download a test file and the fastest is used until it fails). direct sends the group's traffic out without a node, keeping its strategy for when it is turned off; not while the exit is locked. Nodes are named as list_nodes gives them. Not for direct and reject, which never use a node.",
			Input: obj(map[string]any{
				"route":           routeArg,
				"mode":            str("The strategy.", routes.Modes...),
				"node":            str("The fixed node: a node's name or label."),
				"order":           list("The backup list, first preferred: nodes' names or labels."),
				"regions":         list("auto's regions, as country codes; empty for all."),
				"providers":       list("auto's providers, by id or name; empty for all."),
				"exceptProviders": map[string]any{"type": "boolean", "description": "Leave the providers listed out instead of using only them."},
				"nodes":           list("auto's nodes picked by hand; empty for every node the regions and providers allow."),
				"by":              str("How auto chooses.", "delay", "download"),
				"downloadEvery":   map[string]any{"type": "integer", "enum": routes.DownloadEveries, "description": "Hours between download tests; implies by download."},
				"direct":          map[string]any{"type": "boolean", "description": "Send the group's traffic out without a node."},
			}, "route"),
			Run: func(ctx context.Context, s *mcp.Session, raw json.RawMessage) (any, error) {
				var p strategyArgs
				if err := mcp.Args(raw, &p); err != nil {
					return nil, err
				}
				r, err := a.groupOf(p.Route)
				if err != nil {
					return nil, err
				}
				edit, err := a.strategyEdit(r, p)
				if err != nil {
					return nil, err
				}
				changed, err := a.editGroups(ctx, s, words{"更改路由组的节点策略", "Change a route group's strategy"}, func(old, next settings.Settings) string {
					return routes.Title(r) + "\n" + strings.Join(a.policyWords(old.Routing.Policy(r.ID), next.Routing.Policy(r.ID)), "\n")
				}, edit)
				if err != nil {
					return nil, err
				}
				if changed && p.Direct != nil {
					a.closeRoute(r.ID) // so that where it leaves now shows
				}
				return map[string]any{"changed": changed, "groups": a.groupViews(r.ID)}, nil
			},
		},
		{
			Name: "set_route_sites", Title: "Set a route group's test sites",
			Description: "Sets the sites a route group is tested against, the main one first: its nodes' delays and the automatic choice go by the main site; the others are timed through the node in use, for reference. At most 6, as URLs (https:// may be left out). A new main site is tried through the node the group uses first, and refused if it fails there. reset puts the group's defaults back. Not for direct and reject, nor a group that goes direct now.",
			Input: obj(map[string]any{
				"route": routeArg,
				"sites": list("The sites' URLs, the main one first."),
				"reset": map[string]any{"type": "boolean", "description": "Put the group's default sites back."},
			}, "route"),
			Run: func(ctx context.Context, s *mcp.Session, raw json.RawMessage) (any, error) {
				var p struct {
					Route string
					Sites []string
					Reset bool
				}
				if err := mcp.Args(raw, &p); err != nil {
					return nil, err
				}
				r, err := a.groupOf(p.Route)
				if err != nil {
					return nil, err
				}
				if p.Reset == (len(p.Sites) > 0) {
					return nil, errors.New(i18n.T("要给出 sites 或 reset 其中之一", "Give sites or reset, one of them"))
				}
				if !r.NodeGroups() {
					return nil, errors.New(i18n.T("这条路由不使用节点", "This route uses no node"))
				}
				st := a.settings.Get()
				old := st.TargetsOf(r.ID)
				var next []routes.Target // nil: the defaults
				if p.Reset {
					if _, custom := st.RouteTargets[r.ID]; !custom {
						return map[string]any{"changed": false, "groups": a.groupViews(r.ID)}, nil
					}
				} else {
					if next, err = siteList(st, r.ID, p.Sites); err != nil {
						return nil, err
					}
					if slices.Equal(next, old) {
						return map[string]any{"changed": false, "groups": a.groupViews(r.ID)}, nil
					}
				}
				shown := next
				if shown == nil {
					shown = settings.Settings{TestURL: st.TestURL}.TargetsOf(r.ID) // the defaults
				}
				err = a.change(ctx, s, words{"更改测速站点", "Change the test sites"}, fmt.Sprintf("%s: %s → %s", routes.Title(r), siteWords(old), siteWords(shown)),
					func() error { return a.setRouteTargets(r.ID, next) })
				if err != nil {
					return nil, err
				}
				return map[string]any{"changed": true, "groups": a.groupViews(r.ID)}, nil
			},
		},
		{
			Name: "set_routing_source", Title: "Choose whose routing is used", Destructive: true,
			Description: "Chooses whose route groups and rules are used: bowel's own (source bowel), or those of a provider whose config has groups and rules of its own (source: its id or name, as list_providers gives them). Either way the other is kept, for coming back. Open connections are closed so that they reconnect the new way.",
			Input:       obj(map[string]any{"source": str("bowel, or a provider's id or name.")}, "source"),
			Run: func(ctx context.Context, s *mcp.Session, raw json.RawMessage) (any, error) {
				var p struct{ Source string }
				if err := mcp.Args(raw, &p); err != nil {
					return nil, err
				}
				id := ""
				if src := strings.TrimSpace(p.Source); !strings.EqualFold(src, "bowel") {
					pv, err := a.findProvider(src)
					if err != nil {
						return nil, err
					}
					id = pv.ID
				}
				from := a.routingView()
				if from["source"] == cmp.Or(id, "bowel") {
					return map[string]any{"changed": false, "routing": from}, nil
				}
				to := "bowel"
				if id != "" {
					to = a.providerTitle(id)
				}
				err := a.change(ctx, s, words{"更改使用的路由", "Change which routing is used"}, fmt.Sprintf(i18n.T("路由：%s → %s", "Routing: %s → %s"), from["name"], to),
					func() error { return a.setImportedRouting(id) })
				if err != nil {
					return nil, err
				}
				return map[string]any{"changed": true, "routing": a.routingView()}, nil
			},
		},
	}
}
