package cli

// Route groups without the window: `bowel route new`, `move`, `strategy`,
// `order`, `scope`, `direct`, `sites`, `rename`, `remove` and `show`, and
// `bowel routing`, which says whose groups are used. Each is a call of a
// tool of the app's (app_mcp_groups.go), the ones agents call.

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/oboo/moho/internal/i18n"
)

var (
	strategies = []string{"auto", "order", "fixed"}
	everies    = []string{"6", "12", "24"}
)

// groupView is a route group as list_route_groups gives it.
type groupView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Position  int    `json:"position"`
	Builtin   bool   `json:"builtin"`
	Fixed     bool   `json:"fixed"`
	Movable   bool   `json:"movable"`
	UsesNodes bool   `json:"usesNodes"`
	Direct    bool   `json:"direct"`
	Reject    bool   `json:"reject"`
	Locked    bool   `json:"locked"`
	Node      string `json:"node"`
	NodeLabel string `json:"nodeLabel"`
	Broken    string `json:"broken"`
	Strategy  *struct {
		Mode            string            `json:"mode"`
		Regions         []string          `json:"regions"`
		Providers       []string          `json:"providers"`
		ExceptProviders bool              `json:"exceptProviders"`
		Nodes           []string          `json:"nodes"`
		By              string            `json:"by"`
		DownloadEvery   int               `json:"downloadEvery"`
		Order           []string          `json:"order"`
		Fixed           string            `json:"fixed"`
		Labels          map[string]string `json:"labels"`
		ProviderNames   map[string]string `json:"providerNames"`
	} `json:"strategy"`
	Sites []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"sites"`
	SitesChanged bool     `json:"sitesChanged"`
	Services     []string `json:"services"`
	Rules        []rule   `json:"rules"`
}

type groupsAnswer struct {
	Groups  []groupView `json:"groups"`
	Created string      `json:"created"`
	Changed *bool       `json:"changed"`
	Routing struct {
		Source string `json:"source"`
		Name   string `json:"name"`
	} `json:"routing"`
}

// group is the one group of an answer, or the one with id.
func (g groupsAnswer) group(id string) (groupView, bool) {
	for _, v := range g.Groups {
		if id == "" || v.ID == id {
			return v, true
		}
	}
	return groupView{}, false
}

func groupCommands() []*Command {
	place := []Flag{
		{Name: "before", Kind: "string", Help: words{"排在这个路由组前面", "Put it just before this group"}},
		{Name: "after", Kind: "string", Help: words{"排在这个路由组后面", "Put it just after this group"}},
		{Name: "at", Kind: "int", Help: words{"排在第几位，1 最先匹配", "Its place in the list, 1 being tried first"}},
	}
	return []*Command{
		{Name: "show", Args: i18n.T("[路由]", "[route]"), Max: 1,
			Short:    words{"查看路由组的设置：节点策略、范围、测速站点、服务和你的规则", "Show how route groups are set up: the strategy, the range, test sites, services and your rules"},
			Examples: []string{"bowel route show", "bowel route show ai", "bowel route show ai --json"}, Run: runRouteShow},
		{Name: "new", Args: i18n.T("<名字>", "<name>"), Min: 1, Max: 1, Flags: place,
			Short:    words{"新建路由组：自动选最快的节点，还没有规则，再用 rules add 把流量送过来", "Create a route group: the fastest node, no rules yet; send traffic to it with rules add"},
			Examples: []string{"bowel route new Netflix", "bowel route new Work --before ai", "bowel rules add netflix.com Netflix"}, Run: runRouteNew},
		{Name: "rename", Args: i18n.T("<路由> <名字>", "<route> <name>"), Min: 2, Max: 2,
			Short:    words{"重命名路由组（default、direct 和 reject 不能改名）", "Rename a route group (default, direct and reject keep their names)"},
			Examples: []string{`bowel route rename media "Video"`}, Run: runRouteRename},
		{Name: "remove", Aliases: []string{"rm"}, Args: i18n.T("<路由>", "<route>"), Min: 1, Max: 1,
			Flags:    []Flag{{Name: "to", Kind: "string", Help: words{"接收它的服务和规则的路由组（默认 default）", "The group that takes its services and rules (default: default)"}}},
			Short:    words{"删除路由组，它的服务和规则交给另一个", "Delete a route group; its services and rules go to another"},
			Examples: []string{"bowel route remove Netflix", "bowel route remove Netflix --to media"}, Run: runRouteRemove},
		{Name: "move", Args: i18n.T("<路由>", "<route>"), Min: 1, Max: 1, Flags: place,
			Short:    words{"调整路由组的顺序：从上到下匹配，排在前面的先接走流量", "Reorder the route groups: they are tried from the top, and the one above takes the traffic first"},
			Examples: []string{"bowel route move reject --at 1", "bowel route move Netflix --before media", "bowel route move dev --after ai"}, Run: runRouteMove},
		{Name: "strategy", Args: i18n.T("<路由> [auto|order|fixed]", "<route> [auto|order|fixed]"), Min: 1, Max: 2,
			Flags: []Flag{
				{Name: "node", Kind: "string", Help: words{"固定节点用的节点（不写 fixed 也会切到固定节点）", "The fixed node (switches to fixed by itself)"}},
				{Name: "by", Kind: "string", Choices: []string{"delay", "download"}, Help: words{"自动最快按什么选：延迟，或下载速度", "What auto chooses by: delay, or download speed"}},
				{Name: "every", Kind: "int", Choices: everies, Help: words{"按下载速度选时，每几小时测一次", "Hours between download tests, when choosing by download speed"}},
			},
			Short:    words{"设置节点策略：auto 自动最快，order 顺序备用，fixed 固定节点", "Set the node strategy: auto the fastest, order a backup order, fixed one node"},
			Examples: []string{"bowel route strategy ai auto", `bowel route strategy ai fixed --node "🇯🇵 Tokyo 02"`, "bowel route strategy media --by download --every 12", "bowel route strategy dev order"},
			Run:      runRouteStrategy},
		{Name: "order", Args: i18n.T("<路由> <节点>...", "<route> <node>..."), Min: 2, Max: -1,
			Short:    words{"设置顺序备用的节点列表，第一个优先，并切到顺序备用", "Set the backup order, first preferred, and switch to it"},
			Examples: []string{`bowel route order ai "🇯🇵 Tokyo 02" "🇺🇸 US 01"`}, Run: runRouteOrder},
		{Name: "scope", Args: i18n.T("<路由>", "<route>"), Min: 1, Max: 1,
			Flags: []Flag{
				{Name: "regions", Kind: "string", Help: words{"地区代码，如 JP,US，或写成 JSON 列表；all 为全部", "Country codes, as JP,US, or a JSON list; all for every one"}},
				{Name: "providers", Kind: "string", Help: words{"Provider 的 id 或名字，逗号分隔或写成 JSON 列表；all 为全部", "Providers' ids or names, separated by commas or as a JSON list; all for every one"}},
				{Name: "except", Kind: "bool", Help: words{"排除所选的 Provider，用其余的", "Leave those Providers out and use the rest"}},
				{Name: "only", Kind: "bool", Help: words{"只用所选的 Provider", "Use only those Providers"}},
				{Name: "nodes", Kind: "string", Help: words{"手动挑选的节点，逗号分隔或写成 JSON 列表；all 为全部", "Nodes picked by hand, separated by commas or as a JSON list; all for every one"}},
			},
			Short:    words{"设置自动最快的范围：地区、来源和手动挑选的节点同时生效", "Set the range auto chooses from: regions, Providers and nodes picked by hand, all at once"},
			Examples: []string{"bowel route scope ai --regions JP,US", "bowel route scope media --providers a1b2c3 --except", `bowel route scope ai --nodes '["🇯🇵 Tokyo 02","🇯🇵 Osaka 01"]'`, "bowel route scope ai --regions all --providers all --nodes all"},
			Run:      runRouteScope},
		{Name: "direct", Args: i18n.T("<路由> <on|off>", "<route> <on|off>"), Min: 2, Max: 2,
			Short:    words{"让路由组的流量不经过节点直接连接（节点策略保留），或改回走节点", "Send a route group's traffic out without a node (its strategy is kept), or back through one"},
			Examples: []string{"bowel route direct dev on", "bowel route direct dev off"}, Run: runRouteDirect},
		{Name: "sites", Aliases: []string{"targets"}, Args: i18n.T("<路由> [地址...]", "<route> [url...]"), Min: 1, Max: -1,
			Flags:    []Flag{{Name: "reset", Kind: "bool", Help: words{"恢复默认的测速站点", "Put the default test sites back"}}},
			Short:    words{"查看或设置测速站点：第一个是主目标，延迟和自动选择都按它", "Show or set the test sites: the first is the main one, which delays and auto go by"},
			Examples: []string{"bowel route sites ai", "bowel route sites ai https://api.openai.com/v1/models claude.ai", "bowel route sites ai --reset"}, Run: runRouteSites},
	}
}

func routingCommand() *Command {
	return &Command{Name: "routing", Args: i18n.T("[bowel|<Provider>]", "[bowel|<provider>]"), Max: 1,
		Short:    words{"查看或切换使用谁的路由：bowel 自己的路由组和规则，或某个 Provider 配置自带的", "Show or choose whose routing is used: bowel's own route groups and rules, or those a Provider's config has"},
		Examples: []string{"bowel routing", "bowel routing a1b2c3", "bowel routing bowel"}, Run: runRouting}
}

// listArg reads a list as typed: a JSON list, or names separated by
// commas; all, or nothing, is none, which is every one.
func listArg(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	out := []string{}
	switch {
	case s == "" || strings.EqualFold(s, "all"):
		return out, nil
	case strings.HasPrefix(s, "["):
		if err := json.Unmarshal([]byte(s), &out); err != nil {
			return nil, fmt.Errorf(i18n.T("不是 JSON 列表: %s", "Not a JSON list: %s"), s)
		}
		return out, nil
	}
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out, nil
}

// placeArgs are where the place flags say a group goes; none when they
// say nothing, an error when they say more than one place.
func (c *Ctx) placeArgs(args map[string]any) (int, error) {
	n := 0
	for _, f := range []string{"before", "after"} {
		if v := c.flag(f); v != "" {
			args[f] = v
			n++
		}
	}
	if _, ok := c.flags["at"]; ok {
		at := c.intFlag("at", 0)
		if at < 1 {
			return 0, &usageError{msg: i18n.T("--at 从 1 开始", "--at counts from 1"), path: c.path}
		}
		args["position"] = at
		n++
	}
	if n > 1 {
		return 0, &usageError{msg: i18n.T("--before、--after 和 --at 只能用一个", "Use only one of --before, --after and --at"), path: c.path}
	}
	return n, nil
}

func strategyName(mode string) string {
	switch mode {
	case "order":
		return i18n.T("顺序备用", "Backup order")
	case "fixed":
		return i18n.T("固定节点", "Fixed node")
	}
	return i18n.T("自动最快", "Fastest")
}

func joined(l []string) string { return strings.Join(l, i18n.T("、", ", ")) }

// strategyText is how group g chooses its way out, in a few words.
func strategyText(g groupView) cell {
	switch {
	case g.Reject:
		return cell{i18n.T("拒绝", "Reject"), gray}
	case !g.UsesNodes:
		return cell{i18n.T("直连", "Direct"), gray}
	case g.Strategy == nil:
		return t("—")
	}
	q := g.Strategy
	label := func(n string) string {
		if l := q.Labels[n]; l != "" {
			return l
		}
		return n
	}
	var text string
	switch q.Mode {
	case "fixed":
		text = strategyName(q.Mode) + ": " + label(q.Fixed)
	case "order":
		var names []string
		for _, n := range q.Order {
			names = append(names, label(n))
		}
		text = strategyName(q.Mode) + ": " + strings.Join(names, " → ")
	default:
		text = strategyName(q.Mode)
		if s := scopeText(g); s != "" {
			text += " · " + s
		}
		if q.By == "download" {
			text += " · " + fmt.Sprintf(i18n.T("按下载速度，每 %d 小时", "by download speed, every %d h"), q.DownloadEvery)
		}
	}
	if g.Direct {
		return cell{i18n.T("直接连接", "Direct") + " (" + i18n.T("保留：", "kept: ") + text + ")", gray}
	}
	return t(text)
}

// scopeText is the range of auto, "" when it is every node.
func scopeText(g groupView) string {
	q := g.Strategy
	var parts []string
	if len(q.Regions) > 0 {
		parts = append(parts, joined(q.Regions))
	}
	if len(q.Providers) > 0 {
		var names []string
		for _, id := range q.Providers {
			names = append(names, cmpOr(q.ProviderNames[id], id))
		}
		w := joined(names)
		if q.ExceptProviders {
			w = i18n.T("不用 ", "not ") + w
		}
		parts = append(parts, w)
	}
	if n := len(q.Nodes); n > 0 {
		parts = append(parts, fmt.Sprintf(i18n.T("%d 个节点", "%d nodes"), n))
	}
	return strings.Join(parts, " · ")
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func runRouteShow(c *Ctx) error {
	args := map[string]any{}
	if len(c.args) > 0 {
		args["route"] = c.args[0]
	}
	var out groupsAnswer
	raw, err := c.get("list_route_groups", args, &out)
	if err != nil {
		return err
	}
	if c.json {
		c.printJSON(raw, false)
		return nil
	}
	if len(out.Groups) == 0 { // an imported config's routing
		fmt.Fprintf(c.out, i18n.T("正在使用 %s 自带的路由组，bowel 自己的路由组先保留着。用 `bowel routes` 查看它们，`bowel routing bowel` 改回来。", "Using %s's own route groups; bowel's are kept. See them with `bowel routes`; go back with `bowel routing bowel`.")+"\n", out.Routing.Name)
		return nil
	}
	if len(c.args) == 0 {
		var rows [][]cell
		for _, g := range out.Groups {
			main := ""
			if len(g.Sites) > 0 {
				main = g.Sites[0].Name
			}
			rows = append(rows, []cell{t(fmt.Sprint(g.Position)), t(g.ID), t(g.Name), strategyText(g), {main, gray}})
		}
		c.table([]string{"#", "ID", i18n.T("路由组", "Group"), i18n.T("节点策略", "Strategy"), i18n.T("主目标", "Main site")}, rows)
		fmt.Fprintln(c.out, c.paint(gray, i18n.T("从上到下匹配：第一个有规则认出连接的路由组决定它走哪里。", "Tried from the top: the first group with a rule that knows a connection decides where it goes.")))
		return nil
	}
	g, _ := out.group("")
	c.printGroup(g)
	return nil
}

// printGroup shows one group whole.
func (c *Ctx) printGroup(g groupView) {
	kind := i18n.T("自建", "yours")
	if g.Builtin {
		kind = i18n.T("内置", "built in")
	}
	fmt.Fprintf(c.out, "%s  %s\n", c.paint(bold, g.Name), c.paint(gray, fmt.Sprintf("%s · #%d · %s", g.ID, g.Position, kind)))
	var rows [][]string
	add := func(k, v string) { rows = append(rows, []string{k, v}) }
	st := strategyText(g)
	add(i18n.T("节点策略", "Strategy"), c.paint(st.color, st.text))
	if q := g.Strategy; q != nil && g.UsesNodes {
		label := func(n string) string { return cmpOr(q.Labels[n], n) }
		var order []string
		for _, n := range q.Order {
			order = append(order, label(n))
		}
		if q.Mode != "auto" {
			if s := scopeText(g); s != "" {
				add(i18n.T("自动的范围", "Auto's range"), c.paint(gray, s))
			}
		}
		if q.Mode != "order" && len(order) > 0 {
			add(i18n.T("备用顺序", "Backup order"), c.paint(gray, strings.Join(order, " → ")))
		}
		if q.Mode != "fixed" && q.Fixed != "" {
			add(i18n.T("固定节点", "Fixed node"), c.paint(gray, label(q.Fixed)))
		}
	}
	if g.UsesNodes {
		now := cmpOr(g.NodeLabel, "—")
		if g.Locked {
			now += i18n.T(" · 出口锁定", " · exit locked")
		}
		if g.Broken != "" {
			now += " · " + c.paint(red, i18n.T("现在连不上", "can't connect now"))
		}
		add(i18n.T("现在", "Now"), now)
	}
	for i, s := range g.Sites {
		k := ""
		if i == 0 {
			k = i18n.T("测速站点", "Test sites")
		}
		v := s.Name + "  " + c.paint(gray, s.URL)
		if i == 0 {
			v += "  " + c.paint(gray, i18n.T("（主目标）", "(main)"))
		}
		add(k, v)
	}
	if len(g.Services) > 0 {
		add(i18n.T("包含的服务", "Services"), strings.Join(g.Services, " · "))
	}
	for i, r := range g.Rules {
		k := ""
		if i == 0 {
			k = i18n.T("你的规则", "Your rules")
		}
		add(k, r.Kind+" "+r.Value+"  "+c.paint(gray, r.Matches))
	}
	width := 0
	for _, r := range rows {
		width = max(width, dispWidth(r[0]))
	}
	for _, r := range rows {
		fmt.Fprintln(c.out, "  "+c.paint(gray, pad(r[0], width))+"  "+r[1])
	}
}

// changed reports a change to group id: in words, or with --json as the
// tool's answer.
func (c *Ctx) changed(raw json.RawMessage, id, msg string) {
	var out groupsAnswer
	_ = json.Unmarshal(raw, &out)
	if out.Changed != nil && !*out.Changed && !c.json {
		fmt.Fprintln(c.out, i18n.T("本来就是这样，没有改动", "Already so; nothing changed"))
		return
	}
	if g, ok := out.group(id); ok && msg == "" {
		st := strategyText(g)
		msg = g.Name + ": " + st.text
	}
	c.done(raw, msg)
}

func runRouteNew(c *Ctx) error {
	args := map[string]any{"name": c.args[0]}
	if _, err := c.placeArgs(args); err != nil {
		return err
	}
	var out groupsAnswer
	raw, err := c.get("create_route_group", args, &out)
	if err != nil {
		return err
	}
	c.done(raw, fmt.Sprintf(i18n.T("已新建路由组 %s（id %s）。用 `bowel rules add <地址> %s` 把流量送过来。", "Created route group %s (id %s). Send traffic to it with `bowel rules add <target> %s`."), c.args[0], out.Created, out.Created))
	return nil
}

func runRouteRename(c *Ctx) error {
	raw, err := c.call("rename_route_group", map[string]any{"route": c.args[0], "name": c.args[1]})
	if err != nil {
		return err
	}
	c.changed(raw, "", fmt.Sprintf(i18n.T("已改名为 %s", "Renamed to %s"), c.args[1]))
	return nil
}

func runRouteRemove(c *Ctx) error {
	args := map[string]any{"route": c.args[0]}
	if to := c.flag("to"); to != "" {
		args["to"] = to
	}
	var out struct {
		MovedTo string `json:"movedTo"`
	}
	raw, err := c.get("delete_route_group", args, &out)
	if err != nil {
		return err
	}
	c.done(raw, fmt.Sprintf(i18n.T("已删除 %s，它的服务和规则交给了 %s", "Deleted %s; its services and rules went to %s"), c.args[0], out.MovedTo))
	return nil
}

func runRouteMove(c *Ctx) error {
	args := map[string]any{"route": c.args[0]}
	n, err := c.placeArgs(args)
	if err != nil {
		return err
	}
	if n == 0 {
		return &usageError{msg: i18n.T("要用 --before、--after 或 --at 说排在哪里", "Say where with --before, --after or --at"), path: c.path}
	}
	var out groupsAnswer
	raw, err := c.get("move_route_group", args, &out)
	if err != nil {
		return err
	}
	var names []string
	for _, g := range out.Groups {
		names = append(names, g.Name)
	}
	c.changed(raw, "", i18n.T("顺序：", "Order: ")+strings.Join(names, " → "))
	return nil
}

// setStrategy calls set_route_strategy and says what the group does now.
func (c *Ctx) setStrategy(args map[string]any) error {
	var out groupsAnswer
	raw, err := c.get("set_route_strategy", args, &out)
	if err != nil {
		return err
	}
	c.changed(raw, "", "")
	return nil
}

func runRouteStrategy(c *Ctx) error {
	args := map[string]any{"route": c.args[0]}
	if m := c.arg(1); m != "" {
		if !slices.Contains(strategies, m) {
			return usagef(c.path, i18n.T("节点策略只能是 %s，不是 %s", "The strategy is one of %s, not %s"), strings.Join(strategies, ", "), m)
		}
		args["mode"] = m
	}
	if n := c.flag("node"); n != "" {
		args["node"] = n
	}
	if b := c.flag("by"); b != "" {
		args["by"] = b
	}
	if _, ok := c.flags["every"]; ok {
		args["downloadEvery"] = c.intFlag("every", 0)
	}
	if len(args) == 1 {
		return &usageError{msg: i18n.T("要给出节点策略，或 --node、--by、--every", "Give a strategy, or --node, --by or --every"), path: c.path}
	}
	return c.setStrategy(args)
}

func runRouteOrder(c *Ctx) error {
	return c.setStrategy(map[string]any{"route": c.args[0], "mode": "order", "order": c.args[1:]})
}

func runRouteScope(c *Ctx) error {
	args := map[string]any{"route": c.args[0]}
	for _, f := range []string{"regions", "providers", "nodes"} {
		if v, ok := c.flags[f]; ok {
			l, err := listArg(v)
			if err != nil {
				return usagef(c.path, "%s", err.Error())
			}
			args[f] = l
		}
	}
	switch {
	case c.has("except") && c.has("only"):
		return &usageError{msg: i18n.T("--except 和 --only 只能用一个", "Use only one of --except and --only"), path: c.path}
	case c.has("except"):
		args["exceptProviders"] = true
	case c.has("only"):
		args["exceptProviders"] = false
	}
	if len(args) == 1 {
		return &usageError{msg: i18n.T("要给出 --regions、--providers、--nodes、--except 或 --only", "Give --regions, --providers, --nodes, --except or --only"), path: c.path}
	}
	return c.setStrategy(args)
}

func runRouteDirect(c *Ctx) error {
	on := c.args[1]
	if on != "on" && on != "off" {
		return usagef(c.path, i18n.T("只能是 on 或 off，不是 %s", "It is on or off, not %s"), on)
	}
	return c.setStrategy(map[string]any{"route": c.args[0], "direct": on == "on"})
}

func runRouteSites(c *Ctx) error {
	sites, reset := c.args[1:], c.has("reset")
	if reset && len(sites) > 0 {
		return &usageError{msg: i18n.T("--reset 不和地址一起用", "--reset takes no URLs"), path: c.path}
	}
	if !reset && len(sites) == 0 {
		var out groupsAnswer
		raw, err := c.get("list_route_groups", map[string]any{"route": c.args[0]}, &out)
		if err != nil {
			return err
		}
		g, ok := out.group("")
		if c.json || !ok {
			c.printJSON(raw, false)
			return nil
		}
		if len(g.Sites) == 0 {
			fmt.Fprintln(c.out, i18n.T("这个路由组不使用节点，没有测速站点。", "This group uses no node, so it has no test sites."))
			return nil
		}
		var rows [][]cell
		for i, s := range g.Sites {
			main := ""
			if i == 0 {
				main = i18n.T("主目标", "main")
			}
			rows = append(rows, []cell{t(s.Name), t(s.URL), {main, gray}})
		}
		c.table([]string{i18n.T("站点", "Site"), i18n.T("地址", "URL"), ""}, rows)
		if !g.SitesChanged {
			fmt.Fprintln(c.out, c.paint(gray, i18n.T("这是默认的测速站点。", "These are the default test sites.")))
		}
		return nil
	}
	args := map[string]any{"route": c.args[0]}
	if reset {
		args["reset"] = true
	} else {
		args["sites"] = sites
	}
	var out groupsAnswer
	raw, err := c.get("set_route_sites", args, &out)
	if err != nil {
		return err
	}
	msg := ""
	if g, ok := out.group(""); ok && len(g.Sites) > 0 {
		var names []string
		for _, s := range g.Sites {
			names = append(names, s.Name)
		}
		msg = fmt.Sprintf(i18n.T("%s 的测速站点：%s（第一个是主目标）", "%s's test sites: %s (the first is the main one)"), g.Name, joined(names))
	}
	c.changed(raw, "", msg)
	return nil
}

func runRouting(c *Ctx) error {
	if len(c.args) == 0 {
		var out struct {
			Imported *struct {
				Provider string `json:"provider"`
			} `json:"imported"`
		}
		if _, err := c.get("list_routes", nil, &out); err != nil {
			return err
		}
		v := map[string]string{"source": "bowel", "name": "bowel"}
		if out.Imported != nil {
			v = map[string]string{"source": "provider", "name": out.Imported.Provider}
		}
		if c.json {
			b, _ := json.Marshal(map[string]any{"routing": v})
			c.printJSON(b, false)
			return nil
		}
		if out.Imported != nil {
			fmt.Fprintf(c.out, i18n.T("正在使用 %s 配置自带的路由组和规则", "Using the route groups and rules of %s's config")+"\n", out.Imported.Provider)
		} else {
			fmt.Fprintln(c.out, i18n.T("正在使用 bowel 自己的路由组和规则", "Using bowel's own route groups and rules"))
		}
		return nil
	}
	var out groupsAnswer
	raw, err := c.get("set_routing_source", map[string]any{"source": c.args[0]}, &out)
	if err != nil {
		return err
	}
	if out.Changed != nil && !*out.Changed && !c.json {
		fmt.Fprintln(c.out, i18n.T("本来就是这样，没有改动", "Already so; nothing changed"))
		return nil
	}
	c.done(raw, fmt.Sprintf(i18n.T("现在使用 %s 的路由", "Now using %s's routing"), out.Routing.Name))
	return nil
}
