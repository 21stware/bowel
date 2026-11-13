package main

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/devnet"
	"github.com/oboo/moho/internal/settings"
	"github.com/oboo/moho/internal/sysdns"
)

// fakeDNS is a machine's network services, in osDNS's place.
type fakeDNS struct {
	services []sysdns.Service
	writes   [][]sysdns.Service
	refuse   error // what a write answers
	deaf     bool  // a write is accepted and changes nothing
}

func (f *fakeDNS) install(t *testing.T) {
	saved := osDNS
	t.Cleanup(func() { osDNS = saved })
	osDNS.supported = true
	osDNS.snapshot = func(context.Context) (sysdns.Saved, error) {
		return sysdns.Saved{Services: slices.Clone(f.services)}, nil
	}
	osDNS.write = func(ctx context.Context, steps []sysdns.Service) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		f.writes = append(f.writes, steps)
		if f.refuse != nil || f.deaf {
			return f.refuse
		}
		for _, st := range steps {
			for i := range f.services {
				if f.services[i].Name == st.Name {
					f.services[i].Servers = st.Servers
				}
			}
		}
		return nil
	}
	osDNS.dhcp = func(context.Context, string) []string { return []string{"192.168.50.1"} }
}

func (f *fakeDNS) of(name string) []string {
	for _, sv := range f.services {
		if sv.Name == name {
			return sv.Servers
		}
	}
	return nil
}

func twoServices() *fakeDNS {
	return &fakeDNS{services: []sysdns.Service{
		{Name: "Wi-Fi", Device: "en1"},
		{Name: "Ethernet", Device: "en0", Servers: []string{"10.0.0.2"}},
	}}
}

// TestSystemDNSTakenAndPutBack: TUN on sets the placeholder on every
// service and keeps what each had; TUN off puts that back and drops the
// copy.
func TestSystemDNSTakenAndPutBack(t *testing.T) {
	f := twoServices()
	f.install(t)
	a := &App{dataDir: t.TempDir()}
	ctx := context.Background()

	if !a.syncDNS(ctx, true, true) {
		t.Fatal("nothing was written")
	}
	if !slices.Equal(f.of("Wi-Fi"), sysdns.Placeholder) || !slices.Equal(f.of("Ethernet"), sysdns.Placeholder) {
		t.Fatalf("services after the takeover: %+v", f.services)
	}
	saved, ok := a.readDNSBackup()
	if !ok || len(saved.Services) != 2 || len(saved.Services[0].Servers) != 0 || !slices.Equal(saved.Services[1].Servers, []string{"10.0.0.2"}) {
		t.Fatalf("the copy: %+v %v", saved, ok)
	}
	if a.dnsErr.Load() != nil || a.dnsHeld.Load() == nil {
		t.Error("the takeover is not recorded as done")
	}

	// Coming round again writes nothing; a service that appeared since is
	// taken too, and the first copy of the others stays.
	writes := len(f.writes)
	if a.syncDNS(ctx, true, true) || len(f.writes) != writes {
		t.Error("the second round wrote again")
	}
	f.services = append(f.services, sysdns.Service{Name: "Dock", Servers: []string{"9.9.9.9"}})
	if !a.syncDNS(ctx, true, true) || !slices.Equal(f.of("Dock"), sysdns.Placeholder) {
		t.Errorf("the new service was not taken: %+v", f.services)
	}
	saved, _ = a.readDNSBackup()
	if len(saved.Services) != 3 || !slices.Equal(saved.Services[1].Servers, []string{"10.0.0.2"}) || !slices.Equal(saved.Services[2].Servers, []string{"9.9.9.9"}) {
		t.Errorf("the copy after a new service: %+v", saved)
	}

	// The user sets one by hand meanwhile: that one is theirs now.
	f.services[2].Servers = []string{"8.8.8.8"}
	if !a.syncDNS(ctx, false, true) {
		t.Fatal("nothing was put back")
	}
	if len(f.of("Wi-Fi")) != 0 || !slices.Equal(f.of("Ethernet"), []string{"10.0.0.2"}) || !slices.Equal(f.of("Dock"), []string{"8.8.8.8"}) {
		t.Errorf("services after TUN went off: %+v", f.services)
	}
	if exists(a.dnsBackup()) || a.dnsHeld.Load() != nil {
		t.Error("the copy is still there")
	}
	writes = len(f.writes)
	if a.syncDNS(ctx, false, false) || len(f.writes) != writes {
		t.Error("with no copy left, the servers were written again")
	}
}

// TestSystemDNSWaitsForCore: nothing is set before the core, and its TUN,
// is there to answer.
func TestSystemDNSWaitsForCore(t *testing.T) {
	f := twoServices()
	f.install(t)
	a := &App{dataDir: t.TempDir()}
	if a.syncDNS(context.Background(), true, false) || len(f.writes) != 0 || exists(a.dnsBackup()) {
		t.Errorf("written before the core ran: %+v", f.writes)
	}
}

// TestSystemDNSAfterCrash: a launch that finds the copy of a run that
// never put the servers back does so, when TUN is not to be on; when it
// is, the first copy stays the copy.
func TestSystemDNSAfterCrash(t *testing.T) {
	left := `{"services":[{"name":"Wi-Fi","device":"en1","servers":null},{"name":"Ethernet","device":"en0","servers":["10.0.0.2"]}]}`
	held := func() *fakeDNS {
		f := twoServices()
		for i := range f.services {
			f.services[i].Servers = sysdns.Placeholder
		}
		return f
	}
	t.Run("TUN off", func(t *testing.T) {
		f := held()
		f.install(t)
		a := &App{dataDir: t.TempDir()}
		if err := os.WriteFile(a.dnsBackup(), []byte(left), 0o600); err != nil {
			t.Fatal(err)
		}
		a.syncDNS(context.Background(), false, false)
		if len(f.of("Wi-Fi")) != 0 || !slices.Equal(f.of("Ethernet"), []string{"10.0.0.2"}) || exists(a.dnsBackup()) {
			t.Errorf("not put back: %+v", f.services)
		}
	})
	t.Run("TUN on", func(t *testing.T) {
		f := held()
		f.install(t)
		a := &App{dataDir: t.TempDir()}
		if err := os.WriteFile(a.dnsBackup(), []byte(left), 0o600); err != nil {
			t.Fatal(err)
		}
		if a.syncDNS(context.Background(), true, true) {
			t.Error("written again though the placeholder is set")
		}
		saved, _ := a.readDNSBackup()
		if !slices.Equal(saved.Services[1].Servers, []string{"10.0.0.2"}) || len(saved.Services[0].Servers) != 0 {
			t.Errorf("the first copy was replaced: %+v", saved)
		}
	})
	t.Run("copy unreadable", func(t *testing.T) {
		f := held()
		f.install(t)
		a := &App{dataDir: t.TempDir()}
		if err := os.WriteFile(a.dnsBackup(), []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		a.syncDNS(context.Background(), false, false)
		if len(f.of("Wi-Fi")) != 0 || len(f.of("Ethernet")) != 0 || exists(a.dnsBackup()) {
			t.Errorf("the placeholder was left behind: %+v", f.services)
		}
	})
}

// TestQuitSystemDNS: quitting puts the servers back; when time runs out
// first the copy stays, for the next launch.
func TestQuitSystemDNS(t *testing.T) {
	f := twoServices()
	f.install(t)
	a := &App{dataDir: t.TempDir()}
	a.syncDNS(context.Background(), true, true)

	late, cancel := context.WithCancel(context.Background())
	cancel()
	a.quitSystemDNS(late)
	if !exists(a.dnsBackup()) || !slices.Equal(f.of("Wi-Fi"), sysdns.Placeholder) {
		t.Fatal("out of time, and the copy is gone")
	}
	a.quitSystemDNS(context.Background())
	if exists(a.dnsBackup()) || len(f.of("Wi-Fi")) != 0 || !slices.Equal(f.of("Ethernet"), []string{"10.0.0.2"}) {
		t.Errorf("not put back on quit: %+v", f.services)
	}
	// Quitting is final: nothing takes the servers again.
	a.quitting = true
	a.applySystemDNS()
	if len(f.of("Wi-Fi")) != 0 {
		t.Error("taken again while quitting")
	}
}

// dnsApp is an app whose TUN is on, for the status to judge.
func dnsApp(t *testing.T, f *fakeDNS) *App {
	t.Helper()
	f.install(t)
	installed, dialogs := true, 0
	fakeTun(t, &installed, &dialogs)
	st, err := settings.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Update(func(s *settings.Settings) { s.Tun, s.Paused = true, false }); err != nil {
		t.Fatal(err)
	}
	return &App{dataDir: t.TempDir(), settings: st, tunBin: "/tun"}
}

func lanEnv(a *App) {
	a.dev.env = devnet.Env{DefaultDNS: []string{"192.168.50.1"}, PrimaryIface: "en1"}
	a.dev.at, a.dev.dnsOutside = time.Now(), true
}

// TestDNSIssue: the status says so when names are resolved past TUN, and
// only then.
func TestDNSIssue(t *testing.T) {
	running := core.Status{State: core.Running}
	t.Run("refused", func(t *testing.T) {
		f := twoServices()
		f.refuse = errors.New("networksetup -setdnsservers: ** Error: Command requires admin privileges.")
		a := dnsApp(t, f)
		lanEnv(a)
		a.syncDNS(context.Background(), true, true)
		is := a.dnsIssue(a.settings.Get(), running)
		if is == nil || is.Action != "take-dns" || !strings.Contains(is.Detail, "admin privileges") {
			t.Fatalf("issue = %+v", is)
		}
		if st, got := a.judge(true, a.settings.Get(), running, Takeover{}, Snapshot{Nodes: []NodeView{{}}}, nil); st != StateDegraded || got == nil || got.Text != is.Text {
			t.Errorf("judge = %v %+v", st, got)
		}
		// Accepted the next time round: the news goes.
		f.refuse = nil
		a.syncDNS(context.Background(), true, true)
		a.dev.dnsOutside = false // read again: the placeholder routes into TUN
		if is := a.dnsIssue(a.settings.Get(), running); is != nil {
			t.Errorf("issue after the takeover: %+v", is)
		}
	})
	t.Run("accepted but not set", func(t *testing.T) {
		f := twoServices()
		f.deaf = true
		a := dnsApp(t, f)
		lanEnv(a)
		a.syncDNS(context.Background(), true, true)
		if is := a.dnsIssue(a.settings.Get(), running); is == nil || !strings.Contains(is.Detail, "Wi-Fi") {
			t.Errorf("issue = %+v", is)
		}
	})
	t.Run("kept by the user", func(t *testing.T) {
		f := twoServices()
		a := dnsApp(t, f)
		lanEnv(a)
		if _, err := a.settings.Update(func(s *settings.Settings) { s.TunKeepDNS = true }); err != nil {
			t.Fatal(err)
		}
		s := a.settings.Get()
		if a.dnsWanted(s) {
			t.Fatal("wanted though the user keeps the servers")
		}
		is := a.dnsIssue(s, running)
		if is == nil || is.Action != "take-dns" || !strings.Contains(is.Detail, "192.168.50.1") {
			t.Fatalf("issue = %+v", is)
		}
		// A public server routes into TUN: nothing to say.
		a.dev.env.DefaultDNS, a.dev.dnsOutside = []string{"8.8.8.8"}, false
		if is := a.dnsIssue(s, running); is != nil {
			t.Errorf("issue with a public server: %+v", is)
		}
	})
	t.Run("between the takeover and the next reading", func(t *testing.T) {
		a := dnsApp(t, twoServices())
		lanEnv(a) // read before the takeover
		a.syncDNS(context.Background(), true, true)
		if is := a.dnsIssue(a.settings.Get(), running); is != nil {
			t.Errorf("issue from a reading older than the takeover: %+v", is)
		}
		// Read again, and the system still asks the LAN: a VPN's doing.
		a.dev.at = time.Now().Add(time.Second)
		if is := a.dnsIssue(a.settings.Get(), running); is == nil || is.Action != "" {
			t.Errorf("issue = %+v", is)
		}
	})
	t.Run("a system bowel does not set", func(t *testing.T) {
		a := dnsApp(t, twoServices())
		osDNS.supported = false
		lanEnv(a)
		is := a.dnsIssue(a.settings.Get(), running)
		if is == nil || is.Action != "" || !strings.Contains(is.Detail, "systemd-resolved") {
			t.Errorf("issue = %+v", is)
		}
	})
	t.Run("TUN off", func(t *testing.T) {
		a := dnsApp(t, twoServices())
		lanEnv(a)
		if _, err := a.settings.Update(func(s *settings.Settings) { s.Tun = false }); err != nil {
			t.Fatal(err)
		}
		if is := a.dnsIssue(a.settings.Get(), running); is != nil {
			t.Errorf("issue without TUN: %+v", is)
		}
	})
}

// TestLocalDNS: the network's own names keep being asked of the network's
// servers while the system's are the placeholder.
func TestLocalDNS(t *testing.T) {
	f := twoServices()
	a := dnsApp(t, f)
	s := a.settings.Get()
	before := devnet.Env{DefaultDNS: []string{"192.168.50.1"}, PrimaryIface: "en1"}
	if got := a.localDNS(s, before, false); !slices.Equal(got, []string{"192.168.50.1"}) {
		t.Errorf("before the takeover: %v", got)
	}
	a.syncDNS(context.Background(), true, true)
	after := devnet.Env{DefaultDNS: sysdns.Placeholder, PrimaryIface: "en1"}
	if got := a.localDNS(s, after, false); !slices.Equal(got, []string{"192.168.50.1"}) {
		t.Errorf("Wi-Fi had none of its own, so the lease's: %v", got)
	}
	after.PrimaryIface = "en0"
	if got := a.localDNS(s, after, false); !slices.Equal(got, []string{"10.0.0.2"}) {
		t.Errorf("Ethernet's own: %v", got)
	}
	s.TunKeepDNS = true
	if got := a.localDNS(s, before, false); got != nil {
		t.Errorf("not taken over, so the core's \"system\" does: %v", got)
	}
	if dnsOutside([]string{"192.168.50.1"}, []string{"10.0.0.0/8", "192.168.0.0/16"}) != true ||
		dnsOutside([]string{"8.8.8.8", "192.168.50.1"}, []string{"192.168.0.0/16"}) ||
		dnsOutside([]string{"fe80::1%en1"}, []string{"fe80::/10"}) != true ||
		dnsOutside(nil, []string{"192.168.0.0/16"}) {
		t.Error("dnsOutside is wrong")
	}
}
