// Package routes is Moho's routing template: kinds of traffic and the rules
// that recognise them. Providers only supply nodes; every provider's nodes
// can serve every route (decision D1 of docs/product-ia.md).
package routes

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/oboo/moho/internal/i18n"
)

// Route is a kind of traffic and where it may go.
type Route struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Color   string `json:"color"`   // CSS custom property of its color
	Summary string `json:"summary"` // what it matches, for people
	// Rules are mihomo rules without their target, e.g. "DOMAIN-SUFFIX,openai.com".
	Rules []string `json:"-"`
	// Direct routes never use a node.
	Direct bool `json:"direct"`
}

// Default is the route that takes what no rule matched.
const Default = "default"

// Group is the name of a route's mihomo group; Auto that of its automatic
// (fastest + failover) group.
func Group(r Route) string { return r.Name }
func Auto(r Route) string  { return r.Name + "·自动" }

// Title is a route's name as the interface shows it. Name stays the
// mihomo group's, in Chinese, so that configs and choices keep across a
// change of language.
func Title(r Route) string {
	if en, ok := titleEn[r.ID]; ok {
		return i18n.T(r.Name, en)
	}
	return r.Name
}

// Describe is a route's Summary as the interface shows it.
func Describe(r Route) string {
	if en, ok := summaryEn[r.ID]; ok {
		return i18n.T(r.Summary, en)
	}
	return r.Summary
}

var titleEn = map[string]string{Default: "Default", "ai": "AI", "media": "Streaming", "dev": "Dev", "direct": "Direct"}

var summaryEn = map[string]string{
	Default:  "Traffic no other rule matched",
	"ai":     "OpenAI · Claude · Gemini · Perplexity and more",
	"media":  "YouTube · Netflix · Spotify · Disney+ and more",
	"dev":    "GitHub · npm · Docker · PyPI · Go and more",
	"direct": "Mainland China sites · LAN",
}

// Template returns the routes in the order their rules apply; the default
// route comes first for display, its rule (MATCH) last.
func Template() []Route {
	return []Route{
		{ID: Default, Name: "默认", Color: "--r-default", Summary: "没命中其他规则的流量"},
		{ID: "ai", Name: "AI 服务", Color: "--r-ai", Summary: "OpenAI · Claude · Gemini · Perplexity 等", Rules: suffixes(
			"openai.com", "chatgpt.com", "oaistatic.com", "oaiusercontent.com", "sora.com",
			"anthropic.com", "claude.ai", "claude.com", "claudeusercontent.com",
			"gemini.google.com", "generativelanguage.googleapis.com", "aistudio.google.com", "notebooklm.google.com", "bard.google.com",
			"perplexity.ai", "x.ai", "grok.com", "poe.com", "cursor.com", "cursor.sh", "cursorapi.com",
			"mistral.ai", "huggingface.co", "openrouter.ai", "midjourney.com", "character.ai",
		)},
		{ID: "media", Name: "流媒体", Color: "--r-media", Summary: "YouTube · Netflix · Spotify · Disney+ 等", Rules: []string{
			"GEOSITE,youtube", "GEOSITE,netflix", "GEOSITE,spotify", "GEOSITE,disney", "GEOSITE,hbo",
			"GEOSITE,primevideo", "GEOSITE,twitch", "GEOSITE,tiktok", "GEOSITE,bahamut", "GEOSITE,abema",
		}},
		{ID: "dev", Name: "开发", Color: "--r-dev", Summary: "GitHub · npm · Docker · PyPI · Go 等", Rules: suffixes(
			"github.com", "githubusercontent.com", "githubassets.com", "github.io", "ghcr.io",
			"npmjs.org", "npmjs.com", "yarnpkg.com", "docker.io", "docker.com", "pypi.org", "pythonhosted.org",
			"golang.org", "go.dev", "proxy.golang.org", "crates.io", "rust-lang.org", "gitlab.com", "bitbucket.org",
			"stackoverflow.com", "jsdelivr.net", "unpkg.com", "vercel.com", "brew.sh",
		)},
		{ID: "direct", Name: "直连", Color: "--r-direct", Summary: "国内网站 · 局域网", Direct: true, Rules: []string{
			"GEOSITE,private", "GEOSITE,cn", "GEOIP,private,no-resolve", "GEOIP,CN,no-resolve",
		}},
	}
}

func suffixes(domains ...string) []string {
	out := make([]string, len(domains))
	for i, d := range domains {
		out[i] = "DOMAIN-SUFFIX," + d
	}
	return out
}

// Count is the number of rules, MATCH included.
func Count(rs []Route) int {
	n := 1
	for _, r := range rs {
		n += len(r.Rules)
	}
	return n
}

// Target is a site a route is tested against.
type Target struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// MaxTargets is how many sites a route can be tested against. Each is a
// request through the node every few minutes.
const MaxTargets = 6

// DefaultTargets are the sites route id is tested against unless the user
// changes them, the first being the main target. The default route starts
// with the test URL of the settings, which is added by Settings.TargetsOf.
func DefaultTargets(id string) []Target {
	switch id {
	case Default:
		return []Target{{"Google", "https://www.google.com"}, {"GitHub", "https://github.com"}}
	case "ai":
		return []Target{{"OpenAI", "https://api.openai.com"}, {"Claude", "https://claude.ai"}, {"Gemini", "https://gemini.google.com"}}
	case "media":
		return []Target{{"YouTube", "https://www.youtube.com/generate_204"}, {"Netflix", "https://www.netflix.com"},
			{"Spotify", "https://open.spotify.com"}, {"Disney+", "https://www.disneyplus.com"}}
	case "dev":
		// raw, not github.com: that one times out on most nodes when they
		// are all tested at once, and Auto would count them as dead.
		return []Target{{"GitHub Raw", "https://raw.githubusercontent.com"}, {"GitHub", "https://github.com"}, {"npm", "https://registry.npmjs.org"},
			{"Docker Hub", "https://registry-1.docker.io/v2/"}, {"PyPI", "https://pypi.org"}}
	}
	return nil
}

// ParseTarget makes a target of an address as typed: the scheme is optional,
// and the site's name, its host without "www.", stands for it.
func ParseTarget(input string) (Target, error) {
	s := strings.TrimSpace(input)
	if s != "" && !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || strings.ContainsAny(u.Host, " \t") {
		return Target{}, fmt.Errorf(i18n.T("测速站点要是 http 或 https 链接: %s", "A test site must be an http or https link: %s"), input)
	}
	return Target{Name: strings.TrimPrefix(u.Hostname(), "www."), URL: s}, nil
}

// ValidTargets says what is wrong with a list of targets for route id.
func ValidTargets(id string, list []Target) error {
	if !slices.ContainsFunc(Template(), func(r Route) bool { return r.ID == id && !r.Direct }) {
		return fmt.Errorf(i18n.T("路由 %s 不能设置测速站点", "Route %s can't have test sites"), id)
	}
	if len(list) == 0 {
		return errors.New(i18n.T("至少要留一个测速站点", "Keep at least one test site"))
	}
	if len(list) > MaxTargets {
		return fmt.Errorf(i18n.T("一条路由最多 %d 个测速站点", "A route can have at most %d test sites"), MaxTargets)
	}
	for i, t := range list {
		if _, err := ParseTarget(t.URL); err != nil {
			return err
		}
		if t.Name == "" || !strings.Contains(t.URL, "://") {
			return fmt.Errorf(i18n.T("测速站点要是 http 或 https 链接: %s", "A test site must be an http or https link: %s"), t.URL)
		}
		if slices.ContainsFunc(list[:i], func(o Target) bool { return o.URL == t.URL }) {
			return fmt.Errorf(i18n.T("测速站点 %s 重复了", "Test site %s is there twice"), t.URL)
		}
	}
	return nil
}
