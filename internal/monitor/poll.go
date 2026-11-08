package monitor

import (
	"net"
	"net/netip"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/mihomo"
)

// unknownApp is the app of a connection whose process no one knows.
func unknownApp() string { return i18n.T("未知", "Unknown") }

// apply folds one answer of GET /connections, taken at now, into the
// monitor: per-connection rates from the deltas since the previous poll,
// connections that went away become closed, new ones become events, and
// the controller's totals feed today's and this month's traffic.
func (m *Monitor) apply(s mihomo.Snapshot, now time.Time) {
	procs := m.findProcesses(s)
	m.mu.Lock()
	defer m.mu.Unlock()

	first := m.lastPoll.IsZero()
	var secs float64
	if !first {
		secs = now.Sub(m.lastPoll).Seconds()
	}
	rate := func(delta int64) int64 {
		if secs <= 0 || delta <= 0 {
			return 0
		}
		return int64(float64(delta)/secs + 0.5)
	}

	var sumDown, sumUp int64
	groups := map[string]int64{}
	seen := make(map[string]bool, len(s.Connections))
	for _, mc := range s.Connections {
		if mc.ID == "" || seen[mc.ID] {
			continue
		}
		seen[mc.ID] = true
		if m.SkipGroup != "" && len(mc.Chains) > 0 && mc.Chains[len(mc.Chains)-1] == m.SkipGroup {
			continue
		}
		t, ok := m.active[mc.ID]
		if !ok {
			t = &tracked{Conn: convert(mc)}
			if p := procs[mc.ID]; p != "" {
				t.Process, t.App = p, appName("", p)
			}
			if first {
				// Bytes from before the monitor looked are not a rate.
				t.pollDown, t.pollUp = mc.Download, mc.Upload
			}
			m.active[mc.ID] = t
			if t.Process != "" && len(m.paths) < 500 {
				m.paths[path.Base(t.Process)] = t.Process
			}
			when := t.Start
			if when.IsZero() {
				when = now
			}
			m.events = push(m.events, Event{Time: when, App: t.App, Host: t.Host, Chains: t.Chains}, maxEvents)
		} else {
			// Chains can change (e.g. a url-test group switching), so take
			// the latest.
			t.Chains = mc.Chains
		}
		dDown := max(mc.Download-t.pollDown, 0)
		dUp := max(mc.Upload-t.pollUp, 0)
		t.Down, t.Up = mc.Download, mc.Upload
		t.pollDown, t.pollUp = mc.Download, mc.Upload
		t.DownRate, t.UpRate = rate(dDown), rate(dUp)
		sumDown += dDown
		sumUp += dUp
		groups[group(t.Chains)] += t.DownRate
		m.groupBytes[group(t.Chains)] += dDown
	}
	for id, t := range m.active {
		if !seen[id] {
			m.closeLocked(t, now)
			delete(m.active, id)
		}
	}
	m.down, m.up = rate(sumDown), rate(sumUp)
	m.byGroup = groups
	m.lastPoll = now

	// Totals: the controller counts since the core started, so a total
	// that went down means a new core and the whole new value is traffic.
	dDown, dUp := s.DownloadTotal, s.UploadTotal
	if m.totals {
		if s.DownloadTotal >= m.lastDown {
			dDown = s.DownloadTotal - m.lastDown
		}
		if s.UploadTotal >= m.lastUp {
			dUp = s.UploadTotal - m.lastUp
		}
	}
	m.totals = true
	m.lastDown, m.lastUp = s.DownloadTotal, s.UploadTotal
	m.addTotalsLocked(dDown, dUp, now)
}

// findProcesses names the processes of the connections in s that are new
// and whose process the core does not know, by id. The lookup reads the
// system's whole socket list, so it runs with the monitor unlocked: the
// interface may be reading it meanwhile. Only the poller calls apply, so
// the connections new here are still new there.
func (m *Monitor) findProcesses(s mihomo.Snapshot) map[string]string {
	if m.FindProcess == nil {
		return nil
	}
	type want struct {
		id, network string
		src         netip.AddrPort
	}
	var ask []want
	unknown := unknownApp()
	m.mu.Lock()
	for _, mc := range s.Connections {
		if _, ok := m.active[mc.ID]; ok || mc.ID == "" {
			continue
		}
		if m.SkipGroup != "" && len(mc.Chains) > 0 && mc.Chains[len(mc.Chains)-1] == m.SkipGroup {
			continue // apply does not keep them
		}
		if c := convert(mc); c.Process == "" && c.App == unknown {
			src, _ := netip.ParseAddrPort(net.JoinHostPort(mc.Metadata.SourceIP, mc.Metadata.SourcePort))
			ask = append(ask, want{mc.ID, c.Network, src})
		}
	}
	m.mu.Unlock()
	if len(ask) == 0 {
		return nil
	}
	out := make(map[string]string, len(ask))
	for _, w := range ask {
		out[w.id] = m.FindProcess(w.network, w.src)
	}
	return out
}

func (m *Monitor) closeLocked(t *tracked, now time.Time) {
	c := t.Conn
	c.Status = "closed"
	c.End = now
	c.DownRate, c.UpRate = 0, 0
	m.closed = push(m.closed, c, maxClosed)
}

// pollFailed is called when the controller did not answer: nothing moves
// until it does.
func (m *Monitor) pollFailed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.down, m.up = 0, 0
	for _, t := range m.active {
		t.DownRate, t.UpRate = 0, 0
	}
	for g := range m.byGroup {
		m.byGroup[g] = 0
	}
}

// stop ends a Run: the core's connections are gone (or no longer watched).
func (m *Monitor) stop(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, t := range m.active {
		m.closeLocked(t, now)
		delete(m.active, id)
	}
	m.down, m.up = 0, 0
	m.byGroup = map[string]int64{}
	m.lastPoll = time.Time{}
}

func group(chains []string) string {
	if len(chains) == 0 || chains[len(chains)-1] == "" {
		return "DIRECT"
	}
	return chains[len(chains)-1]
}

func convert(mc mihomo.Connection) Conn {
	md := mc.Metadata
	c := Conn{
		ID:      mc.ID,
		App:     appName(md.Process, md.ProcessPath),
		Process: md.ProcessPath,
		Host:    firstNonEmpty(md.Host, md.SniffHost, md.DestinationIP),
		IP:      md.DestinationIP,
		Network: strings.ToLower(md.Network),
		Rule:    mc.Rule,
		Payload: mc.RulePayload,
		Chains:  mc.Chains,
		Start:   mc.Start,
		Status:  "active",
		Source:  md.SourceIP,
	}
	c.Port, _ = strconv.Atoi(md.DestinationPort)
	return c
}

// appName is what the interface calls a process: the app bundle it is
// part of ("Claude" for …/Claude.app/…/Claude Helper.app/…/Claude Helper),
// else its name.
func appName(process, processPath string) string {
	// mihomo reports POSIX paths on macOS; also handle Windows ones.
	p := strings.ReplaceAll(processPath, `\`, "/")
	if i := strings.Index(p, ".app/"); i > 0 {
		return path.Base(p[:i])
	}
	if process != "" {
		return process
	}
	if p != "" {
		return path.Base(p)
	}
	return unknownApp()
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
