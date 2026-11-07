package monitor

import (
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/oboo/moho/internal/mihomo"
)

// dialFailure is what a failed-dial log line of mihomo says.
type dialFailure struct {
	Network string // "tcp" / "udp"
	Group   string // the proxy (usually the rule's target group) mihomo dialed through
	Rule    string // rule type, e.g. "Domain"; empty in global/direct mode
	Payload string
	Source  string // ip:port of the client
	Process string // process name, if mihomo found it
	Host    string // destination host or IP
	Port    int
	Error   string
}

// mihomo's tunnel logs a failed dial, at level warning, as
//
//	[TCP] dial Refused (match Domain/refused.example) 127.0.0.1:62508(curl) --> refused.example:80 error: 127.0.0.1:1 connect error: dial tcp 127.0.0.1:1: connect: connection refused
//	[UDP] dial Refused (match IPCIDR/9.9.9.9/32) 127.0.0.1:51520(python3.11) --> 9.9.9.9:53 error: ...
//	[TCP] dial GLOBAL 127.0.0.1:62616(curl) --> global.example:80 error: ...   (no rule: global/direct mode)
//
// The source is "ip:port", "ip:port(process)", "ip:port(process, uid=N)"
// or "ip:port(uid=N)". Group names may contain spaces, process names
// parentheses and rule payloads slashes, hence the lazy groups anchored on
// the fixed " --> " and " error: ".
var dialFailRe = regexp.MustCompile(
	`^\[(TCP|UDP)\] dial (.+?) (?:\(match ([^/()]*)/(.*?)\) )?(\[[^\]]*\]:\d+|[^\s()]+)(?:\((.*?)\))? --> (\S+) error: (.*)$`)

func parseDialFailure(text string) (dialFailure, bool) {
	g := dialFailRe.FindStringSubmatch(strings.TrimSpace(text))
	if g == nil {
		return dialFailure{}, false
	}
	f := dialFailure{
		Network: strings.ToLower(g[1]),
		Group:   g[2],
		Rule:    g[3],
		Payload: g[4],
		Source:  g[5],
		Process: g[6],
		Host:    g[7],
		Error:   g[8],
	}
	if i := strings.Index(f.Process, ", uid="); i >= 0 {
		f.Process = f.Process[:i]
	} else if strings.HasPrefix(f.Process, "uid=") {
		f.Process = ""
	}
	if h, p, err := net.SplitHostPort(g[7]); err == nil {
		f.Host = h
		f.Port, _ = strconv.Atoi(p)
	}
	return f, true
}

// addLog keeps one line of the core's log and turns failed dials into
// failed connections and events.
func (m *Monitor) addLog(l mihomo.Log, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	m.logs = push(m.logs, Log{Seq: m.seq, Time: now, Level: l.Type, Text: l.Payload}, maxLogs)

	if l.Type != "warning" && l.Type != "error" {
		return
	}
	f, ok := parseDialFailure(l.Payload)
	if !ok || (m.SkipGroup != "" && f.Group == m.SkipGroup) {
		return
	}
	// mihomo retries a TCP dial several times within a few seconds, each
	// logged alike; the client's source port tells one dial from another.
	for k, t := range m.recentFail {
		if now.Sub(t) > time.Minute {
			delete(m.recentFail, k)
		}
	}
	key := f.Network + "|" + f.Source + "|" + f.Host + ":" + strconv.Itoa(f.Port) + "|" + f.Group
	if _, dup := m.recentFail[key]; dup {
		m.recentFail[key] = now
		return
	}
	m.recentFail[key] = now

	m.failSeq++
	proc := m.paths[f.Process]
	if f.Process == "" && m.FindProcess != nil {
		// the client may still be waiting on its socket
		if src, err := netip.ParseAddrPort(f.Source); err == nil {
			proc = m.FindProcess(f.Network, src)
		}
	}
	app := appName(f.Process, proc)
	c := Conn{
		ID:      "failed-" + strconv.FormatInt(m.failSeq, 10),
		App:     app,
		Process: proc,
		Host:    f.Host,
		Port:    f.Port,
		Network: f.Network,
		Rule:    f.Rule,
		Payload: f.Payload,
		Start:   now,
		End:     now,
		Status:  "failed",
		Error:   f.Error,
	}
	if net.ParseIP(f.Host) != nil {
		c.IP = f.Host
	}
	if f.Group != "" {
		c.Chains = []string{f.Group}
	}
	m.closed = push(m.closed, c, maxClosed)
	m.events = push(m.events, Event{Time: now, App: app, Host: f.Host, Chains: c.Chains, Failed: true, Error: f.Error}, maxEvents)
}
