package main

import (
	"context"
	"errors"
	"net/netip"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oboo/moho/internal/gateway"
	"github.com/oboo/moho/internal/settings"
)

// A running server must notice a different router even if its interface,
// static address and prefix stayed the same. Returning home needs a new check.
func TestGatewayAutoStopsOnRouterChange(t *testing.T) {
	for _, change := range []string{"hardware address", "router address", "missing router"} {
		t.Run(change, func(t *testing.T) {
			a, l, f := autoApp(t)
			if err := a.setGatewayAuto(true); err != nil || !a.serving() {
				t.Fatalf("initial start: serving %v, error %v", a.serving(), err)
			}
			home, before := f.net, l.asked()
			switch change {
			case "hardware address":
				l.set(func(l *lanFake) {
					l.neigh = map[netip.Addr]string{netip.MustParseAddr(home.Router): "c0:00:00:00:00:09"}
				})
			case "router address":
				f.net.Router = "192.168.1.2"
			case "missing router":
				f.net.Router = ""
			}
			a.auto.neighMu.Lock()
			a.auto.neighAt = time.Time{}
			a.auto.neighMu.Unlock()
			a.syncGateway()
			a.tendAuto()
			v := a.autoView(a.settings.Get(), true)
			t.Logf("changed %s: serving=%v status=%s extra probes=%d", change, a.serving(), v.Status, l.asked()-before)
			if a.serving() || v.Status != "error" || v.Detail == "" {
				t.Fatalf("kept serving after router changed: %+v", v)
			}
			if l.asked() != before {
				t.Fatal("probed a LAN that was not approved")
			}

			f.net = home
			l.set(func(l *lanFake) {
				l.neigh = map[netip.Addr]string{netip.MustParseAddr(home.Router): "a4:00:00:00:00:01"}
			})
			a.auto.neighMu.Lock()
			a.auto.neighAt = time.Time{}
			a.auto.neighMu.Unlock()
			a.syncGateway()
			settle(t, a, true)
			if l.asked() != before+1 {
				t.Fatalf("returning home asked %d times, want a fresh check", l.asked()-before)
			}
		})
	}
}

// Hold the network read after a successful resume probe. Each stop must
// finish while that read waits, and the old startup must not undo it.
func TestGatewayAutoPendingStartDoesNotUndoStop(t *testing.T) {
	for _, stop := range []string{"off", "manual", "pause", "quit"} {
		t.Run(stop, func(t *testing.T) {
			a, _, f := autoApp(t)
			if err := a.setGatewayAuto(true); err != nil || !a.serving() {
				t.Fatalf("initial start: serving %v, error %v", a.serving(), err)
			}
			setPaused(t, a, true)
			if _, err := a.settings.Update(func(s *settings.Settings) { s.Paused = false }); err != nil {
				t.Fatal(err)
			}
			self, lan, _ := osDHCP.lan(f.net)
			key := lanKey(f.net, self, lan)
			readLAN, probe := osDHCP.lan, osDHCP.probe
			var afterProbe atomic.Bool
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			osDHCP.probe = func(ctx context.Context, iface string, self netip.Addr, wait time.Duration) ([]netip.Addr, error) {
				others, err := probe(ctx, iface, self, wait)
				afterProbe.Store(true)
				return others, err
			}
			osDHCP.lan = func(n gateway.Network) (netip.Addr, netip.Prefix, bool) {
				if afterProbe.CompareAndSwap(true, false) {
					close(entered)
					<-release
				}
				return readLAN(n)
			}
			go func() { defer close(done); a.clearToServe(key) }()
			defer func() { unblock(); <-done }()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("startup did not reach the post-probe network read")
			}
			t.Log("resume probe completed; startup is waiting in the network read")
			switch stop {
			case "off":
				if _, err := a.settings.Update(func(s *settings.Settings) { wantGateway(s, false) }); err != nil {
					t.Fatal(err)
				}
				a.syncGateway()
			case "manual":
				if err := a.setGatewayAuto(false); err != nil {
					t.Fatal(err)
				}
			case "pause":
				setPaused(t, a, true)
			case "quit":
				a.mu.Lock()
				a.quitting = true
				a.mu.Unlock()
				a.stopDHCP()
			}
			t.Logf("%s completed before releasing startup", stop)
			unblock()
			<-done
			if a.serving() {
				s := a.settings.Get()
				t.Fatalf("startup undid %s: gateway=%v automatic=%v paused=%v", stop, s.Gateway, s.GatewayAuto, s.Paused)
			}
		})
	}
}

// Turning automatic off while its initial probe runs must also cancel the
// pending choice, so a later gateway enable or resume cannot revive it.
func TestGatewayAutoPendingEnableDoesNotUndoStop(t *testing.T) {
	for _, stop := range []string{"off", "manual", "pause"} {
		t.Run(stop, func(t *testing.T) {
			a, l, _ := autoApp(t)
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			l.set(func(l *lanFake) {
				l.probing = func(netip.Addr) []netip.Addr {
					close(entered)
					<-release
					return nil
				}
			})
			go func() { defer close(done); _ = a.setGatewayAuto(true) }()
			defer func() { unblock(); <-done }()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("initial probe did not start")
			}
			switch stop {
			case "off":
				if _, err := a.settings.Update(func(s *settings.Settings) { wantGateway(s, false) }); err != nil {
					t.Fatal(err)
				}
				a.syncGateway()
			case "manual":
				if err := a.setGatewayAuto(false); err != nil {
					t.Fatal(err)
				}
			case "pause":
				setPaused(t, a, true)
			}
			t.Logf("%s completed while initial enable probe waits", stop)
			unblock()
			<-done
			s := a.settings.Get()
			if a.serving() || s.GatewayAuto || s.GatewayAutoLAN != "" {
				t.Fatalf("pending enable undid %s: serving=%v automatic=%v LAN=%q", stop, a.serving(), s.GatewayAuto, s.GatewayAutoLAN)
			}
		})
	}
}

// A probe that finishes after pausing must leave neither clearance for a
// later resume nor an obsolete startup error behind.
func TestGatewayAutoStoppedProbeCannotClearLAN(t *testing.T) {
	for _, answer := range []string{"quiet", "other server", "error"} {
		t.Run(answer, func(t *testing.T) {
			a, _, f := autoApp(t)
			if err := a.setGatewayAuto(true); err != nil || !a.serving() {
				t.Fatalf("initial start: serving %v, error %v", a.serving(), err)
			}
			setPaused(t, a, true)
			if _, err := a.settings.Update(func(s *settings.Settings) { s.Paused = false }); err != nil {
				t.Fatal(err)
			}
			self, lan, _ := osDHCP.lan(f.net)
			key := lanKey(f.net, self, lan)
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			osDHCP.probe = func(context.Context, string, netip.Addr, time.Duration) ([]netip.Addr, error) {
				close(entered)
				<-release
				switch answer {
				case "other server":
					return []netip.Addr{netip.MustParseAddr("192.168.1.1")}, nil
				case "error":
					return nil, errors.New("probe failed")
				default:
					return nil, nil
				}
			}
			go func() { defer close(done); a.clearToServe(key) }()
			defer func() { unblock(); <-done }()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("resume probe did not start")
			}
			setPaused(t, a, true)
			t.Logf("pause completed before probe returned %s", answer)
			unblock()
			<-done
			a.auto.mu.Lock()
			clear, err := a.auto.clear, a.auto.err
			a.auto.mu.Unlock()
			if a.serving() || clear != "" || err != nil {
				t.Fatalf("stopped probe changed startup state: serving=%v clearance=%q error=%v", a.serving(), clear, err)
			}
		})
	}
}

// A start for en1 must not accept an answer obtained on en0, as can happen
// when it joins an earlier check. The next check on en1 can start it.
func TestGatewayAutoResumeKeepsProbeLAN(t *testing.T) {
	a, l, f := autoApp(t)
	if err := a.setGatewayAuto(true); err != nil {
		t.Fatal(err)
	}
	setPaused(t, a, true)
	if _, err := a.settings.Update(func(s *settings.Settings) { s.Paused = false }); err != nil {
		t.Fatal(err)
	}
	next := f.net
	next.Iface = "en1"
	self, lan, _ := osDHCP.lan(next)
	key, before := lanKey(next, self, lan), l.asked()
	l.set(func(l *lanFake) {
		l.probing = func(netip.Addr) []netip.Addr {
			f.net = next
			return nil
		}
	})
	a.clearToServe(key)
	if a.serving() {
		t.Fatal("en1 started with the answer from en0")
	}
	l.set(func(l *lanFake) { l.probing = nil })
	a.tendAuto()
	settle(t, a, true)
	if l.asked() != before+2 {
		t.Fatalf("asked %d times, want the discarded check and a fresh one on en1", l.asked()-before)
	}
}

// The approval belongs to the network that was actually probed, including
// the interface and router identity, not whichever LAN is there afterward.
func TestGatewayAutoEnableDoesNotReuseOtherLANProbe(t *testing.T) {
	for _, change := range []string{"interface", "subnet", "router"} {
		t.Run(change, func(t *testing.T) {
			a, l, f := autoApp(t)
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			osDHCP.probe = func(ctx context.Context, iface string, self netip.Addr, wait time.Duration) ([]netip.Addr, error) {
				close(entered)
				<-release
				return nil, nil
			}
			var enableErr error
			go func() { defer close(done); enableErr = a.setGatewayAuto(true) }()
			defer func() { unblock(); <-done }()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("initial probe did not start")
			}
			t.Logf("probe is waiting on %s; changing %s", f.net.Iface, change)
			switch change {
			case "interface":
				f.net.Iface = "en1"
			case "subnet":
				l.set(func(l *lanFake) {
					l.self, l.lan = netip.MustParseAddr("10.0.0.37"), netip.MustParsePrefix("10.0.0.0/24")
					l.neigh = map[netip.Addr]string{netip.MustParseAddr("10.0.0.1"): "b8:00:00:00:00:01"}
				})
				f.net = gateway.Network{IP: "10.0.0.37", Iface: "en1", Router: "10.0.0.1"}
			case "router":
				l.set(func(l *lanFake) {
					l.neigh = map[netip.Addr]string{netip.MustParseAddr("192.168.1.1"): "c0:00:00:00:00:09"}
				})
			}
			a.auto.neighMu.Lock()
			a.auto.neighAt = time.Time{}
			a.auto.neighMu.Unlock()
			unblock()
			<-done
			s := a.settings.Get()
			t.Logf("old probe completed: serving=%v approved LAN=%q error=%v", a.serving(), s.GatewayAutoLAN, enableErr)
			if a.serving() || s.GatewayAuto || s.GatewayAutoLAN != "" || enableErr == nil {
				t.Fatal("the old network's probe approved the changed network")
			}
		})
	}
}

// Any successful config reload reconciles the gateway. While automatic is
// still being checked, that is not a request to cancel its initial enable.
func TestGatewayAutoReconcileKeepsPendingEnable(t *testing.T) {
	a, l, _ := autoApp(t)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	l.set(func(l *lanFake) {
		l.probing = func(netip.Addr) []netip.Addr { close(entered); <-release; return nil }
	})
	var enableErr error
	go func() { defer close(done); enableErr = a.setGatewayAuto(true) }()
	defer func() { unblock(); <-done }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("initial enable did not reach the probe")
	}
	a.syncGateway()
	unblock()
	<-done
	if enableErr != nil || !a.serving() || !a.settings.Get().GatewayAuto {
		t.Fatalf("reconciliation canceled enable: error=%v serving=%v automatic=%v", enableErr, a.serving(), a.settings.Get().GatewayAuto)
	}
}

// The CLI must reach the cancellation even before the pending enable has
// saved GatewayAuto. Exercise both commands through the actual transport.
func TestGatewayAutoManualCLIStopsPendingEnable(t *testing.T) {
	a, _, run := cliApp(t)
	a.emitQueued = true
	installed, dialogs := true, 0
	fakeTun(t, &installed, &dialogs)
	l := fakeLAN(t)
	f := &gwFake{ready: true, net: gateway.Network{IP: "192.168.1.20", Iface: "en0", Router: "192.168.1.1"}}
	a.gw = gateway.New(f, filepath.Join(a.dataDir, "gateway-before.json"))
	if _, err := a.settings.Update(func(s *settings.Settings) {
		s.Gateway, s.Tun, s.Paused, s.SystemProxy = true, true, false, false
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.stopDHCP)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	l.set(func(l *lanFake) {
		l.probing = func(netip.Addr) []netip.Addr { close(entered); <-release; return nil }
	})
	var enableCode int
	go func() { defer close(done); enableCode, _, _ = run("", "gateway", "auto") }()
	defer func() { unblock(); <-done }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("CLI enable did not reach the probe")
	}
	if code, out, errOut := run("", "gateway", "manual"); code != 0 {
		t.Fatalf("manual failed: exit=%d stdout=%q stderr=%q", code, out, errOut)
	}
	unblock()
	<-done
	if a.serving() || a.settings.Get().GatewayAuto || enableCode == 0 {
		t.Fatalf("manual was undone: serving=%v automatic=%v enable exit=%d", a.serving(), a.settings.Get().GatewayAuto, enableCode)
	}
}

// Completion of a superseded request must not hide the newer enable from
// a manual request. The UI's shared busy flag cannot identify its owner.
func TestGatewayAutoManualCancelsNewestEnable(t *testing.T) {
	a, _, run := cliApp(t)
	a.emitQueued = true
	installed, dialogs := true, 0
	fakeTun(t, &installed, &dialogs)
	l := fakeLAN(t)
	f := &gwFake{ready: true, net: gateway.Network{IP: "192.168.1.20", Iface: "en0", Router: "192.168.1.1"}}
	a.gw = gateway.New(f, filepath.Join(a.dataDir, "gateway-before.json"))
	if _, err := a.settings.Update(func(s *settings.Settings) {
		s.Gateway, s.Tun, s.Paused, s.SystemProxy = true, true, false, false
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.stopDHCP)
	firstRead, releaseFirst := make(chan struct{}), make(chan struct{})
	secondProbe, releaseSecond := make(chan struct{}), make(chan struct{})
	firstDone, secondDone := make(chan struct{}), make(chan struct{})
	var firstOnce, secondOnce sync.Once
	unblockFirst := func() { firstOnce.Do(func() { close(releaseFirst) }) }
	unblockSecond := func() { secondOnce.Do(func() { close(releaseSecond) }) }
	readLAN := osDHCP.lan
	var reads, probes atomic.Int32
	osDHCP.lan = func(n gateway.Network) (netip.Addr, netip.Prefix, bool) {
		if reads.Add(1) == 2 { // the first enable's post-probe LAN read
			close(firstRead)
			<-releaseFirst
		}
		return readLAN(n)
	}
	l.set(func(l *lanFake) {
		l.probing = func(netip.Addr) []netip.Addr {
			if probes.Add(1) == 2 {
				close(secondProbe)
				<-releaseSecond
			}
			return nil
		}
	})
	wait := func(ch <-chan struct{}, reason string) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(3 * time.Second):
			t.Fatal(reason)
		}
	}
	var firstCode, secondCode int
	go func() { defer close(firstDone); firstCode, _, _ = run("", "gateway", "auto") }()
	defer func() { unblockFirst(); <-firstDone }()
	wait(firstRead, "first enable did not reach its post-probe LAN read")
	go func() { defer close(secondDone); secondCode, _, _ = run("", "gateway", "auto") }()
	defer func() { unblockSecond(); <-secondDone }()
	wait(secondProbe, "second enable did not reach its probe")
	unblockFirst()
	wait(firstDone, "superseded enable did not finish")
	if code, out, errOut := run("", "gateway", "manual"); code != 0 {
		t.Fatalf("manual failed: exit=%d stdout=%q stderr=%q", code, out, errOut)
	}
	unblockSecond()
	wait(secondDone, "second enable did not finish")
	if a.serving() || a.settings.Get().GatewayAuto || firstCode == 0 || secondCode == 0 {
		t.Fatalf("manual lost to overlapping enables: serving=%v automatic=%v exits=%d/%d", a.serving(), a.settings.Get().GatewayAuto, firstCode, secondCode)
	}
}
