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
	Kind  string `json:"kind"`  // suffix, domain, keyword, ip, geo, geoip, app
	Value string `json:"value"` // a domain in ASCII, a keyword, an IP prefix, a GeoSite or GeoIP list, or an application
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
	KindGeo     = "geo"     // the domains of a GeoSite list: youtube, or google@cn
	KindGeoIP   = "geoip"   // the addresses of a GeoIP list: CN, private
)

// Kinds are the kinds of UserRule, in the order the interface offers them.
var Kinds = []string{KindSuffix, KindDomain, KindKeyword, KindIP, KindGeo, KindGeoIP}

// GeoLists are the names of the lists the core's data holds for kind
// (KindGeo or KindGeoIP), in lower case and sorted; ok is false when the
// data can't be read. It is set where the data is known (geodata.Use).
// Without it, or the data, a list is taken as written.
var GeoLists func(kind string) (names []string, ok bool)

// AllKinds are Kinds and KindApp: every kind a rule of the user's can have.
// An application is never guessed, and only offered where AppRules is true.
func AllKinds() []string { return append(slices.Clone(Kinds), KindApp) }

// AppRules says whether a rule for an application can work here: whether
// the core can tell which program opened a connection. It can on Windows
// and Linux, and on macOS, where two things make it so: the core is not
// the app's child (core.startCore) and it is signed under an identifier
// with a dot in it (core.BinName). On iOS no app may read another's
// sockets, nor on Android, but from Android 10 the VPN app may ask who owns
// a connection: the phone's bridge turns this on there (bridge.SetAppRules),
// and a rule names the app's package. A rule saved elsewhere stays valid
// here; it only never matches.
var AppRules = runtime.GOOS == "windows" || runtime.GOOS == "linux" || runtime.GOOS == "darwin"

// Unlisted reports whether r names a GeoSite or GeoIP list that the core's
// data doesn't hold: such a rule doesn't count (Active) until the data has
// the list again.
func (r UserRule) Unlisted() bool {
	return (r.Kind == KindGeo || r.Kind == KindGeoIP) && geoHeld(r.Kind, r.Value) == missing
}

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
	case KindGeo:
		return "GEOSITE," + r.Value
	case KindGeoIP:
		return "GEOIP," + r.Value + ",no-resolve"
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

// WebKitProcess is the program that connects for every web page macOS's
// own WebKit shows: Safari's, and those of each app with a web view in it
// (WKWebView). It lives in WebKit.framework, outside any app's bundle, and
// one runs for each such app, all at the same path. The kernel knows which
// app each works for (so_e_pid in its socket list), but the core goes by
// the process that owns the socket (so_last_pid): to the core they are
// one program, and a rule for Safari.app never sees a page. Tried on macOS
// 27.2 with mihomo v1.19.33: a WKWebView app's page load was reported as
// this process and matched PROCESS-NAME, not the app's bundle.
const WebKitProcess = "com.apple.WebKit.Networking"

// webKitApps are the bundles known to load their pages with WebKitProcess
// and little else of their own: a rule for one of them alone is not what
// its name says.
var webKitApps = []string{"Safari.app", "Safari Technology Preview.app"}

// IsWebKit reports whether a rule for app is one for WebKitProcess, which
// takes the pages of every app that uses it.
func IsWebKit(app string) bool { return bundles && strings.EqualFold(app, WebKitProcess) }

// UsesWebKit reports whether app is known to leave its pages to
// WebKitProcess, where a rule for app does not reach them.
func UsesWebKit(app string) bool {
	return bundles && slices.ContainsFunc(webKitApps, func(a string) bool { return strings.EqualFold(a, app) })
}

// AppNote says what a rule for app does that its name does not: a rule
// for Safari leaves its pages out, one for WebKitProcess takes other apps'
// pages in. "" when the rule is what it looks like, as far as is known.
func AppNote(app string) string {
	switch {
	case UsesWebKit(app):
		return fmt.Sprintf(i18n.T("%[1]s 的网页不是它自己连的，是 macOS 的 WebKit 网络进程（%[2]s）连的，这条规则管不到网页，只管 %[1]s 自己发出的连接。", "%[1]s doesn't load its pages itself: macOS's WebKit networking process (%[2]s) does, so this rule won't reach web pages, only what %[1]s itself connects to."), app, WebKitProcess)
	case IsWebKit(app):
		return i18n.T("这是 macOS 的 WebKit 网络进程：Safari 的网页，和所有用系统 WebKit 显示网页的应用，都由它连网。内核分不出它在替哪个应用连，这条规则会把它们一起管到。", "This is macOS's WebKit networking process: it connects for Safari's pages and for every app that shows web pages with the system's WebKit. The core can't tell which app it is working for, so this rule takes them all.")
	}
	return ""
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
	case KindGeo:
		return fmt.Sprintf(i18n.T("GeoSite 列表 %s 里的域名", "Domains in the GeoSite list %s"), r.Value)
	case KindGeoIP:
		return fmt.Sprintf(i18n.T("GeoIP 列表 %s 里的地址", "Addresses in the GeoIP list %s"), r.Value)
	case KindApp:
		switch {
		case IsWebKit(r.Value):
			return fmt.Sprintf(i18n.T("应用 %s（Safari 的网页和所有内嵌网页的应用）", "App %s (Safari's pages and every app with web pages in it)"), r.Value)
		case UsesWebKit(r.Value):
			return fmt.Sprintf(i18n.T("应用 %s（不含网页）", "App %s (not its web pages)"), r.Value)
		}
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
		return i18n.T("GeoSite 域名库", "GeoSite list")
	case KindGeoIP:
		return i18n.T("GeoIP 地址库", "GeoIP list")
	}
	return i18n.T("域名及子域名", "Domain and subdomains")
}

// Matched reports whether a connection the core matched with ruleType and
// payload (as /connections reports them: "DomainSuffix", "openai.com")
// matched this rule.
func (r UserRule) Matched(ruleType, payload string) bool {
	switch r.Kind {
	case KindApp:
		kind, pattern, _ := strings.Cut(r.Mihomo(), ",")
		return strings.EqualFold(strings.ReplaceAll(kind, "-", ""), ruleType) && strings.EqualFold(payload, pattern)
	case KindGeo:
		return strings.EqualFold(ruleType, "GeoSite") && strings.EqualFold(payload, r.Value)
	case KindGeoIP:
		return strings.EqualFold(ruleType, "GeoIP") && strings.EqualFold(payload, r.Value)
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
// goes by who connects (MatchesApp), not by where to; nor does one for a
// GeoSite or GeoIP list, which only the core can look in.
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
// kind "" guesses: an address or a range is an IP rule, a name the
// GeoSite data has a list of (youtube) that list, anything else a domain
// and its subdomains. It takes a pasted link, "*.example.com" and
// ".example.com" too. An application is never guessed, since "Code.exe"
// reads as a domain: its kind has to be given, with the program's name
// or path.
//
// What says its own kind goes by it, whatever kind is given: a rule as
// mihomo writes it ("GEOSITE,youtube,Proxy", whose target is dropped:
// the route is where the rule is added) and "geosite:youtube" or
// "geoip:cn". A GeoSite or GeoIP list has to be in the core's data
// (GeoLists), since the core refuses a config naming one that isn't.
func ParseRule(kind, text, route string) (UserRule, error) {
	return parseRule(kind, text, route, true)
}

// parseRule is ParseRule; typed false reads only the rule as it is stored,
// whatever the data now holds.
func parseRule(kind, text, route string, typed bool) (UserRule, error) {
	s := strings.TrimSpace(text)
	if s == "" {
		switch kind {
		case KindApp:
			return UserRule{}, errors.New(i18n.T("先写应用的进程名", "Type the program's name first"))
		case KindGeo, KindGeoIP:
			return UserRule{}, errors.New(i18n.T("先写列表的名字", "Type the list's name first"))
		}
		return UserRule{}, errors.New(i18n.T("先写一个域名或 IP", "Type a domain or an IP first"))
	}
	if typed {
		k, v, err := selfKinded(s)
		if err != nil {
			return UserRule{}, err
		}
		if k != "" {
			kind, s = k, v
		}
	}
	if kind == KindGeo || kind == KindGeoIP {
		name, err := parseGeo(kind, s, typed)
		if err != nil {
			return UserRule{}, err
		}
		return UserRule{Kind: kind, Value: name, Route: route}, nil
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
		if typed && !strings.ContainsAny(s, ".:/") {
			// A word, which no domain is: a GeoSite list, if there is one.
			if name, err := parseGeo(KindGeo, s, true); err == nil && geoHeld(KindGeo, name) == held {
				return UserRule{Kind: KindGeo, Value: name, Route: route}, nil
			}
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

// Valid says what is wrong with a saved rule, if anything. A GeoSite or
// GeoIP list the data no longer has is not: the rule just doesn't count
// (Active) until the data has it again.
func (r UserRule) Valid() error {
	n, err := parseRule(r.Kind, r.Value, r.Route, false)
	if err != nil {
		return err
	}
	if n.Value != r.Value || n.Kind != r.Kind {
		return fmt.Errorf(i18n.T("规则写法不对: %s", "Malformed rule: %s"), r.Value)
	}
	return nil
}

// mihomoKinds are the rule types as mihomo writes them that are a kind of
// UserRule. PROCESS-NAME is one only where AppRules is true.
var mihomoKinds = map[string]string{
	"DOMAIN": KindDomain, "DOMAIN-SUFFIX": KindSuffix, "DOMAIN-KEYWORD": KindKeyword,
	"IP-CIDR": KindIP, "IP-CIDR6": KindIP, "GEOSITE": KindGeo, "GEOIP": KindGeoIP, "PROCESS-NAME": KindApp,
}

// otherMihomo are mihomo's other rule types (rules/parser.go), which a rule
// set can hold but a rule of the user's can't.
var otherMihomo = strings.Fields(`DOMAIN-REGEX DOMAIN-WILDCARD SRC-GEOIP IP-ASN SRC-IP-ASN IP-SUFFIX
	SRC-IP-CIDR SRC-IP-SUFFIX SRC-PORT DST-PORT IN-PORT DSCP PROCESS-PATH PROCESS-NAME-REGEX
	PROCESS-PATH-REGEX PROCESS-NAME-WILDCARD PROCESS-PATH-WILDCARD NETWORK UID IN-TYPE IN-USER
	IN-NAME AND OR NOT RULE-SET SUB-RULE MATCH`)

// selfKinded reads text that says its own kind: a rule as mihomo writes it,
// or "geosite:x" / "geoip:x". kind is "" when text is not such.
func selfKinded(s string) (kind, value string, err error) {
	if p, v, ok := strings.Cut(s, ":"); ok {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "geosite":
			return KindGeo, strings.TrimSpace(v), nil
		case "geoip":
			return KindGeoIP, strings.TrimSpace(v), nil
		}
	}
	t, rest, ok := strings.Cut(s, ",")
	if !ok {
		return "", "", nil
	}
	t = strings.ToUpper(strings.TrimSpace(t))
	if slices.Contains(otherMihomo, t) || t == "PROCESS-NAME" && !AppRules {
		return "", "", fmt.Errorf(i18n.T("%s 规则不能在这里添加，可以写进规则集文件，再导入规则集", "A %s rule can't be added here. Put it in a rule set file and import that"), t)
	}
	kind = mihomoKinds[t]
	if kind == "" {
		return "", "", nil
	}
	value, _, _ = strings.Cut(rest, ",") // what follows is the target and its options
	return kind, strings.TrimSpace(value), nil
}

// How a list stands in the core's data.
const (
	held    = iota
	missing // the data is there and hasn't the list
	unknown // there is no data to tell by
)

// geoHeld says whether the data holds the list name of kind.
func geoHeld(kind, name string) int {
	if GeoLists == nil {
		return unknown
	}
	names, ok := GeoLists(kind)
	if !ok {
		return unknown
	}
	if kind == KindGeoIP && name == "lan" {
		return held // the core's own: the local networks, with no data
	}
	list, _, _ := strings.Cut(strings.ToLower(name), "@") // google@cn: of google, those marked cn
	if _, found := slices.BinarySearch(names, list); found {
		return held
	}
	return missing
}

// parseGeo reads the name of a GeoSite or GeoIP list: in lower case, as the
// core looks them up, but a country's code in capitals, as Moho's own
// rules name them (GEOIP,CN), so that the same rule reads the same. check
// says whether the list must be in the data.
func parseGeo(kind, s string, check bool) (string, error) {
	name := strings.ToLower(strings.TrimSpace(s))
	ok := name != "" && len(name) <= 64 && !strings.HasPrefix(name, "@")
	for _, c := range name {
		ok = ok && (c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || strings.ContainsRune("-_.!", c) || c == '@' && kind == KindGeo)
	}
	what := "GeoSite"
	if kind == KindGeoIP {
		what = "GeoIP"
	}
	if !ok {
		return "", fmt.Errorf(i18n.T("不是 %s 列表的名字: %s", "Not the name of a %s list: %s"), what, s)
	}
	if kind == KindGeoIP && len(name) == 2 {
		name = strings.ToUpper(name)
	}
	if check && geoHeld(kind, name) == missing {
		names, _ := GeoLists(kind)
		if near := nearLists(names, name); len(near) > 0 {
			return "", fmt.Errorf(i18n.T("%s 数据里没有叫 %s 的列表，是不是：%s", "The %s data has no list called %s. Did you mean: %s"), what, name, strings.Join(near, i18n.T("、", ", ")))
		}
		return "", fmt.Errorf(i18n.T("%s 数据里没有叫 %s 的列表", "The %s data has no list called %s"), what, name)
	}
	return name, nil
}

// nearLists are up to three of names that a mistyped name may have meant:
// those it begins, then those it is in.
func nearLists(names []string, name string) []string {
	name, _, _ = strings.Cut(strings.ToLower(name), "@")
	var starts, has []string
	for _, n := range names {
		switch {
		case strings.HasPrefix(n, name):
			starts = append(starts, n)
		case strings.Contains(n, name):
			has = append(has, n)
		}
	}
	out := append(starts, has...)
	return out[:min(3, len(out))]
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

// Edit puts next where the rule matching old's traffic is, dropping any
// other rule that matches next's. On the same route it stays in old's
// service; on another it goes to that group's 我添加的. With no such rule,
// next comes first, as Upsert puts it.
func Edit(rules []UserRule, old, next UserRule) []UserRule {
	i := slices.IndexFunc(rules, old.Same)
	if i < 0 {
		return Upsert(rules, next)
	}
	next.Service = ""
	if rules[i].Route == next.Route {
		next.Service = rules[i].Service
	}
	out := make([]UserRule, 0, len(rules))
	for j, r := range rules {
		switch {
		case j == i:
			out = append(out, next)
		case !r.Same(next):
			out = append(out, r)
		}
	}
	return out
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
