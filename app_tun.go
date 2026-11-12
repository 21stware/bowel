package main

import (
	"context"
	"time"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/settings"
	"github.com/oboo/moho/internal/tun"
)

// spawnTun starts the core the way TUN's installation needs where that is
// not as a child of the app: on Windows, through the scheduled task that
// runs the protected copy elevated. For any other core, and on the other
// systems always, it starts nothing and the manager does.
func spawnTun(bin string) (core.Process, error) {
	t, err := tun.Spawn(bin)
	if t == nil {
		return nil, err
	}
	return t, err
}

// With TUN on, Linux's systemd-resolved must be told that the TUN device
// is where lookups go: the core does that with resolvectl each time the
// device comes up, which polkit lets a user's process do only under the
// rule TUN's installation leaves (internal/tun/dnsrule_linux.go). Without
// it the device is up, every packet goes through it, and the system still
// asks the network's own servers: what does not use the proxy port is
// then routed by address alone, and no rule by domain applies to it
// (issue 117). Two things tell of it: the installation lacking the rule,
// which one authorization mends, and resolved having no servers for the
// device, which is the fault itself whatever caused it.

// tunDNSProbe is what the last look at resolved found, for one run of
// the core with TUN on.
type tunDNSProbe struct {
	started time.Time // the core's start this is about
	since   time.Time // when TUN was first seen on in that run
	at      time.Time // the last look; zero: none yet
	known   bool      // resolved answered for the device
	empty   int       // looks in a row that found it without servers
}

const (
	// tunDNSSettle is how long the core has to bring the device up and
	// make its resolvectl calls before the first look.
	tunDNSSettle = 15 * time.Second
	tunDNSEvery  = 10 * time.Second
	// tunDNSLooks is how many looks in a row must find no servers: a
	// device the core is just replacing has none for a moment.
	tunDNSLooks = 2
)

// probeTunDNS looks in the background, one look at a time, while TUN is
// on. It asks nothing where there is no systemd-resolved to ask.
func (a *App) probeTunDNS() {
	s := a.settings.Get()
	cs := a.core.Status()
	now := time.Now()
	wanted := a.takeoverOn(s) && s.Tun && cs.State == core.Running && a.tunFault.Load() == nil &&
		osTun.installed(a.tunBin) && !a.tunGone.Load()
	a.mu.Lock()
	if !wanted {
		a.tunDNS = tunDNSProbe{}
		a.mu.Unlock()
		return
	}
	if a.tunDNS.since.IsZero() || !a.tunDNS.started.Equal(cs.StartedAt) {
		a.tunDNS = tunDNSProbe{started: cs.StartedAt, since: now}
	}
	p := a.tunDNS
	due := !a.tunDNSBusy && now.Sub(p.since) >= tunDNSSettle && now.Sub(p.at) >= tunDNSEvery
	if due {
		a.tunDNSBusy = true
	}
	a.mu.Unlock()
	if !due {
		return
	}
	go func() {
		a.tellTunDevice()
		pointed, known := osTun.linkDNS(context.Background())
		a.mu.Lock()
		a.tunDNSBusy = false
		changed := false
		if q := &a.tunDNS; q.since.Equal(p.since) && q.started.Equal(p.started) { // still the run that was asked about
			before := [2]bool{q.unpointed(), q.pointed()}
			q.note(time.Now(), pointed, known)
			changed = before != [2]bool{q.unpointed(), q.pointed()}
		}
		a.mu.Unlock()
		if changed {
			a.emit()
		}
	}()
}

// note takes one look's answer.
func (p *tunDNSProbe) note(at time.Time, pointed, known bool) {
	p.at, p.known = at, known
	if known && !pointed {
		p.empty++
	} else {
		p.empty = 0
	}
}

// unpointed is whether resolved was seen, often enough to be believed,
// to have no servers for the TUN device.
func (p tunDNSProbe) unpointed() bool { return p.known && p.empty >= tunDNSLooks }

// pointed is whether the last look found servers there.
func (p tunDNSProbe) pointed() bool { return p.known && p.empty == 0 && !p.at.IsZero() }

// tunDNSState is what is known of the system's DNS with TUN on: noRule,
// that the installation lacks the rule and one authorization would write
// it; unpointed, that resolved has no servers for the device. A missing
// rule is nothing to tell of once the device is seen to have its servers
// (the user answered the system's own dialogs): turning TUN on next asks,
// as it always has.
func (a *App) tunDNSState(cs core.Status) (noRule, unpointed bool) {
	a.mu.Lock()
	p := a.tunDNS
	a.mu.Unlock()
	if !p.started.Equal(cs.StartedAt) {
		p = tunDNSProbe{}
	}
	if p.pointed() {
		return false, false
	}
	a.tellTunDevice()
	return osTun.needsRule(a.tunBin), p.unpointed()
}

// tunDNSDetail is what the TUN entry says while it is on.
func tunDNSDetail(noRule, unpointed bool) string {
	switch {
	case unpointed:
		return i18n.T("系统 DNS 没有指向 TUN，不走代理端口的流量不会按域名分流", "System DNS isn't pointed at TUN, so traffic that doesn't use the proxy port isn't routed by domain")
	case noRule:
		return i18n.T("缺少让 TUN 设置系统 DNS 的授权，不走代理端口的流量可能不会按域名分流", "The authorization that lets TUN set the system DNS is missing, so traffic that doesn't use the proxy port may not be routed by domain")
	}
	return ""
}

// tunDNSIssue is the news line for it. Nothing is broken that worked
// before, so the light stays green, as for a stale core.
func (a *App) tunDNSIssue(s settings.Settings, cs core.Status, tk Takeover) *Issue {
	if !s.Tun || tk.Tun.Status != "on" {
		return nil
	}
	noRule, unpointed := a.tunDNSState(cs)
	what := i18n.T("域名解析没有经过 bowel，所以按域名的规则对不走代理端口的流量不生效（走代理端口的不受影响）。", "Lookups don't go through bowel, so rules by domain don't apply to traffic that doesn't use the proxy port (what uses the port is not affected).")
	fix := i18n.T("这次安装缺少让 TUN 设置系统 DNS 的授权（bowel 0.4.0 起需要）。授权一次就能补上，网络会中断几秒；没有窗口时运行 bowel tun on。", "This installation lacks the authorization that lets TUN set the system DNS (needed since bowel 0.4.0). One authorization adds it; the network drops for a few seconds. With no window, run bowel tun on.")
	switch {
	case noRule && unpointed:
		return &Issue{Level: "warn", Text: i18n.T("系统 DNS 没有指向 TUN", "System DNS isn't pointed at TUN"), Detail: what + fix, Action: "tun-update", Label: i18n.T("授权", "Authorize")}
	case noRule:
		return &Issue{Level: "warn", Text: i18n.T("TUN 缺少设置系统 DNS 的授权", "TUN lacks the authorization to set the system DNS"),
			Detail: i18n.T("系统 DNS 多半没有指向 TUN：", "System DNS is most likely not pointed at TUN: ") + what + fix, Action: "tun-update", Label: i18n.T("授权", "Authorize")}
	case unpointed:
		return &Issue{Level: "warn", Text: i18n.T("系统 DNS 没有指向 TUN", "System DNS isn't pointed at TUN"),
			Detail: what + i18n.T("systemd-resolved 里 TUN 网卡没有 DNS 服务器：内核设置它的请求没有被系统接受。关掉再打开 TUN 会重试；系统弹出授权窗口时需要允许。", "systemd-resolved has no DNS servers for the TUN device: the system didn't accept the core's request to set them. Turning TUN off and on tries again; allow it if the system asks.")}
	}
	return nil
}
