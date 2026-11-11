package routes

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"path"
	"runtime"
	"slices"
	"strings"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"

	"github.com/oboo/moho/internal/i18n"
)

// UserRule is a rule people add: what it matches and the route it sends
// there. User rules come before the template's, so they win.
type UserRule struct {
	Kind  string `json:"kind"`  // suffix, domain, keyword, ip, app
	Value string `json:"value"` // a domain in ASCII, a keyword, an IP prefix, or an application
	Route string `json:"route"` // a route group's id
	// Service is the id of the service it is in; "" is the group's 我添加的.
	Service string `json:"service,omitempty"`
}

// The kinds of UserRule.
const (
	KindSuffix  = "suffix"  // the domain and every name under it
	KindDomain  = "domain"  // that name only
	KindKeyword = "keyword" // any name containing it
	KindIP      = "ip"      // an address or a range
	KindApp     = "app"     // an application on this computer, whatever it connects to
	KindGeo     = "geo"     // Moho's own: a GeoSite list
	KindGeoIP   = "geoip"   // Moho's own: a GeoIP list
)

// Kinds are the kinds of UserRule, in the order the interface offers them.
var Kinds = []string{KindSuffix, KindDomain, KindKeyword, KindIP}

// AllKinds are Kinds and KindApp: every kind a rule of the user's can have.
// An application is never guessed, and only offered where AppRules is true.
func AllKinds() []string { return append(slices.Clone(Kinds), KindApp) }

// AppRules says whether a rule for an application can work here: whether
// the core can tell which program opened a connection. It can on Windows
// and Linux. On macOS 27 the core, started by bowel, gets an empty socket
// list from the kernel; on Android and iOS no app may read another's
// sockets. A rule saved elsewhere stays valid here; it only never matches.
var AppRules = runtime.GOOS == "windows" || runtime.GOOS == "linux"

// Same reports whether two rules match the same traffic, whatever their
// routes. Every value but an application's is in lower case already, and
// the core compares an application's name without regard to case.
func (r UserRule) Same(o UserRule) bool {
	return r.Kind == o.Kind && strings.EqualFold(r.Value, o.Value)
}

// Mihomo is the rule as mihomo writes it, without its target.
func (r UserRule) Mihomo() string {
	switch r.Kind {
	case KindDomain:
		return "DOMAIN," + r.Value
	case KindKeyword:
		return "DOMAIN-KEYWORD," + r.Value
	case KindIP:
		return "IP-CIDR," + r.Value + ",no-resolve"
	case KindApp:
		if isBundle(r.Value) {
			// Every program inside the bundle: a browser's connections
			// are its helpers'.
			return "PROCESS-PATH-WILDCARD,*/" + r.Value + "/*"
		}
		return "PROCESS-NAME," + r.Value
	}
	return "DOMAIN-SUFFIX," + r.Value
}

// bundles says whether applications are bundles here, as on macOS, where
// what people call an app is a directory of programs. Elsewhere a name
// ending in ".app" is a program's like any other.
var bundles = runtime.GOOS == "darwin"

// isBundle reports whether an application is named as a macOS bundle
// ("Google Chrome.app") and not as a program ("chrome.exe").
func isBundle(app string) bool {
	return bundles && len(app) > 4 && strings.EqualFold(app[len(app)-4:], ".app")
}

// Describe is the rule in words: what it matches.
func (r UserRule) Describe() string {
	switch r.Kind {
	case KindDomain:
		return fmt.Sprintf(i18n.T("仅 %s", "Only %s"), r.Value)
	case KindKeyword:
		return fmt.Sprintf(i18n.T("名字含 “%s” 的域名", "Domains containing “%s”"), r.Value)
	case KindIP:
		return fmt.Sprintf(i18n.T("IP %s", "IP %s"), r.Value)
	case KindApp:
		return fmt.Sprintf(i18n.T("应用 %s", "App %s"), r.Value)
	}
	return fmt.Sprintf(i18n.T("%s 及其子域名", "%s and its subdomains"), r.Value)
}

// KindName is a kind in words.
func KindName(kind string) string {
	switch kind {
	case KindDomain:
		return i18n.T("仅此域名", "This domain only")
	case KindKeyword:
		return i18n.T("关键词", "Keyword")
	case KindIP:
		return i18n.T("IP 段", "IP range")
	case KindApp:
		return i18n.T("应用", "App")
	case KindGeo:
		return i18n.T("内置域名库", "Built-in domain list")
	case KindGeoIP:
		return i18n.T("内置 IP 库", "Built-in IP list")
	}
	return i18n.T("域名及子域名", "Domain and subdomains")
}

// Matched reports whether a connection the core matched with ruleType and
// payload (as /connections reports them: "DomainSuffix", "openai.com")
// matched this rule.
func (r UserRule) Matched(ruleType, payload string) bool {
	if r.Kind == KindApp {
		kind, pattern, _ := strings.Cut(r.Mihomo(), ",")
		return strings.EqualFold(strings.ReplaceAll(kind, "-", ""), ruleType) && strings.EqualFold(payload, pattern)
	}
	want := map[string]string{KindSuffix: "domainsuffix", KindDomain: "domain", KindKeyword: "domainkeyword", KindIP: "ipcidr"}[r.Kind]
	t := strings.ToLower(strings.ReplaceAll(ruleType, "-", ""))
	if r.Kind == KindIP && t == "ipcidr6" {
		t = "ipcidr"
	}
	return t == want && strings.EqualFold(payload, r.Value)
}

// Matches reports whether the rule would take a connection to host (a
// domain, or "") at ip (or ""). A rule for an application never does: it
// goes by who connects (MatchesApp), not by where to.
func (r UserRule) Matches(host, ip string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	switch r.Kind {
	case KindSuffix:
		return host != "" && (host == r.Value || strings.HasSuffix(host, "."+r.Value))
	case KindDomain:
		return host == r.Value
	case KindKeyword:
		return host != "" && strings.Contains(host, r.Value)
	case KindIP:
		p, err := netip.ParsePrefix(r.Value)
		a, err2 := netip.ParseAddr(ip)
		return err == nil && err2 == nil && p.Contains(a.Unmap())
	}
	return false
}

// MatchesApp reports whether a rule for an application takes the
// connections of the program named process, whose executable is at
// processPath; either may be "".
func (r UserRule) MatchesApp(process, processPath string) bool {
	if r.Kind != KindApp {
		return false
	}
	p := strings.ReplaceAll(processPath, `\`, "/")
	if isBundle(r.Value) {
		return strings.Contains(strings.ToLower(p), "/"+strings.ToLower(r.Value)+"/")
	}
	if process == "" && p != "" {
		process = path.Base(p)
	}
	return process != "" && strings.EqualFold(process, r.Value)
}

// AppOf is the application a program belongs to, as a rule names it: the
// outermost bundle of a path on macOS ("Google Chrome.app" for the helper
// deep inside it), else the program's file name. "" when there is none.
func AppOf(processPath string) string {
	p := strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(processPath), `\`, "/"), "/")
	if i := strings.Index(strings.ToLower(p), ".app/"); i > 0 && bundles {
		p = p[:i+4]
	}
	if p == "" {
		return ""
	}
	return path.Base(p)
}

// parseApp reads an application's name, or the path of its program.
func parseApp(s string) (string, error) {
	app := AppOf(s)
	bad := strings.ContainsFunc(app, func(c rune) bool { return c == ',' || c < ' ' || c == 0x7f })
	if app == "" || app == "." || app == ".." || bad || len(app) > 255 {
		return "", fmt.Errorf(i18n.T("不是应用的进程名: %s", "Not a program's name: %s"), s)
	}
	return app, nil
}

// ParseRule reads what a person typed into a rule of kind for route.
// kind "" guesses: an address or a range is an IP rule, anything else a
// domain and its subdomains. It takes a pasted link, "*.example.com" and
// ".example.com" too. An application is never guessed, since "Code.exe"
// reads as a domain: its kind has to be given, with the program's name
// or path.
func ParseRule(kind, text, route string) (UserRule, error) {
	s := strings.TrimSpace(text)
	if s == "" {
		if kind == KindApp {
			return UserRule{}, errors.New(i18n.T("先写应用的进程名", "Type the program's name first"))
		}
		return UserRule{}, errors.New(i18n.T("先写一个域名或 IP", "Type a domain or an IP first"))
	}
	if kind == KindApp {
		app, err := parseApp(s)
		if err != nil {
			return UserRule{}, err
		}
		return UserRule{Kind: KindApp, Value: app, Route: route}, nil
	}
	if kind == "" || kind == KindIP {
		if p, ok := parseIP(s); ok {
			return UserRule{Kind: KindIP, Value: p, Route: route}, nil
		}
		if kind == KindIP {
			return UserRule{}, fmt.Errorf(i18n.T("不是 IP 或 IP 段: %s", "Not an IP or an IP range: %s"), s)
		}
		kind = KindSuffix
	}
	if kind == KindKeyword {
		s = strings.ToLower(s)
		if strings.ContainsAny(s, ", \t/") {
			return UserRule{}, fmt.Errorf(i18n.T("关键词里不能有逗号、空格或斜杠: %s", "A keyword can't have commas, spaces or slashes: %s"), s)
		}
		return UserRule{Kind: KindKeyword, Value: s, Route: route}, nil
	}
	if !slices.Contains(Kinds, kind) {
		return UserRule{}, fmt.Errorf(i18n.T("未知规则类型: %s", "Unknown kind of rule: %s"), kind)
	}
	d, wild, err := parseDomain(s)
	if err != nil {
		return UserRule{}, err
	}
	if wild && kind == KindDomain {
		kind = KindSuffix // "*.x" asks for what is under x
	}
	return UserRule{Kind: kind, Value: d, Route: route}, nil
}

// Valid says what is wrong with a saved rule, if anything.
func (r UserRule) Valid() error {
	n, err := ParseRule(r.Kind, r.Value, r.Route)
	if err != nil {
		return err
	}
	if n.Value != r.Value || n.Kind != r.Kind {
		return fmt.Errorf(i18n.T("规则写法不对: %s", "Malformed rule: %s"), r.Value)
	}
	return nil
}

func parseIP(s string) (string, bool) {
	if p, err := netip.ParsePrefix(s); err == nil {
		return p.Masked().String(), true
	}
	if a, err := netip.ParseAddr(s); err == nil {
		a = a.Unmap()
		return netip.PrefixFrom(a, a.BitLen()).String(), true
	}
	return "", false
}

// parseDomain is s as a domain in ASCII; wild when it was written
// "*.example.com" or ".example.com".
func parseDomain(s string) (d string, wild bool, err error) {
	if strings.Contains(s, "://") {
		if u, e := url.Parse(s); e == nil && u.Hostname() != "" {
			s = u.Hostname()
		}
	}
	s, _, _ = strings.Cut(s, "/")
	if h, _, ok := strings.Cut(s, ":"); ok && !strings.Contains(h, ":") {
		s = h // a port
	}
	if t, ok := strings.CutPrefix(s, "*."); ok {
		s, wild = t, true
	} else if t, ok := strings.CutPrefix(s, "."); ok {
		s, wild = t, true
	}
	s = strings.TrimSuffix(strings.ToLower(s), ".")
	bad := fmt.Errorf(i18n.T("不是有效的域名: %s", "Not a valid domain: %s"), s)
	if a, e := idna.Lookup.ToASCII(s); e == nil {
		s = a
	} else {
		return "", false, bad
	}
	if s == "" || !strings.Contains(s, ".") && s != "localhost" {
		return "", false, bad
	}
	for _, l := range strings.Split(s, ".") {
		if l == "" || len(l) > 63 || strings.HasPrefix(l, "-") || strings.HasSuffix(l, "-") {
			return "", false, bad
		}
		for _, ch := range l {
			if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_') {
				return "", false, bad
			}
		}
	}
	return s, wild, nil
}

// Site is the part of host people mean by "this site": its registrable
// domain, as api.openai.com → openai.com, www.bbc.co.uk → bbc.co.uk. An
// IP or a name with no public suffix stays as it is.
func Site(host string) string {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if _, err := netip.ParseAddr(host); err == nil {
		return host
	}
	if s, err := publicsuffix.EffectiveTLDPlusOne(host); err == nil {
		return s
	}
	return host
}

// Upsert puts r in rules: in the place of a rule matching the same
// traffic, or first, as the newest.
func Upsert(rules []UserRule, r UserRule) []UserRule {
	out := slices.Clone(rules)
	if i := slices.IndexFunc(out, r.Same); i >= 0 {
		out[i] = r
		return out
	}
	return append([]UserRule{r}, out...)
}

// Remove takes the rule matching r's traffic out of rules.
func Remove(rules []UserRule, r UserRule) []UserRule {
	return slices.DeleteFunc(slices.Clone(rules), r.Same)
}

// Explain says where a connection to host would go and why, as far as can
// be told without the core: rules in the order the core tries them, the
// domain and address ones. sure is false when GeoSite or GeoIP data would
// decide; route is then "".
func Explain(host string, user []UserRule, tpl []Route) (route, rule string, mine, sure bool) {
	route, rule, _, mine, sure = ExplainSets(host, user, tpl, nil)
	return route, rule, mine, sure
}

// ExplainSets is Explain, looking inside rule sets with match, which says
// which rule of set id catches host at ip. set is the id of the set that
// decided, if one did.
func ExplainSets(host string, user []UserRule, tpl []Route, match func(id, host, ip string) (string, bool)) (route, rule, set string, mine, sure bool) {
	ip := ""
	if _, err := netip.ParseAddr(host); err == nil {
		ip, host = host, ""
	}
	for _, l := range Lines(tpl, user) {
		if l.Set != "" {
			if match != nil {
				if hit, ok := match(l.Set, host, ip); ok {
					return l.Route, hit, l.Set, false, true
				}
			}
			continue
		}
		r := ruleOf(l.Text)
		if r.Kind == KindGeo || r.Kind == KindGeoIP {
			continue
		}
		if (UserRule{Kind: r.Kind, Value: r.Value}).Matches(host, ip) {
			return l.Route, l.Text, "", l.Mine, true
		}
	}
	return "", "", "", false, false
}
