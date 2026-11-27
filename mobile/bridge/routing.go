package bridge

// The route groups at run time, as the desktop has them in app_routing.go,
// and what the 路由组 pages read and change.

import (
	"context"
	"errors"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/oboo/moho/internal/geo"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/mihomo"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
	"github.com/oboo/moho/internal/speed"
	"gopkg.in/yaml.v3"
)

const siteTimeout = 5 * time.Second

// siteDelay is one of a route's other test sites, timed through the node
// the route uses.
type siteDelay struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Delay  int    `json:"delay"`
	Tested bool   `json:"tested"`
	// Refused: the test failed because the site answered and turned the
	// node away, not because nothing answered.
	Refused bool `json:"refused,omitempty"`
}

func mainURLs(s settings.Settings) map[string]string {
	out := map[string]string{}
	for _, r := range routes.Build(s.Routing, nil) {
		if !r.Direct {
			out[r.ID] = s.TargetsOf(r.ID)[0].URL
		}
	}
	return out
}

// coreSame says whether two settings give the core the same config.
func coreSame(old, next settings.Settings) bool {
	if reflect.DeepEqual(coreView(old.Routing), coreView(next.Routing)) { // a strategy is a selector's position
		next.Routing = old.Routing
	}
	if maps.Equal(mainURLs(old), mainURLs(next)) { // only the main targets are in the config
		next.RouteTargets = old.RouteTargets
	}
	return reflect.DeepEqual(old, next)
}

func coreView(p routes.Plan) routes.Plan {
	p = p.Clone()
	p.ChangePolicy(routes.Default, func(*routes.Policy) {}) // a plan never changed lists no groups yet
	for i := range p.Groups {
		g := &p.Groups[i]
		g.Name, g.Policy.Fixed = "", ""
		if g.Policy.Mode == routes.ModeOrder || len(g.Policy.Order) > 0 {
			g.Policy.Mode = routes.ModeOrder
		} else {
			g.Policy.Mode = ""
		}
	}
	for i := range p.Services {
		p.Services[i].Name = ""
	}
	return p
}

// fileNodes are the full names of the enabled Providers' nodes, from their
// files, since the config that loads them must already list them.
func (x *bridge) fileNodes() []string {
	var out []string
	for _, p := range x.store.List() {
		if p.Enabled {
			out = append(out, fullNames(x.store.Names(p.ID), p.ID)...)
		}
	}
	slices.Sort(out)
	return out
}

func inScope(q routes.Policy, name, provider, country string) bool {
	return q.FromProvider(provider) &&
		(len(q.Regions) == 0 || slices.Contains(q.Regions, country)) &&
		(len(q.Nodes) == 0 || slices.Contains(q.Nodes, name))
}

// nodeLists are the node lists the config needs for the groups' strategies.
func (x *bridge) nodeLists(all []routes.Route) (scopes, orders map[string][]string) {
	scopes, orders = map[string][]string{}, map[string][]string{}
	names := x.fileNodes()
	for _, r := range all {
		if !r.NodeGroups() {
			continue
		}
		q := r.Policy
		if q.Scoped() {
			list := []string{}
			for _, n := range names {
				if inScope(q, n, providerOf(n), placeOf(n).Country) {
					list = append(list, n)
				}
			}
			scopes[r.ID] = list
		}
		if q.Mode == routes.ModeOrder || len(q.Order) > 0 {
			list := []string{}
			for _, n := range q.Order {
				if slices.Contains(names, n) {
					list = append(list, n) // one no longer there is skipped
				}
			}
			orders[r.ID] = list
		}
	}
	return scopes, orders
}

// wanted is what route r's selector should be on.
func wanted(r routes.Route) string {
	switch r.Policy.Mode {
	case routes.ModeOrder:
		return routes.Backup(r)
	case routes.ModeFixed:
		if r.Policy.Fixed != "" {
			return r.Policy.Fixed
		}
	}
	return routes.Auto(r)
}

// followPolicies puts every group's selector where its strategy says.
func (x *bridge) followPolicies(ctx context.Context, c *mihomo.Client) error {
	if x.importedNow() != nil {
		return nil // the core's selectors are the source's groups, not the strategies'
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	px, err := c.Proxies(ctx)
	if err != nil {
		return err
	}
	var first error
	for _, r := range x.routes() {
		g, ok := px[routes.Group(r)]
		if !r.NodeGroups() || !ok {
			continue
		}
		want := wanted(r)
		if g.Now == want || !slices.Contains(g.All, want) {
			continue // a fixed node that is gone shows as broken
		}
		if err := c.Select(ctx, routes.Group(r), want); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// ---- what the pages read ----

type ruleData struct {
	Text     string `json:"text"`
	Kind     string `json:"kind"`
	KindName string `json:"kindName"`
	Value    string `json:"value"`
	Shown    string `json:"shown"`
	Builtin  bool   `json:"builtin"`
	Off      bool   `json:"off"`
	// Unlisted is a rule of the user's whose GeoSite or GeoIP list the data
	// doesn't have: it doesn't count until it does.
	Unlisted bool `json:"unlisted,omitempty"`
	// Shadowed says what takes all of its traffic first, a group above it or
	// the tailnet: it never matches. Ahead is a rule of the user's inside the
	// tailnet, which goes before the tailnet's own.
	Shadowed string `json:"shadowed,omitempty"`
	Ahead    bool   `json:"ahead,omitempty"`
}

type serviceData struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Builtin bool       `json:"builtin"`
	Mine    bool       `json:"mine"`
	Off     bool       `json:"off"`
	Changed bool       `json:"changed"`
	Rules   []ruleData `json:"rules"`
	// Set is there for a rule set, whose rules are the source's and not in Rules.
	Set *ruleSetData `json:"set,omitempty"`
	// Except are the domains its rules leave to 其他流量 (国内网站's Google
	// Play hosts).
	Except []string `json:"except,omitempty"`
}

type groupData struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Summary  string `json:"summary"`
	Direct   bool   `json:"direct"`
	Reject   bool   `json:"reject"`
	Custom   bool   `json:"custom"`
	Movable  bool   `json:"movable"`  // all but the default route, which stays last
	Chooses  bool   `json:"chooses"`  // between a node and going direct, which Direct then says
	Locked   bool   `json:"locked"`   // its exit lock: by a node or not at all
	Lockable bool   `json:"lockable"` // whether the lock can be put on it
	// Pointless: a rule or service here changes nothing, its rules coming
	// last, right before what takes the rest anyway (the default route).
	Pointless bool            `json:"pointless"`
	Mode      string          `json:"mode"`
	Regions   []string        `json:"regions"`
	Providers []string        `json:"providers"`
	Except    bool            `json:"exceptProviders"` // Providers are the ones left out
	Order     []string        `json:"order"`
	Fixed     string          `json:"fixed"`
	Now       string          `json:"now"`
	Services  []serviceData   `json:"services"`
	Targets   []routes.Target `json:"targets"`
	Sites     []siteDelay     `json:"sites"`
}

type pickData struct {
	Name       string `json:"name"`
	Display    string `json:"display"`
	Provider   string `json:"provider"`
	ProviderID string `json:"providerId"`
	Country    string `json:"country"`
	Flag       string `json:"flag"`
	Delay      int    `json:"delay"` // -1 untested, 0 timed out
	// Routes is the node's delay on each other route's main target (see
	// nodeData.Routes); Delay is the default route's.
	Routes map[string]int `json:"routes"`
	// Refused are the routes on whose main target the node's failed test
	// was the site's refusal (nodeData.Refused).
	Refused []string `json:"refused,omitempty"`
}

type regionData struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type providerPick struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type routingData struct {
	// Imported is there while a provider's own routing is in use: then
	// Groups are its routes, which the page shows and does not edit.
	Imported  *importedData  `json:"imported,omitempty"`
	Groups    []groupData    `json:"groups"`
	Nodes     []pickData     `json:"nodes"`
	Regions   []regionData   `json:"regions"`
	Providers []providerPick `json:"providers"`
	Kinds     []kindData     `json:"kinds"`
	// Tailnet: the tailnet's rules are in use, before every group's, as the
	// list shows at its top. Ahead are the user's rules inside it, which go
	// before those; Hidden the user's that a group above takes all of.
	Tailnet bool `json:"tailnet"`
	Ahead   int  `json:"ahead"`
	Hidden  int  `json:"hidden"`
}

type kindData struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (x *bridge) routing() routingData {
	s := x.set.Get()
	im := x.importedNow()
	all := x.routes()
	x.mu.Lock()
	px, prov := x.proxies, x.provNodes
	x.mu.Unlock()
	rd := readingOf(s, im)
	rd.refusals = &x.refused
	out := routingData{Groups: []groupData{}, Nodes: []pickData{}, Regions: []regionData{}, Providers: []providerPick{}, Kinds: []kindData{}}
	out.Imported = x.imported(s, im, all, px, prov)
	kinds := routes.Kinds
	if routes.AppRules { // an app's package, on Android from 10
		kinds = routes.AllKinds()
	}
	for _, k := range kinds {
		out.Kinds = append(out.Kinds, kindData{k, routes.KindName(k)})
	}
	count := map[string]int{}
	for _, p := range x.store.List() {
		if !p.Enabled {
			continue
		}
		out.Providers = append(out.Providers, providerPick{p.ID, p.Name})
		for _, n := range fullNames(x.store.Names(p.ID), p.ID) {
			pl := placeOf(n)
			tests, others := rd.tests(px[n])
			d := -1
			if ms, ok := rd.last(tests); ok {
				d = ms
			}
			out.Nodes = append(out.Nodes, pickData{n, display(n), p.Name, p.ID, pl.Country, strings.ToLower(pl.Country), d, others, rd.refused(n, px[n])})
			if pl.Country != "" {
				count[pl.Country]++
			}
		}
	}
	for code, n := range count {
		name := code
		if p, ok := geo.Country(code); ok {
			name = geo.Display(p.City)
		}
		out.Regions = append(out.Regions, regionData{code, name, n})
	}
	slices.SortFunc(out.Regions, func(a, b regionData) int { return b.Count - a.Count })
	// What the list's order makes of the rules: an imported config's routing
	// has its own order, which the page doesn't edit.
	dev := tailnetOverlay(s.Tailnet)
	var covers func(string) bool
	if !dev.Empty() && im == nil {
		covers = dev.Covers
	}
	shadows := map[string]routes.Shadow{}
	if im == nil {
		shadows = routes.Shadowed(all, covers, x.ruleSets.Match)
		out.Tailnet = covers != nil
	}
	for _, r := range all {
		g := groupData{ID: r.ID, Name: routes.Title(r), Summary: routes.Describe(r), Direct: r.Direct, Reject: r.Reject, Custom: r.Custom, Movable: routes.Movable(r.ID), Chooses: choosesExit(r.ID), Locked: r.Locked, Lockable: routes.Lockable(r.ID),
			Mode: r.Policy.Mode, Regions: nonNil(r.Policy.Regions), Providers: nonNil(r.Policy.Providers), Except: r.Policy.ExceptProviders, Order: nonNil(r.Policy.Order), Fixed: r.Policy.Fixed,
			Services: []serviceData{}, Targets: []routes.Target{}, Sites: []siteDelay{}}
		if im != nil {
			g.Movable, g.Chooses, g.Locked, g.Lockable = false, false, false, false
		} else {
			g.Pointless = routes.Pointless(r.ID)
		}
		if !r.Direct && !r.Reject {
			node := x.nodeOf(px, prov, r, im)
			g.Now = display(node)
			g.Targets = s.TargetsOf(r.ID)
			for _, t := range g.Targets[1:] { // the node's own tests of them, as the core keeps them
				sd := siteDelay{Name: t.Name, URL: t.URL}
				sd.Delay, sd.Tested = rd.last(px[node].ResultsOf(t.URL)) // a failure is 0: a timeout, or the site's refusal
				sd.Refused = rd.refusedOn(node, t.URL, px[node])
				g.Sites = append(g.Sites, sd)
			}
		}
		for _, sv := range r.Services {
			d := serviceData{ID: sv.ID, Name: routes.ServiceTitle(sv), Builtin: sv.Builtin, Mine: sv.Mine, Off: sv.Off, Changed: sv.Changed, Rules: []ruleData{}, Except: sv.Except}
			for _, l := range sv.Rules {
				rd := ruleData{Text: l.Text, Kind: l.Kind, KindName: routes.KindName(l.Kind), Value: l.Value, Shown: l.Shown(), Builtin: l.Builtin, Off: l.Off,
					Unlisted: !l.Builtin && l.User(sv).Unlisted()}
				if live := !l.Off && !sv.Off; live {
					rd.Ahead = !l.Builtin && covers != nil && covers(l.Text)
					if sh, ok := shadows[routes.ShadowKey(sv.ID, l.Text)]; ok {
						rd.Shadowed = sh.Word(all)
					}
					switch {
					case rd.Ahead:
						out.Ahead++
					case rd.Shadowed != "" && !l.Builtin:
						out.Hidden++
					}
				}
				d.Rules = append(d.Rules, rd)
			}
			if sv.Source != "" {
				d.Set = x.ruleSetView(sv)
			}
			g.Services = append(g.Services, d)
		}
		out.Groups = append(out.Groups, g)
	}
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// explainData says where a host goes, for 测试一个地址.
type explainData struct {
	Error     string `json:"error,omitempty"`
	Route     string `json:"route,omitempty"`
	Name      string `json:"name,omitempty"`
	Rule      string `json:"rule,omitempty"`
	Service   string `json:"service,omitempty"`
	ServiceID string `json:"serviceId,omitempty"`
	Exit      string `json:"exit,omitempty"`
	Note      string `json:"note,omitempty"`
}

func (x *bridge) explain(input string) explainData {
	host := strings.TrimSpace(input)
	if host == "" {
		return explainData{}
	}
	if t, err := routes.ParseRule(routes.KindDomain, host, ""); err == nil {
		host = t.Value
	} else if t, err := routes.ParseRule(routes.KindIP, host, ""); err == nil {
		host, _, _ = strings.Cut(t.Value, "/")
	} else {
		return explainData{Error: err.Error()}
	}
	s := x.set.Get()
	if x.importedNow() != nil {
		return explainData{Error: i18n.T("正在使用导入配置的路由，测试一个地址现在不可用；实际命中了哪条规则，可以在「活动」里看。", "An imported config's routing is in use, so an address can't be tested now; Activity shows which rule each connection matched.")}
	}
	if r, ok := tailnetOverlay(s.Tailnet).Match(host); ok {
		return explainData{Rule: r.Text, Name: "Tailscale", Note: i18n.T("走内置 Tailscale：tailnet 的地址和 MagicDNS 名字在所有模式下都走它。", "Goes by the built-in Tailscale: the tailnet's addresses and MagicDNS names go by it in every mode.")}
	}
	all := x.routes()
	id, rule, set, _, sure := routes.ExplainSets(host, routes.Active(all, s.Rules), all, x.ruleSets.Match)
	if !sure {
		for _, c := range slices.Backward(x.realConns()) {
			if (c.Host == host || c.IP == host) && c.Route != "" {
				id, rule = c.Route, c.Rule
				break
			}
		}
	}
	if id == "" {
		return explainData{Note: i18n.T("没有命中域名规则：国内网站和局域网按 GeoSite / GeoIP 走「直连」，其余归「其他流量」。", "No domain rule matches it: mainland sites and the LAN go Direct by GeoSite / GeoIP, the rest is Other.")}
	}
	r, err := x.route(id)
	if err != nil {
		return explainData{Rule: rule}
	}
	out := explainData{Route: id, Name: routes.Title(r), Rule: rule}
	sv, ok := routes.FindRule(all, rule)
	if set != "" { // the rule is the list's own: its service is the set
		sv, ok = routes.FindMatched(all, "RuleSet", routes.RuleSetProvider(set))
	}
	if ok {
		out.Service, out.ServiceID = routes.ServiceTitle(sv), sv.ID
	}
	x.mu.Lock()
	out.Exit = display(currentNode(x.proxies, r))
	x.mu.Unlock()
	return out
}

// todayShare counts what the groups carried since the last tick, and says
// each route's part of today's traffic.
func (x *bridge) todayShare(groupBytes map[string]int64) map[string]float64 {
	day := time.Now().Format("2006-01-02")
	all := x.routes()
	x.mu.Lock()
	defer x.mu.Unlock()
	days := x.trafficDays()
	if x.shareDay != day {
		// Picks up where the app left off when it restarts during the day.
		x.shareDay, x.routeBytes = day, days.day(day)
	}
	for g, v := range groupBytes {
		if d := v - x.groupSeen[g]; d > 0 {
			if id := routeOfGroup(all, g); id != "" {
				x.routeBytes[id] += d
				days.add(day, id, d)
			}
		}
	}
	x.groupSeen = groupBytes
	days.saveIfDue(time.Now())
	var total int64
	for _, v := range x.routeBytes {
		total += v
	}
	if total == 0 {
		return nil
	}
	out := make(map[string]float64, len(x.routeBytes))
	for id, v := range x.routeBytes {
		out[id] = float64(v) / float64(total)
	}
	return out
}

// routeOfGroup maps a group the core names to the route it belongs to.
func routeOfGroup(all []routes.Route, group string) string {
	for _, r := range all {
		if (!r.Direct || r.OwnExit()) && (group == routes.Group(r) || routes.Internal(r, group)) {
			return r.ID
		}
	}
	if group == "DIRECT" {
		for _, r := range all {
			if r.Direct && !r.Reject && r.ID != routes.Default && !r.OwnExit() {
				return r.ID
			}
		}
	}
	return ""
}

// ruleHome is where in the route groups the rule a connection matched lives.
type ruleHome struct {
	Group   string `json:"group,omitempty"`
	Service string `json:"service,omitempty"`
}

func (x *bridge) ruleHome(ruleType, payload string) ruleHome {
	sv, ok := routes.FindMatched(x.routes(), ruleType, payload)
	if !ok {
		return ruleHome{}
	}
	return ruleHome{sv.Group, sv.ID}
}

// parsedRule is what a rule being typed would become.
type parsedRule struct {
	Error    string `json:"error,omitempty"`
	KindName string `json:"kindName,omitempty"`
	Value    string `json:"value,omitempty"`
}

func parseRule(kind, text string) parsedRule {
	r, err := routes.ParseRule(kind, text, "")
	if err != nil {
		return parsedRule{Error: err.Error()}
	}
	return parsedRule{KindName: routes.KindName(r.Kind), Value: r.Value}
}

// ---- what the pages change ----

func (x *bridge) routeOp(ctx context.Context, a callArgs) error {
	if err := x.rulesEditable(); err != nil {
		return err
	}
	switch a.Op {
	case "setStrategy":
		x.mu.Lock()
		r, err := x.routeLocked(a.ID)
		cur := currentNode(x.proxies, r)
		x.mu.Unlock()
		if err != nil {
			return err
		}
		return x.change(ctx, func(s *settings.Settings) { s.Routing.SetStrategy(a.ID, a.Mode, cur) })
	case "setScope":
		return x.change(ctx, func(s *settings.Settings) {
			s.Routing.ChangePolicy(a.ID, func(q *routes.Policy) {
				q.Regions, q.Providers = a.Regions, a.Providers
				if a.Except != nil {
					q.ExceptProviders = *a.Except
				}
				if len(q.Providers) == 0 {
					q.ExceptProviders = false
				}
			})
		})
	case "setOrder":
		return x.change(ctx, func(s *settings.Settings) {
			s.Routing.ChangePolicy(a.ID, func(q *routes.Policy) { q.Order = a.Order })
		})
	case "setDirect":
		if !choosesExit(a.ID) {
			return errors.New(i18n.T("没有这条路由", "No such route"))
		}
		if a.On && x.set.Get().Routing.Locked(a.ID) {
			return errors.New(i18n.T("已锁定出口，先关掉出口锁定", "The exit is locked; turn the lock off first"))
		}
		if err := x.change(ctx, func(s *settings.Settings) { s.Routing.ChangePolicy(a.ID, func(q *routes.Policy) { q.Direct = a.On }) }); err != nil {
			return err
		}
		x.closeRoute(ctx, a.ID)
		return nil
	case "setLocked":
		// The exit lock, as on the desktop: the group leaves by a node or
		// not at all. With the VPN off the phone has nothing to refuse it by.
		if !routes.Lockable(a.ID) || x.importedNow() != nil {
			return errors.New(i18n.T("没有这条路由", "No such route"))
		}
		if err := x.change(ctx, func(s *settings.Settings) { s.Routing.SetLocked(a.ID, a.On) }); err != nil {
			return err
		}
		x.closeRoute(ctx, a.ID)
		return nil
	case "setFixed":
		return x.change(ctx, func(s *settings.Settings) { s.Routing.PickNode(a.ID, a.Node) })
	case "newGroup":
		if msg := routes.GroupNameError(x.routes(), "", a.Name); msg != "" {
			return errors.New(msg)
		}
		return x.change(ctx, func(s *settings.Settings) { s.Routing.AddGroup(s.Rules, a.Name) })
	case "renameGroup":
		if msg := routes.GroupNameError(x.routes(), a.ID, a.Name); msg != "" {
			return errors.New(msg)
		}
		return x.change(ctx, func(s *settings.Settings) { s.Routing.RenameGroup(a.ID, a.Name) })
	case "moveGroup":
		return x.change(ctx, func(s *settings.Settings) { s.Routing.MoveGroup(a.ID, a.To) })
	case "deleteGroup":
		return x.change(ctx, func(s *settings.Settings) {
			s.Routing.DeleteGroup(s.Rules, a.ID, a.To)
			delete(s.RouteTargets, a.ID)
		})
	case "newService":
		if msg := routes.ServiceNameError(x.routes(), "", a.Name); msg != "" {
			return errors.New(msg)
		}
		return x.change(ctx, func(s *settings.Settings) { s.Routing.AddService(s.Rules, a.ID, a.Name) })
	case "renameService":
		if msg := routes.ServiceNameError(x.routes(), a.ID, a.Name); msg != "" {
			return errors.New(msg)
		}
		return x.change(ctx, func(s *settings.Settings) { s.Routing.RenameService(a.ID, a.Name) })
	case "toggleService":
		return x.change(ctx, func(s *settings.Settings) { s.Routing.ToggleService(a.ID) })
	case "toggleRule":
		return x.change(ctx, func(s *settings.Settings) { s.Routing.ToggleRule(a.ID, a.Text) })
	case "moveService":
		return x.change(ctx, func(s *settings.Settings) { s.Routing.MoveService(s.Rules, a.ID, a.To) })
	case "splitService":
		return x.change(ctx, func(s *settings.Settings) { s.Routing.SplitService(s.Rules, a.ID) })
	case "removeService":
		return x.removeService(ctx, a.ID)
	case "addRule", "moveRule":
		r, err := routes.ParseRule(a.Kind, a.Text, a.Route)
		if err != nil {
			return err
		}
		if a.Op == "addRule" { // a rule moved here is one there already
			if err := appRuleWorks(r); err != nil {
				return err
			}
		}
		all := x.routes()
		if a.Service != "" && a.Service != routes.MineID(a.Route) {
			r.Service = a.Service
		}
		if from, ok := routes.FindRule(all, r.Mihomo()); ok && a.Op == "addRule" {
			if from.ID == cmpService(a.Service, a.Route) {
				return errors.New(i18n.T("这条规则已经在这个服务里了", "That rule is already in this service"))
			}
			return &conflictError{from: routes.ServiceTitle(from)}
		}
		return x.setRule(ctx, r, false)
	case "editRule":
		return x.editRule(ctx, a.Old, a.Kind, a.Text, a.Route)
	case "removeRule":
		r, err := routes.ParseRule(a.Kind, a.Text, a.Route)
		if err != nil {
			return err
		}
		return x.setRule(ctx, r, true)
	}
	return errors.New("unknown route operation " + a.Op)
}

// choosesExit says whether group id can be sent direct from the phone, as
// on the desktop: all but 直连 and 拦截, which never use a node, and 其他流量,
// whose choice the phone has no place for yet.
func choosesExit(id string) bool { return routes.ChoosesExit(id) && id != routes.Default }

// closeRoute closes the connections on group id, so that a change of where
// it leaves shows.
func (x *bridge) closeRoute(ctx context.Context, id string) {
	c, err := x.core()
	if err != nil {
		return
	}
	snap, err := c.Connections(ctx)
	if err != nil {
		return
	}
	all := x.routes()
	for _, cn := range snap.Connections {
		if len(cn.Chains) > 0 && routeOfGroup(all, cn.Chains[len(cn.Chains)-1]) == id {
			_ = c.CloseConnection(ctx, cn.ID)
		}
	}
	x.mon.Wake()
}

func cmpService(service, route string) string {
	if service != "" {
		return service
	}
	return routes.MineID(route)
}

// conflictError says a rule already lives in another service. The page
// offers to move it, on the code before the colon.
type conflictError struct{ from string }

func (e *conflictError) Error() string { return "conflict:" + e.from }

func (x *bridge) routeLocked(id string) (routes.Route, error) {
	if i := slices.IndexFunc(x.routesNow, func(r routes.Route) bool { return r.ID == id }); i >= 0 {
		return x.routesNow[i], nil
	}
	return routes.Route{}, errors.New(i18n.T("没有这条路由", "No such route"))
}

// setRule puts r in the rules, or takes it out, and closes the connections
// it matches that go elsewhere, so that the change shows.
func (x *bridge) setRule(ctx context.Context, r routes.UserRule, removed bool) error {
	err := x.change(ctx, func(s *settings.Settings) {
		if removed {
			s.Rules = routes.Remove(s.Rules, r)
			return
		}
		s.Routing.Claim(s.Rules, r)
		s.Rules = routes.Upsert(s.Rules, r)
	})
	if err != nil {
		return err
	}
	keep := r.Route
	if removed {
		keep = ""
	}
	x.reroute(ctx, r, keep)
	return nil
}

// appRuleWorks refuses a new rule for an app where the core can't tell apps
// apart (SetAppRules): on iOS, and on Android before 10. One brought over
// from a desktop stays, and can be removed.
func appRuleWorks(r routes.UserRule) error {
	if r.Kind == routes.KindApp && !routes.AppRules {
		return errors.New(i18n.T("这个系统上内核认不出连接来自哪个应用，按应用的规则不会生效", "On this system the core can't tell which app opened a connection, so a rule for an app would never match"))
	}
	return nil
}

// editRule makes old match text by kind and go by route: in old's service
// on the same group, in route's own rules on another. A rule another
// service has, or this one already, is refused.
func (x *bridge) editRule(ctx context.Context, old routes.UserRule, kind, text, route string) error {
	next, err := routes.ParseRule(kind, text, route)
	if err != nil {
		return err
	}
	if err := appRuleWorks(next); err != nil {
		return err
	}
	if next.Mihomo() != old.Mihomo() {
		if from, ok := routes.FindRule(x.routes(), next.Mihomo()); ok {
			if from.ID == cmpService(old.Service, old.Route) && next.Route == old.Route {
				return errors.New(i18n.T("这条规则已经在这个服务里了", "That rule is already in this service"))
			}
			return &conflictError{from: routes.ServiceTitle(from)}
		}
	}
	if err := x.change(ctx, func(s *settings.Settings) { s.Rules = routes.Edit(s.Rules, old, next) }); err != nil {
		return err
	}
	x.reroute(ctx, old, "")
	x.reroute(ctx, next, next.Route)
	return nil
}

// reroute closes the open connections r matches that go elsewhere than the
// route keep: an app's by the app the core found for them, which it looks
// for only while a rule for an app is there. Those opened before the first
// such rule have no app: reload closes them (closeUnlooked).
func (x *bridge) reroute(ctx context.Context, r routes.UserRule, keep string) {
	if r.Kind == routes.KindApp {
		x.rerouteWhere(ctx, func(cn mihomo.Connection) bool { return r.MatchesApp(cn.Metadata.Process, cn.Metadata.ProcessPath) }, keep)
		return
	}
	x.rerouteBy(ctx, r.Matches, keep)
}

// findsApps says whether the config at path has the core look up which app
// opened a connection: on "always", or on "strict" with a rule that names an
// app. A rule inside a rule set is not seen; bowel writes none there.
func findsApps(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var cfg struct {
		Mode     string              `yaml:"find-process-mode"`
		Rules    []string            `yaml:"rules"`
		SubRules map[string][]string `yaml:"sub-rules"`
	}
	if yaml.Unmarshal(raw, &cfg) != nil {
		return false
	}
	switch cfg.Mode {
	case "always":
		return true
	case "strict":
	default:
		return false
	}
	all := slices.Concat(cfg.Rules, slices.Concat(slices.Collect(maps.Values(cfg.SubRules))...))
	return slices.ContainsFunc(all, func(r string) bool {
		// PROCESS-NAME, PROCESS-PATH and their -REGEX and -WILDCARD forms,
		// and UID, also inside AND, OR and NOT.
		return strings.Contains(r, "PROCESS-") || strings.HasPrefix(r, "UID,") || strings.Contains(r, "(UID,")
	})
}

// closeUnlooked closes the connections the TUN brought in that the core
// never looked an app up for: it had no rule for one when they opened. The
// core starts looking with the first such rule, and every connection opened
// before would otherwise keep its route until it closed, whatever its app.
// The core closes those that went by a node itself, as it reloads the
// nodes; this closes the rest, which went direct. Bowel's own (the probe,
// the update's listener) did not come in by the TUN and are left alone.
func (x *bridge) closeUnlooked(ctx context.Context, c *mihomo.Client) {
	snap, err := c.Connections(ctx)
	if err != nil {
		return
	}
	for _, cn := range snap.Connections {
		if cn.Metadata.Type == "Tun" && cn.Metadata.Process == "" && cn.Metadata.ProcessPath == "" {
			_ = c.CloseConnection(ctx, cn.ID)
		}
	}
	x.mon.Wake()
}

// rerouteBy closes the open connections whose host or address match and go
// elsewhere than the route keep, so that a change of rules shows at once.
func (x *bridge) rerouteBy(ctx context.Context, match func(host, ip string) bool, keep string) {
	x.rerouteWhere(ctx, func(cn mihomo.Connection) bool {
		host := cn.Metadata.Host
		if host == "" {
			host = cn.Metadata.SniffHost
		}
		return match(host, cn.Metadata.DestinationIP)
	}, keep)
}

func (x *bridge) rerouteWhere(ctx context.Context, match func(mihomo.Connection) bool, keep string) {
	c, err := x.core()
	if err != nil {
		return
	}
	snap, err := c.Connections(ctx)
	if err != nil {
		return
	}
	byGroup := map[string]string{}
	for _, g := range x.routes() {
		byGroup[routes.Group(g)] = g.ID
	}
	for _, cn := range snap.Connections {
		if !match(cn) {
			continue
		}
		group := ""
		if len(cn.Chains) > 0 {
			group = cn.Chains[len(cn.Chains)-1]
		}
		if keep == "" || byGroup[group] != keep {
			_ = c.CloseConnection(ctx, cn.ID)
		}
	}
	x.mon.Wake()
}

// setTargets changes what a route is tested against; nil puts its defaults
// back. A new main target is tried first through the route's node, because
// the automatic group would otherwise find every node dead on it.
func (x *bridge) setTargets(ctx context.Context, routeID string, list []routes.Target) error {
	if err := x.rulesEditable(); err != nil {
		return err
	}
	r, err := x.route(routeID)
	if err != nil {
		return err
	}
	if r.Direct {
		return errors.New(i18n.T("没有这条路由", "No such route"))
	}
	if list != nil {
		list = slices.Clone(list)
		for i, t := range list { // one the page only has an address for
			if t.Name == "" {
				parsed, err := routes.ParseTarget(t.URL)
				if err != nil {
					return err
				}
				list[i] = parsed
			}
		}
		if err := routes.ValidTargets(routeID, list); err != nil {
			return err
		}
		if c, _ := x.core(); c != nil && list[0].URL != x.set.Get().TargetsOf(routeID)[0].URL {
			x.mu.Lock()
			node := currentNode(x.proxies, r)
			x.mu.Unlock()
			if p := providerOf(node); p != "" {
				if _, err := c.NodeDelay(ctx, "p-"+p, node, list[0].URL, routes.Expected(list[0].URL), siteTimeout); err != nil {
					return err
				}
			}
		}
	}
	return x.change(ctx, func(s *settings.Settings) {
		if list == nil {
			delete(s.RouteTargets, routeID)
			return
		}
		if s.RouteTargets == nil {
			s.RouteTargets = map[string][]routes.Target{}
		}
		s.RouteTargets[routeID] = slices.Clone(list)
	})
}

// testSites times a route's other targets through the node it uses now.
func (x *bridge) testSites(ctx context.Context, routeID string) error {
	if _, err := x.core(); err != nil {
		return err
	}
	r, err := x.route(routeID)
	if err != nil {
		return err
	}
	im := x.importedNow()
	x.mu.Lock()
	px, prov := x.proxies, x.provNodes
	x.mu.Unlock()
	node := x.nodeOf(px, prov, r, im)
	if providerOf(node) == "" {
		return errors.New(i18n.T("这条路由现在没有节点", "This route has no node now"))
	}
	var list []speed.Target
	for _, t := range x.set.Get().TargetsOf(routeID)[1:] {
		list = append(list, speed.Target{Node: node, URL: t.URL})
	}
	x.tests.Now(ctx, list)
	x.readTests() // the results are the node's, in the core
	return nil
}
