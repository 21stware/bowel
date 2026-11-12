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
