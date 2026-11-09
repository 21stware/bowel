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

// dialFailure is what a log line of mihomo says about a connection that
// went nowhere: a failed dial, or a refusal.
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

// mihomo's tunnel logs what a rule refused, at level info, as
//
//	[TCP] 127.0.0.1:50567(curl) --> x.example:80 match DomainSuffix(example.com) using REJECT
//	[UDP] 127.0.0.1:53631 --> 192.0.2.7:53 match IPCIDR(192.0.2.0/24) using REJECT
//
// A group left with no node ends in REJECT too, but is logged by the
// group's name: only a rule aimed straight at REJECT matches here.
var rejectedRe = regexp.MustCompile(
	`^\[(TCP|UDP)\] (\[[^\]]*\]:\d+|[^\s()]+)(?:\((.*?)\))? --> (\S+) match ([^\s()]+)(?:\((.*)\))? using REJECT$`)

// Reject is the name of mihomo's refusing outbound.
const Reject = "REJECT"

func parseDialFailure(text string) (dialFailure, bool) {
	g := dialFailRe.FindStringSubmatch(strings.TrimSpace(text))
	if g == nil {
		return dialFailure{}, false
	}
	f := dialFailure{Network: strings.ToLower(g[1]), Group: g[2], Rule: g[3], Payload: g[4], Source: g[5], Error: g[8]}
	f.place(g[6], g[7])
	return f, true
}

// parseRejected reads a line saying a rule refused a connection; the
// result has no Error.
func parseRejected(text string) (dialFailure, bool) {
	g := rejectedRe.FindStringSubmatch(strings.TrimSpace(text))
	if g == nil {
		return dialFailure{}, false
	}
	f := dialFailure{Network: strings.ToLower(g[1]), Group: Reject, Rule: g[5], Payload: g[6], Source: g[2]}
	f.place(g[3], g[4])
	return f, true
}

// place fills in the process and the destination, as the log wrote them.
func (f *dialFailure) place(process, target string) {
	f.Process, f.Host = process, target
	if i := strings.Index(f.Process, ", uid="); i >= 0 {
		f.Process = f.Process[:i]
	} else if strings.HasPrefix(f.Process, "uid=") {
		f.Process = ""
	}
	if h, p, err := net.SplitHostPort(target); err == nil {
		f.Host = h
		f.Port, _ = strconv.Atoi(p)
	}
}

// ownerOf asks the system whose socket f came from, when the core didn't
// say. The client may still be waiting on it.
func (m *Monitor) ownerOf(f dialFailure) string {
	if f.Process != "" || m.FindProcess == nil {
		return ""
	}
	src, err := netip.ParseAddrPort(f.Source)
	if err != nil {
		return ""
	}
	return m.FindProcess(f.Network, src)
}

// addLog keeps one line of the core's log and turns failed dials into
// failed connections and events, and refusals into blocked connections.
func (m *Monitor) addLog(l mihomo.Log, now time.Time) {
	var blocked dialFailure
	var isBlocked bool
	var owner string
	if l.Type == "info" {
		if blocked, isBlocked = parseRejected(l.Payload); isBlocked {
			owner = m.ownerOf(blocked) // outside the lock: there can be many
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	m.logs = push(m.logs, Log{Seq: m.seq, Time: now, Level: l.Type, Text: l.Payload}, maxLogs)

	if isBlocked {
		if !m.repeated(blocked, now) {
			m.blocked = push(m.blocked, m.connOf(blocked, owner, "blocked", now), maxClosed)
		}
		return
	}
	if l.Type != "warning" && l.Type != "error" {
		return
	}
	f, ok := parseDialFailure(l.Payload)
	if !ok || (m.SkipGroup != "" && f.Group == m.SkipGroup) || m.repeated(f, now) {
		return
	}
	c := m.connOf(f, m.ownerOf(f), "failed", now)
	m.closed = push(m.closed, c, maxClosed)
	m.events = push(m.events, Event{Time: now, App: c.App, Host: f.Host, Chains: c.Chains, Failed: true, Error: f.Error}, maxEvents)
}

// repeated says whether f was logged moments ago: mihomo retries a TCP dial
// several times within a few seconds, each logged alike, and a UDP client
// sends again. The client's source port tells one attempt from another.
func (m *Monitor) repeated(f dialFailure, now time.Time) bool {
	for k, t := range m.recentFail {
		if now.Sub(t) > time.Minute {
			delete(m.recentFail, k)
		}
	}
	key := f.Network + "|" + f.Source + "|" + f.Host + ":" + strconv.Itoa(f.Port) + "|" + f.Group
	_, dup := m.recentFail[key]
	m.recentFail[key] = now
	return dup
}

// connOf is the connection a log line stands for, one that ended at once.
func (m *Monitor) connOf(f dialFailure, owner, status string, now time.Time) Conn {
	m.failSeq++
	proc := m.paths[f.Process]
	if owner != "" {
		proc = owner
	}
	c := Conn{
		ID:      status + "-" + strconv.FormatInt(m.failSeq, 10),
		App:     appName(f.Process, proc),
		Process: proc,
		Host:    f.Host,
		Port:    f.Port,
		Network: f.Network,
		Rule:    f.Rule,
		Payload: f.Payload,
		Start:   now,
		End:     now,
		Status:  status,
		Error:   f.Error,
	}
	if net.ParseIP(f.Host) != nil {
		c.IP = f.Host
	}
	if f.Group != "" {
		c.Chains = []string{f.Group}
	}
	return c
}
