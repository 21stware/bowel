package main

import (
	"context"
	"errors"
	"fmt"
	"log"
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

// noteTunInstall puts how TUN's installation went among the logs, where
// there is something to say: why it failed (on Windows, the part of the
// elevated command that stopped and what Windows said), or what an
// installation that was made is without. A prompt the user dismissed is
// not a failure.
func (a *App) noteTunInstall(err error) {
	text := ""
	switch {
	case err == nil:
		text = osTun.note(a.tunBin)
	case !errors.Is(err, tun.ErrCanceled):
		text = err.Error()
	}
	if text == "" {
		return
	}
	log.Println("tun: installation:", text)
	a.mon.Note("bowel: tun: " + text)
}

// tunWanted is whether the config the core runs has TUN in it: wanted,
// taken over, installed, and not given up on.
func (a *App) tunWanted(s settings.Settings) bool {
	return s.Tun && a.takeoverOn(s) && osTun.installed(a.tunBin) && !a.tunGone.Load() && a.tunFault.Load() == nil
}

// useTunCore makes the next config run on the copy TUN's installation
// made when TUN is in it and the core is another: the bundled one, which
// has no right to make the device. That is the core of an app that
// started before the installation was there or was valid (issue 117: a
// mark that became valid while the app ran), and on Windows it is not the
// one the task starts. A core that is running is stopped, and applying
// the config starts the right one.
func (a *App) useTunCore(s settings.Settings) {
	if a.tunBin == "" || a.tunBin == a.coreBin || !a.tunWanted(s) || a.core.BinPath() == a.tunBin {
		return
	}
	log.Println("tun: moving to the installed core, the running one cannot make the device")
	a.core.Stop()
	a.core.SetBin(a.tunBin)
}

// Whether TUN is on is asked of the core, not read off the settings: a
// core that cannot bring the device up (no right to: "configure tun
// interface: Access is denied") logs one line and goes on without it,
// its controller answering as usual. Two things tell of it: that line,
// when the app was reading the core's log as it was printed, and the
// core's own account of its TUN listener (GET /configs), asked every few
// seconds while TUN is meant to be on.

// tunUpProbe is what the core last said of its TUN device, for one run
// of the core.
type tunUpProbe struct {
	started time.Time // the core's start this is about
	since   time.Time // when TUN was first seen wanted in that run
	at      time.Time // the last answer; zero: none yet
	down    int       // answers in a row that said the device is not up
	busy    bool
}

const (
	// tunUpSettle is how long the core has, from its start or from TUN
	// being turned on, before an answer counts: its controller answers
	// before the device is made.
	tunUpSettle = 5 * time.Second
	tunUpEvery  = 4 * time.Second
	// tunUpLooks is how many answers in a row must say it is down: the
	// core takes the device down and up again when its config changes.
	tunUpLooks = 2
)

// coreTunUp asks the running core. Tests replace it.
var coreTunUp = func(m *core.Manager, ctx context.Context) (up, known bool) { return m.TunUp(ctx) }

// tunProbed is whether there is a device to ask about: TUN in the config
// of a core that runs and has taken its config in.
func (a *App) tunProbed(s settings.Settings, cs core.Status) bool {
	return a.tunWanted(s) && cs.State == core.Running && !a.core.Loading()
}

// probeTunUp asks in the background, one question at a time.
func (a *App) probeTunUp() {
	s, cs, now := a.settings.Get(), a.core.Status(), time.Now()
	wanted := a.tunProbed(s, cs)
	a.mu.Lock()
	if !wanted {
		a.tunUp = tunUpProbe{busy: a.tunUp.busy}
		a.mu.Unlock()
		return
	}
	if a.tunUp.since.IsZero() || !a.tunUp.started.Equal(cs.StartedAt) {
		a.tunUp = tunUpProbe{started: cs.StartedAt, since: now, busy: a.tunUp.busy}
	}
	p := a.tunUp
	due := !p.busy && now.Sub(p.since) >= tunUpSettle && now.Sub(p.at) >= tunUpEvery
	if due {
		a.tunUp.busy = true
	}
	a.mu.Unlock()
	if !due {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		up, known := coreTunUp(a.core, ctx)
		cancel()
		if a.noteTunUp(p, up, known) {
			a.emit()
		}
	}()
}

// noteTunUp takes the core's answer to a question asked when the probe
// was p, and reports whether what is shown changed.
func (a *App) noteTunUp(p tunUpProbe, up, known bool) (changed bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	q := &a.tunUp
	q.busy = false
	if !q.since.Equal(p.since) || !q.started.Equal(p.started) || !known {
		return false // another run, or no answer: nothing is known
	}
	before := q.isDown()
	q.at = time.Now()
	if up {
		q.down = 0
	} else {
		q.down++
	}
	return before != q.isDown()
}

func (p tunUpProbe) isDown() bool { return p.down >= tunUpLooks }

// awaitTunUp waits, after TUN was turned on, for the core to say whether
// the device came up, so that whoever asked is answered with what
// happened: a few seconds at most, and no time when it is up at once.
func (a *App) awaitTunUp() {
	s, cs := a.settings.Get(), a.core.Status()
	if !a.tunProbed(s, cs) {
		return
	}
	a.mu.Lock()
	a.tunUp = tunUpProbe{started: cs.StartedAt, since: time.Now(), busy: a.tunUp.busy}
	p := a.tunUp
	a.mu.Unlock()
	for deadline := time.Now().Add(tunUpSettle); ; time.Sleep(250 * time.Millisecond) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		up, known := coreTunUp(a.core, ctx)
		cancel()
		if known && up {
			a.noteTunUp(p, true, true)
			return
		}
		if time.Now().After(deadline) {
			if known { // it had its time: this answer is believed at once
				for i := 0; i < tunUpLooks; i++ {
					a.noteTunUp(p, false, true)
				}
				a.emit()
			}
			return
		}
	}
}

// tunDown is why TUN, meant to be on, is not: "" when the core has it up
// or nothing says otherwise.
func (a *App) tunDown(cs core.Status) string {
	a.mu.Lock()
	p := a.tunUp
	a.mu.Unlock()
	asked := p.started.Equal(cs.StartedAt) && !p.at.IsZero()
	switch {
	case cs.State != core.Running:
		return ""
	case asked && p.down == 0:
		return "" // the core says it is up, whatever it logged before
	case cs.TunError != "":
		return tunDownText(cs.TunError)
	case asked && p.isDown():
		return tunDownText("")
	}
	return ""
}

// tunDownText is what the TUN entry says of a device that is not up;
// said is the core's own reason, when it was heard.
func tunDownText(said string) string {
	if said == "" {
		return i18n.T("内核没有把 TUN 设备建起来，流量没有被 TUN 接管；原因在 bowel logs 里找「Start TUN listening error」。关掉再打开 TUN 会重试",
			"The core didn't bring the TUN device up, so TUN isn't taking traffic; look for \"Start TUN listening error\" in bowel logs for why. Turning TUN off and on tries again")
	}
	return fmt.Sprintf(i18n.T("内核没有把 TUN 设备建起来，流量没有被 TUN 接管：%s。关掉再打开 TUN 会重试", "The core didn't bring the TUN device up, so TUN isn't taking traffic: %s. Turning TUN off and on tries again"), said)
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
