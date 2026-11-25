package bridge

// A provider's own routing, as the desktop has it in app_imported.go: when
// the user chooses it, the proxy-groups and rules of a complete Clash config
// replace the route groups. The config is read by internal/core; here is
// what the phone's pages see of it. Its groups that rules point at are
// routes like any other (core.Imported.Routes), so the board, the map and
// 活动 need no second way of working; what would edit bowel's route groups is
// refused until the user goes back to them.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/mihomo"
	"github.com/oboo/moho/internal/provider"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

// importedCache is the routing in use, read again only when what it was
// read from has changed. Guarded by impMu.
type importedCache struct {
	loaded bool
	key    string // what it was read from; "" when none is in use
	now    *core.Imported
	err    string // why the routing chosen could not be read
}

// routingInfo is what a provider's file brings, as of key.
type routingInfo struct {
	key     string
	routing providerRouting
}

// providerRouting is the routing a provider's file brings, for its row.
type providerRouting struct {
	Groups int  `json:"groups,omitempty"`
	Rules  int  `json:"rules,omitempty"`
	InUse  bool `json:"inUse,omitempty"`
	// Remote is how many subscriptions of its own (proxy-providers) the
	// config's groups take nodes from: the core downloads those, and only
	// while the routing is in use.
	Remote int `json:"remote,omitempty"`
	// Problem is why it can't be used, "" when it can.
	Problem string `json:"problem,omitempty"`
	// NeedsFile is the proxy-provider of the config that is a local file
	// bowel has no local file Provider for.
	NeedsFile string `json:"needsFile,omitempty"`
}

// importedData is the routing in use, as the 路由组 page shows it.
type importedData struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
	Rules    int    `json:"rules"`
	// The user's rules and rule sets, which wait for bowel's routing.
	PausedRules int `json:"pausedRules"`
	PausedSets  int `json:"pausedSets"`
	// Unused are the config's settings that act on traffic and are not
	// used here, as the config names them (core.Imported.Unused).
	Unused []string        `json:"unused"`
	Groups []importedGroup `json:"groups,omitempty"` // in the 路由组 page's data, not the view's
}

type importedGroup struct {
	Name  string `json:"name"`
	Kind  string `json:"kind"`            // the core's word: select, url-test, …
	Route string `json:"route,omitempty"` // the route it is, "" if only other groups use it
	Rules int    `json:"rules"`
	Now   string `json:"now,omitempty"`
	// Exit is the node at the end of the chain, as the node list names it.
	Exit       string           `json:"exit,omitempty"`
	Selectable bool             `json:"selectable"`
	Members    []importedMember `json:"members"`
}

type importedMember struct {
	Name   string `json:"name"` // the core's
	Node   string `json:"node,omitempty"`
	Group  bool   `json:"group,omitempty"`
	Delay  int    `json:"delay"`
	Tested bool   `json:"tested"`
}

// importedWaiting is what a config in use names for the core to download
// that the core has nothing of yet: rule sets without a rule, and node
// providers without a node.
type importedWaiting struct{ sets, subs []string }

// localFiles gives, for a `type: file` proxy-provider of config's, the files
// bowel keeps under that name: those of the enabled local file Providers
// called the same. The path the config names is never read.
func (x *bridge) localFiles(config string) func(name string) []string {
	return func(name string) []string {
		var paths []string
		for _, p := range x.store.List() {
			if p.Kind == provider.File && p.Enabled && p.ID != config && p.Name == name {
				paths = append(paths, x.store.Path(p.ID))
			}
		}
		return paths
	}
}

// localSig changes when what localFiles has to offer does, or what of it
// the user removed.
func (x *bridge) localSig() string {
	var b strings.Builder
	for _, p := range x.store.List() {
		if p.Kind == provider.File && p.Enabled {
			fmt.Fprintf(&b, "%s\x00%s\x00%d\x00%s\x01", p.ID, p.Name, p.UpdatedAt.UnixNano(), strings.Join(p.Hidden, "\x00"))
		}
	}
	return b.String()
}

// importedNow is the routing in use from a provider's config, nil with
// bowel's own: none chosen, its provider disabled, or its file unreadable.
func (x *bridge) importedNow() *core.Imported {
	s := x.set.Get()
	key := ""
	var p provider.Provider
	if s.ImportedRouting != "" {
		if q, ok := x.store.Get(s.ImportedRouting); ok && q.Enabled {
			p = q
			key = fmt.Sprintf("%s\x00%d\x00%s\x00%s", p.ID, p.UpdatedAt.UnixNano(), strings.Join(p.Hidden, "\x00"), x.localSig())
		}
	}
	x.impMu.Lock()
	defer x.impMu.Unlock()
	if x.imp.loaded && x.imp.key == key {
		return x.imp.now
	}
	x.imp = importedCache{loaded: true, key: key}
	if key == "" {
		return nil
	}
	data, err := x.store.Content(p.ID)
	var im *core.Imported
	if err == nil {
		im, err = core.ParseImported(data, x.localFiles(p.ID))
	}
	if err == nil && im == nil {
		err = errors.New(i18n.T("这份配置里没有路由组", "This config has no route groups"))
	}
	if err != nil {
		x.imp.err = err.Error()
		return nil
	}
	im = im.Without(p.Hidden) // the nodes the user removed are gone from its routing too
	for _, f := range x.store.List() {
		if f.Kind == provider.File && f.Enabled && f.ID != p.ID {
			im = im.WithoutInFile(x.store.Path(f.ID), f.Hidden)
		}
	}
	x.imp.now = im
	return im
}

// importedErr is why the routing chosen could not be read, "" if it could.
func (x *bridge) importedErr() string {
	x.importedNow()
	x.impMu.Lock()
	defer x.impMu.Unlock()
	return x.imp.err
}

// providerRouting says what p's file brings. Asked for each view, it reads
// a file only when that has changed.
func (x *bridge) providerRouting(p provider.Provider, inUse string) *providerRouting {
	key := fmt.Sprintf("%d\x00%s", p.UpdatedAt.UnixNano(), x.localSig())
	x.impMu.Lock()
	info, ok := x.impInfo[p.ID]
	x.impMu.Unlock()
	if !ok || info.key != key {
		info = routingInfo{key: key}
		if data, err := x.store.Content(p.ID); err == nil && core.HasGroups(data) {
			if im, err := core.ParseImported(data, x.localFiles(p.ID)); err != nil {
				info.routing.Problem = err.Error()
				var missing *core.MissingFile
				if errors.As(err, &missing) {
					info.routing.NeedsFile = missing.Name
				}
			} else if im != nil {
				info.routing.Groups, info.routing.Rules, info.routing.Remote = len(im.Groups), len(im.Rules), len(im.ProviderNames())
			}
		}
		x.impMu.Lock()
		if x.impInfo == nil {
			x.impInfo = map[string]routingInfo{}
		}
		x.impInfo[p.ID] = info
		x.impMu.Unlock()
	}
	if info.routing == (providerRouting{}) {
		return nil
	}
	r := info.routing
	r.InUse = p.Enabled && inUse == p.ID && r.Groups > 0
	return &r
}

// vetUpdate keeps an update of the provider in use from breaking the
// routing: what comes down must still have groups the config can be built
// from, or the last good file stays and the update counts as failed. Never
// the template instead.
func (x *bridge) vetUpdate(id string, data []byte) error {
	s := x.set.Get()
	if s.ImportedRouting != id {
		return nil
	}
	im, err := core.ParseImported(data, x.localFiles(id))
	if err == nil && im == nil {
		err = errors.New(i18n.T("这次下载的内容里没有路由组", "What was downloaded has no route groups"))
	}
	if err == nil {
		_, err = x.config(s, im)
	}
	if err != nil {
		return fmt.Errorf(i18n.T("这次更新没有应用，继续用上一份：%w", "This update wasn't applied; the last one stays: %w"), err)
	}
	return nil
}

// setImportedRouting uses provider id's own routing; "" goes back to
// bowel's. The user's rules and route groups are not touched either way.
// Connections already open would stay on the old routing: they are closed
// so that the change shows at once.
func (x *bridge) setImportedRouting(ctx context.Context, id string) error {
	if id != "" {
		p, ok := x.store.Get(id)
		if !ok {
			return fmt.Errorf(i18n.T("没有这个 Provider: %s", "No such Provider: %s"), id)
		}
		if !p.Enabled {
			return errors.New(i18n.T("这个 Provider 已停用", "This Provider is disabled"))
		}
		data, err := x.store.Content(id)
		if err != nil {
			return err
		}
		im, err := core.ParseImported(data, x.localFiles(id))
		if err != nil {
			return err
		}
		if im == nil {
			return errors.New(i18n.T("这份配置里没有路由组", "This config has no route groups"))
		}
	}
	if err := x.change(ctx, func(s *settings.Settings) { s.ImportedRouting = id }); err != nil {
		return err
	}
	if c, _ := x.core(); c != nil {
		_ = c.CloseAllConnections(ctx)
		x.mon.Wake()
		go x.locateExits()
	}
	return nil
}

// selectMember points an imported group of the kind the user chooses in at
// member. The core remembers it across reloads (store-selected).
func (x *bridge) selectMember(ctx context.Context, group, member string) error {
	im := x.importedNow()
	if im == nil {
		return errors.New(i18n.T("没有这个路由组", "No such route group"))
	}
	g, ok := im.GroupOf(group)
	if !ok {
		return errors.New(i18n.T("没有这个路由组", "No such route group"))
	}
	if core.GroupKind(g) != "select" {
		return fmt.Errorf(i18n.T("「%s」的类型是 %s，由内核自己选择", "%s is of the kind %s: the core chooses by itself"), group, core.GroupKind(g))
	}
	if member == "" {
		return errors.New(i18n.T("这个路由组需要选一个成员", "This route group needs a member chosen"))
	}
	c, err := x.core()
	if err != nil {
		return err
	}
	if err := c.Select(ctx, group, member); err != nil {
		return err
	}
	_ = x.refresh(ctx)
	x.push()
	go x.locateExits()
	return nil
}

// rulesEditable refuses a change to the user's rules and route groups while
// an imported config has them paused: it could not take effect.
func (x *bridge) rulesEditable() error {
	if x.importedNow() != nil {
		return errors.New(i18n.T("正在使用导入配置的路由，你的规则已暂停。改回 bowel 路由后才能修改", "An imported config's routing is in use and your rules are paused. Go back to bowel's routing to change them"))
	}
	return nil
}

// listedNode is the name the node list has for one a group names: the
// provider's copy of a node carries its marker, the group's has none.
func (x *bridge) listedNode(name string, prov map[string][]string) string {
	if name == "" {
		return name
	}
	id := x.set.Get().ImportedRouting
	if full := name + core.NodeSep + id; slices.Contains(prov[id], full) {
		return full
	}
	return name
}

// sourceName is the name in the source's config of a node of the node list.
func (x *bridge) sourceName(node string) string {
	if id := x.set.Get().ImportedRouting; id != "" && strings.HasSuffix(node, core.NodeSep+id) {
		return strings.TrimSuffix(node, core.NodeSep+id)
	}
	return node
}

// leafOf is where a chain of groups that starts at name ends.
func leafOf(px map[string]mihomo.Proxy, name string) string {
	for i := 0; i < 16 && px[name].Now != ""; i++ {
		name = px[name].Now
	}
	return name
}

// nodeOf is the node route r is on now, as the node list names it: through
// the helper groups of bowel's routing, or the chain of an imported config's
// groups. "" if none.
func (x *bridge) nodeOf(px map[string]mihomo.Proxy, prov map[string][]string, r routes.Route, im *core.Imported) string {
	if im == nil {
		return currentNode(px, r)
	}
	if px[routes.Group(r)].Now == "" {
		return ""
	}
	return x.listedNode(leafOf(px, routes.Group(r)), prov)
}

// imported is im as it stands in the core, for the 路由组 page.
func (x *bridge) imported(s settings.Settings, im *core.Imported, all []routes.Route, px map[string]mihomo.Proxy, prov map[string][]string) *importedData {
	if im == nil {
		return nil
	}
	name := s.ImportedRouting
	if p, ok := x.store.Get(name); ok {
		name = p.Name
	}
	v := &importedData{Provider: s.ImportedRouting, Name: name, Rules: len(im.Rules), PausedRules: len(s.Rules), PausedSets: len(x.ruleSetRefs(s)),
		Unused: nonNil(im.Unused), Groups: []importedGroup{}}
	byName := map[string]string{}
	for _, r := range all {
		byName[r.Name] = r.ID
	}
	groups := map[string]bool{}
	for _, g := range im.Groups {
		groups[core.GroupName(g)] = true
	}
	for _, g := range im.Groups {
		gname := core.GroupName(g)
		p := px[gname]
		gv := importedGroup{Name: gname, Kind: core.GroupKind(g), Route: byName[gname], Rules: im.RulesTo(gname), Now: p.Now, Selectable: core.GroupKind(g) == "select", Members: []importedMember{}}
		if leaf := leafOf(px, gname); p.Now != "" && leaf != "DIRECT" && leaf != "REJECT" && leaf != "REJECT-DROP" {
			gv.Exit = x.listedNode(leaf, prov)
		}
		for _, m := range p.All {
			mv := importedMember{Name: m, Group: groups[m]}
			if !mv.Group {
				mv.Node = x.listedNode(m, prov)
			}
			if h := px[m].Results(); len(h) > 0 {
				mv.Tested, mv.Delay = true, h[len(h)-1].Delay
			}
			gv.Members = append(gv.Members, mv)
		}
		v.Groups = append(v.Groups, gv)
	}
	return v
}

// noteWaiting asks the core which of im's rule sets and node providers it
// has nothing of yet.
func (x *bridge) noteWaiting(ctx context.Context, c *mihomo.Client, im *core.Imported, pv map[string]mihomo.ProxyProvider) {
	var w importedWaiting
	if im != nil {
		for _, name := range im.ProviderNames() {
			if p, ok := pv[name]; ok && len(p.Proxies) == 0 {
				w.subs = append(w.subs, name)
			}
		}
		if len(im.RuleProviders) > 0 {
			if rp, err := c.RuleProviders(ctx); err == nil {
				for name := range im.RuleProviders {
					if p, ok := rp[name]; ok && p.RuleCount == 0 {
						w.sets = append(w.sets, name)
					}
				}
				slices.Sort(w.sets)
			}
		}
	}
	x.mu.Lock()
	x.impWaiting = w
	x.mu.Unlock()
}

// importedIssue is what the 代理 page says about the routing chosen: a
// config that can't be read, an update that was not applied, or what the
// core has not downloaded yet. nil when there is nothing to say.
func (x *bridge) importedIssue(s settings.Settings, im *core.Imported, running bool) *issueData {
	if why := x.importedErr(); why != "" {
		return &issueData{Level: "warn", Text: i18n.T("读不了这份配置的路由，现在用的是 bowel 路由", "This config's routing can't be read; bowel's routing is in use"), Detail: why, Action: "providers", Label: i18n.T("查看", "View")}
	}
	if im == nil {
		return nil
	}
	if p, ok := x.store.Get(s.ImportedRouting); ok && p.Error != "" {
		return &issueData{Level: "warn", Text: fmt.Sprintf(i18n.T("%s 这次更新没有应用，路由没有变化", "%s: the last update wasn't applied; the routing is unchanged"), p.Name), Detail: p.Error, Action: "providers", Label: i18n.T("查看", "View")}
	}
	if !running {
		return nil
	}
	x.mu.Lock()
	w := x.impWaiting
	x.mu.Unlock()
	if len(w.sets)+len(w.subs) == 0 {
		return nil
	}
	hosts := im.RemoteHosts(slices.Concat(w.subs, w.sets))
	detail := fmt.Sprintf(i18n.T("内核直接从 %s 下载它们，现在还没有下到。", "The core downloads them straight from %s and has not got them yet."), strings.Join(hosts, i18n.T("、", ", ")))
	if len(w.subs) > 0 {
		detail += " " + fmt.Sprintf(i18n.T("没有节点的订阅：%s。", "Subscriptions without nodes: %s."), strings.Join(w.subs, i18n.T("、", ", ")))
	}
	if len(w.sets) > 0 {
		detail += " " + fmt.Sprintf(i18n.T("空的规则集：%s。用到它们的规则在下载好之前不会匹配。", "Empty rule sets: %s. The rules that use them match nothing until they arrive."), strings.Join(w.sets, i18n.T("、", ", ")))
	}
	detail += " " + i18n.T("如果这些地址在当前网络下打不开，可以改回 bowel 路由，或把配置里的地址换成能直连的镜像。", "If these addresses can't be opened from this network, go back to bowel's routing, or point the config at a mirror that can be reached directly.")
	text := fmt.Sprintf(i18n.T("这份配置有 %d 个规则集还没下载下来", "%d of this config's rule sets haven't been downloaded"), len(w.sets))
	if len(w.subs) > 0 {
		text = fmt.Sprintf(i18n.T("这份配置的订阅「%s」还没下载到节点", "This config's subscription %s has no nodes yet"), w.subs[0])
	}
	return &issueData{Level: "warn", Text: text, Detail: detail, Action: "providers", Label: i18n.T("查看", "View")}
}
