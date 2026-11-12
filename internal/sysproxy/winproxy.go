package sysproxy

// What Windows' proxy settings look like, kept apart from the calls that
// read and write them (sysproxy_windows.go) so that it is tested anywhere.

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf16"

	"github.com/oboo/moho/internal/i18n"
)

// WinINet's INTERNET_PER_CONN_FLAGS bits.
const (
	winDirect     = 0x1 // PROXY_TYPE_DIRECT
	winProxy      = 0x2 // PROXY_TYPE_PROXY: the proxies in ProxyServer
	winPAC        = 0x4 // PROXY_TYPE_AUTO_PROXY_URL
	winAutoDetect = 0x8 // PROXY_TYPE_AUTO_DETECT (WPAD)
)

// winSettings are the LAN connection's proxy settings, as WinINet has them.
type winSettings struct {
	flags    uint32
	server   string // ProxyServer: "host:port" or "http=…;https=…;socks=…"
	override string // ProxyOverride: entries separated by ';'
	pac      string // AutoConfigURL
}

// winService reads ws as a Service.
func winService(ws winSettings) Service {
	sv := Service{Name: "LAN", PAC: ws.pac, PACOn: ws.flags&winPAC != 0, AutoDetect: ws.flags&winAutoDetect != 0}
	sv.Web, sv.Secure, sv.Socks = parseWinServer(ws.server)
	on := ws.flags&winProxy != 0
	for _, e := range sv.endpoints() {
		e.On = on && e.Host != ""
	}
	for _, b := range strings.Split(ws.override, ";") {
		if b = strings.TrimSpace(b); b != "" {
			sv.Bypass = append(sv.Bypass, b)
		}
	}
	return sv
}

// winSettings is sv as WinINet has it. The proxies are on if any of them
// is: WinINet has one switch for them all.
func (sv Service) winSettings() winSettings {
	ws := winSettings{flags: winDirect, server: winServer(sv.Web, sv.Secure, sv.Socks), override: strings.Join(sv.Bypass, ";"), pac: sv.PAC}
	for _, e := range sv.endpoints() {
		if e.On && e.Host != "" {
			ws.flags |= winProxy
		}
	}
	if sv.PACOn && sv.PAC != "" {
		ws.flags |= winPAC
	}
	if sv.AutoDetect {
		ws.flags |= winAutoDetect
	}
	return ws
}

// winStatus is Check's answer for ws. A PAC script comes before the
// proxies, so while one is on they are not Moho's.
func winStatus(ws winSettings, port int) Status {
	sv := winService(ws)
	ep := func(e Endpoint) endpoint { return endpoint{on: e.On, host: e.Host, port: e.Port} }
	s := status(ep(sv.Web), ep(sv.Secure), ep(sv.Socks), port)
	if sv.PACOn {
		s.Ours = false
	}
	return s
}

// parseWinServer reads ProxyServer. A bare "host:port" serves every
// protocol but SOCKS; "proto=host:port" entries name theirs. Entries may
// carry a scheme ("http://host:port").
func parseWinServer(s string) (web, secure, socks Endpoint) {
	var bare *Endpoint
	for _, entry := range strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == ' ' || r == '\t' }) {
		proto, addr, named := strings.Cut(entry, "=")
		if !named {
			addr, proto = entry, ""
		}
		if _, rest, ok := strings.Cut(addr, "://"); ok {
			addr = rest
		}
		defPort := 80
		if proto = strings.ToLower(proto); proto == "socks" {
			defPort = 1080
		}
		e := hostPort(addr, defPort)
		switch proto {
		case "":
			bare = &e
		case "http":
			web = e
		case "https":
			secure = e
		case "socks":
			socks = e
		}
	}
	if bare != nil {
		if web.Host == "" {
			web = *bare
		}
		if secure.Host == "" {
			secure = *bare
		}
	}
	return web, secure, socks
}

func hostPort(addr string, defPort int) Endpoint {
	addr = strings.TrimSuffix(addr, "/")
	if h, p, err := net.SplitHostPort(addr); err == nil {
		n, _ := strconv.Atoi(p)
		return Endpoint{Host: h, Port: n}
	}
	return Endpoint{Host: strings.Trim(addr, "[]"), Port: defPort}
}

// winServer writes ProxyServer: the bare form when HTTP and HTTPS share a
// proxy and there is no SOCKS one, as Windows' own settings write it.
func winServer(web, secure, socks Endpoint) string {
	addr := func(e Endpoint) string { return net.JoinHostPort(e.Host, strconv.Itoa(e.Port)) }
	if web.Host != "" && web.Host == secure.Host && web.Port == secure.Port && socks.Host == "" {
		return addr(web)
	}
	var parts []string
	for _, p := range []struct {
		proto string
		e     Endpoint
	}{{"http", web}, {"https", secure}, {"socks", socks}} {
		if p.e.Host != "" {
			parts = append(parts, p.proto+"="+addr(p.e))
		}
	}
	return strings.Join(parts, ";")
}

// winBypassMax is how long ProxyOverride may get, in UTF-16 units. Microsoft
// documents no limit; 2064 is the one reported for WinINet, and a list of
// 1210 is known to be taken (#36). What does not fit is left out: the core
// still sends it direct, only by way of the proxy port.
const winBypassMax = 2000

// winBypass turns Moho's bypass list into ProxyOverride, and says which
// entries it left out. InternetSetOption refuses the whole call with
// ERROR_INVALID_PARAMETER for a list it does not like, and Microsoft does
// not say what it likes, so only forms known to be taken go in:
//
//   - a host, with wildcards, a scheme or a port (*.lan, 192.168.*,
//     https://intra, example.com:8080), and Windows' own <local>;
//   - an IPv4 network as the wildcards that cover it (10.0.0.0/8 → 10.*):
//     WinINet matches wildcards, not networks.
//
// Left out are IPv6 addresses and networks in any spelling (one of those is
// refused, #36, and WinINet cannot match them), entries with a separator,
// a space or a control character in them, with a path, or with a port
// that is no number, and what would make the list longer than
// winBypassMax. The order is kept and duplicates go. <local>, the names
// without a dot, is added, as Windows' "Don't use the proxy server for
// local addresses" does.
func winBypass(list []string) (string, []Dropped) {
	var out []string
	var dropped []Dropped
	size := len("<local>") // kept room for
	for _, b := range list {
		if b = strings.TrimSpace(b); b == "" {
			continue
		}
		forms, why := winBypassForms(b)
		if why == "" {
			forms = slices.DeleteFunc(forms, func(f string) bool { return f == "<local>" || slices.Contains(out, f) })
			n := 0
			for _, f := range forms {
				n += utf16Len(f) + 1 // and its separator
			}
			if size+n <= winBypassMax {
				out, size = append(out, forms...), size+n
				continue
			}
			why = "length"
		}
		if !slices.ContainsFunc(dropped, func(d Dropped) bool { return d.Entry == b }) {
			dropped = append(dropped, Dropped{Entry: b, Why: why})
		}
	}
	return strings.Join(append(out, "<local>"), ";"), dropped
}

// winBypassForms is one entry as ProxyOverride has it, or why it has none.
func winBypassForms(b string) (forms []string, why string) {
	if strings.HasPrefix(b, "<") && strings.HasSuffix(b, ">") && !strings.ContainsAny(b[1:len(b)-1], "<>;") {
		return []string{b}, "" // <local>, <-loopback>
	}
	if strings.ContainsFunc(b, func(r rune) bool { return r <= ' ' || r == 0x7f || strings.ContainsRune(`;,"<>|\^`, r) }) {
		return nil, "malformed"
	}
	host, scheme := b, ""
	if sch, rest, ok := strings.Cut(b, "://"); ok {
		scheme, host = sch+"://", strings.TrimSuffix(rest, "/")
	}
	if isIPv6Entry(host) {
		return nil, "ipv6"
	}
	if scheme == "" && strings.Contains(host, "/") {
		if w := wildcards(host); len(w) > 0 {
			return w, ""
		}
		return nil, "malformed"
	}
	b = scheme + host
	if h, port, ok := strings.Cut(host, ":"); ok {
		if _, err := strconv.ParseUint(port, 10, 16); err != nil && port != "*" {
			return nil, "malformed"
		}
		host = h
	}
	if host == "" || strings.Contains(host, "/") {
		return nil, "malformed"
	}
	return []string{b}, ""
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// winMinimalBypass is the list tried when Windows refuses bowel's: this
// machine and the private networks, in the forms Windows' own settings
// page and every other proxy program write.
var winMinimalBypass, _ = winBypass([]string{"localhost", "127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})

// winSet names the options a write sets: the rest stay as Windows has them.
type winSet uint8

const (
	setFlags winSet = 1 << iota
	setServer
	setBypass
	setPAC
)

// winTakeover points the proxies at server through write, which sets the
// options named of a winSettings. Windows refuses a write whole, and says
// neither which option it was about nor why, so a refusal is narrowed
// down by what it takes next: the same with the minimal bypass list, then
// with no bypass list at all, which leaves the system's own. The first
// that works is the takeover, and Written says what was refused. If none
// does, the error names what the last, smallest write held.
func winTakeover(write func(winSettings, winSet) error, server string, bypass []string) (Written, error) {
	ws := winSettings{flags: winDirect | winProxy, server: server}
	var w Written
	ws.override, w.Dropped = winBypass(bypass)
	first := write(ws, setFlags|setServer|setBypass)
	if first == nil {
		w.Bypass = ws.override
		return w, nil
	}
	w.Refused = &Refused{Value: "ProxyOverride", Content: ws.override, Error: first.Error()}
	if ws.override != winMinimalBypass {
		ws.override = winMinimalBypass
		if write(ws, setFlags|setServer|setBypass) == nil {
			w.Bypass, w.Refused.Instead = ws.override, "minimal"
			return w, nil
		}
	}
	err := write(ws, setFlags|setServer)
	if err == nil {
		w.Refused.Instead = "kept"
		return w, nil
	}
	// Not the bypass list, then: the write holds nothing else but these.
	w.Refused = &Refused{Value: "ProxyServer", Content: server, Error: err.Error()}
	return w, fmt.Errorf(i18n.T("Windows 不接受代理地址 ProxyServer=%q（flags=%#x，不写「不走代理」的列表也一样）：%w", "Windows refused ProxyServer=%q (flags=%#x, and so it did without a bypass list): %w"), server, ws.flags, err)
}

// winRefusal names what a refused write held, for its error.
func winRefusal(ws winSettings, set winSet, err error) error {
	var parts []string
	if set&setFlags != 0 {
		parts = append(parts, fmt.Sprintf("flags=%#x", ws.flags))
	}
	if set&setServer != 0 {
		parts = append(parts, fmt.Sprintf("ProxyServer=%q", ws.server))
	}
	if set&setBypass != 0 {
		parts = append(parts, fmt.Sprintf("ProxyOverride=%q", ws.override))
	}
	if set&setPAC != 0 {
		parts = append(parts, fmt.Sprintf("AutoConfigURL=%q", ws.pac))
	}
	return fmt.Errorf(i18n.T("Windows 不接受这次写入的代理设置（%s）：%w", "Windows refused the proxy settings written (%s): %w"), strings.Join(parts, " "), err)
}

// winRestoreSet is what putting saved back writes, over cur: the script's
// address only if it differs, as there is seldom one and an empty string
// written for it is one more thing Windows could refuse.
func winRestoreSet(saved, cur winSettings) winSet {
	set := setFlags | setServer | setBypass
	if saved.pac != cur.pac {
		set |= setPAC
	}
	return set
}

// isIPv6Entry reports whether a bypass entry names an IPv6 address, bracketed
// or not, wildcards included (::1, [::1], fe80::*). One colon is a host and
// port (example.com:8080), which WinINet takes.
func isIPv6Entry(b string) bool {
	return strings.HasPrefix(b, "[") || strings.Count(b, ":") >= 2
}

// wildcards covers an IPv4 network with WinINet wildcards. It accepts
// macOS's short form too (169.254/16).
func wildcards(cidr string) []string {
	addr, bitsStr, _ := strings.Cut(cidr, "/")
	if !strings.Contains(addr, ":") {
		for strings.Count(addr, ".") < 3 {
			addr += ".0"
		}
	}
	p, err := netip.ParsePrefix(addr + "/" + bitsStr)
	if err != nil || !p.Addr().Is4() {
		return nil
	}
	p = p.Masked()
	ip, bits := p.Addr().As4(), p.Bits()
	if bits == 32 {
		return []string{p.Addr().String()}
	}
	whole := bits / 8 // octets fixed by the network
	prefix := ""
	for i := range whole {
		prefix += strconv.Itoa(int(ip[i])) + "."
	}
	if bits%8 == 0 {
		return []string{prefix + "*"}
	}
	n := 1 << (8 - bits%8) // values the partial octet takes
	var out []string
	for v := int(ip[whole]); v < int(ip[whole])+n; v++ {
		w := prefix + strconv.Itoa(v)
		if whole < 3 {
			w += ".*"
		}
		out = append(out, w)
	}
	return out
}

// winInvalidParameter is ERROR_INVALID_PARAMETER, what WinINet answers
// for a call it does not take.
const winInvalidParameter = syscall.Errno(87)

// winQuery reads the LAN settings through query, which takes the WinINet
// handle to ask on. It asks with none, as Windows' own examples do; some
// builds refuse that as an invalid parameter (#124, Windows 11 26200), so
// then it asks again on a session handle from open, which loads the same
// settings. The handle is closed before it returns. Only reads go this
// way: a write on a session handle may change only that session.
func winQuery[T any](query func(h uintptr) (T, error), open func() (uintptr, func(), error)) (T, error) {
	v, err := query(0)
	if err == nil || !errors.Is(err, winInvalidParameter) {
		return v, err
	}
	h, done, oerr := open()
	if oerr != nil {
		return v, fmt.Errorf("%w (and on a session handle: %w)", err, oerr)
	}
	defer done()
	v, herr := query(h)
	if herr != nil {
		return v, fmt.Errorf("%w (and on a session handle: %w)", err, herr)
	}
	return v, nil
}
