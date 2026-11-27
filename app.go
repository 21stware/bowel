package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"maps"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/netproxy"
	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/devnet"
	"github.com/oboo/moho/internal/gateway"
	"github.com/oboo/moho/internal/geo"
	"github.com/oboo/moho/internal/geodata"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/locate"
	"github.com/oboo/moho/internal/mihomo"
	"github.com/oboo/moho/internal/monitor"
	"github.com/oboo/moho/internal/procinfo"
	"github.com/oboo/moho/internal/provider"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/ruleset"
	"github.com/oboo/moho/internal/settings"
	"github.com/oboo/moho/internal/speed"
	"github.com/oboo/moho/internal/sysproxy"
	"github.com/oboo/moho/internal/tun"
)

// App ties the core, the stores, the monitors and the window together.
type App struct {
	dataDir   string
	settings  *settings.Store
	providers *provider.Store
	ruleSets  *ruleset.Store
	core      *core.Manager
	mon       *monitor.Monitor
	revive    *reviver // starts a core that quit by itself
	auto      gwAuto   // the gateway handing out addresses
	devs      devices  // the devices that use the gateway
	loc       *locate.Locator
	http      *http.Client
	coreBin   string      // the bundled core
	tunBin    string      // the copy TUN runs: setuid, or on Windows started elevated
	tunStale  atomic.Bool // tunBin is another version than coreBin
	// tunFault is why the core at tunBin did not start; while it is set
	// the bundled core runs in its place, with TUN out of its config.
	tunFault  atomic.Pointer[string]
	tunAsking atomic.Bool // Windows' UAC prompt for TUN's core waits for an answer (tunPrompt)
	lockFell  atomic.Bool // the exit lock's fallback to the system proxy, as refresh last found it
	tunUp     tunUpProbe  // whether the core's TUN device is up, as last asked (app_tun.go); under mu
	probePort int
	gw        *gateway.Gateway
	// tunFirewall is whether a firewall that may drop what comes back on
	// the TUN device was on at the last look (probeTunDNS).
	tunFirewall atomic.Bool
	// handingOver: the quit under way is a restart for an update, which
	// leaves the core running for the new version (app_handover.go).
	handingOver atomic.Bool

	setMu     sync.Mutex // serialises updateSettings
	routesMu  sync.Mutex // guards the two below: see routes
	routesNow []routes.Route
	imp       importedCache // the routing of a provider's own config, if in use
	routesAt  uint64
	// proxyMu is held while the system proxy changes, so that quitting
	// waits for a change under way instead of being undone by it.
	proxyMu sync.Mutex
	// dnsMu is held while the system's DNS servers change, as proxyMu is.
	dnsMu sync.Mutex
	// dnsErr is why they could not be pointed into TUN, and dnsHeld since
	// when they are.
	dnsErr  atomic.Pointer[string]
	dnsHeld atomic.Pointer[time.Time]
	// openAtLogin is whether Moho is a login item, as last read.
	openAtLogin bool
	mu          sync.Mutex
	pending     map[string]bool
	locating    map[string]bool
	sys         sysproxy.Status
	proxyErr    string       // why the last try to take the system proxy failed
	failed      *failedWrite // that try, if the OS refused it: see retryWrite
	writtenLine string       // what noteWritten last put in the log
	proxies     map[string]mihomo.Proxy
	provNodes   map[string][]string // provider id → node names, in the provider's order
	// nodesSig is the names /proxies had when provNodes was read, at
	// nodesAt: the providers' lists are read again when they change.
	nodesSig uint64
	nodesAt  time.Time
	// impWaiting is what the config in use has the core download and the
	// core does not have yet, as of nodesAt.
	impWaiting importedWaiting
	// nodeHome is the core's provider each node is in, by the node's name:
	// where a test of it is asked for. Replaced, never changed.
	nodeHome map[string]string
	// Every speed test goes through one of these: tests has the requests
	// through nodes, pinger the pings, sweeps the providers' own checks.
	tests          *speed.Queue[speed.Target]
	pinger, sweeps *speed.Queue[string]
	// refused tells the failed tests a site answered with a refusal from
	// the ones nothing answered, read off each refresh's node lists.
	refused speed.Refusals
	// viewing is what each window last drew, by window.
	viewing map[string]viewing
	// nodeProxies are the providers' nodes as last read. The maps below are
	// replaced, never changed, so a snapshot can hold on to them.
	nodeProxies map[string]mihomo.Proxy
	pings       map[string]pingResult
	bandwidth   map[string]bandwidthResult // each node's last download test, by node
	pingMu      sync.Mutex                 // guards pingEnv
	pingEnv     pingEnv
	you         *locate.Result
	lastNode    map[string]string // route id → node, to notice automatic switches
	switched    *switchNote
	shareDay    string
	routeBytes  map[string]int64 // today's download per route id
	groupSeen   map[string]int64 // the monitor's GroupBytes when last added in
	monCancel   context.CancelFunc
	lastHash    [32]byte
	emitQueued  bool
	win         *mygo.Window
	ui          *UI

	// picks are what the scheduled download tests chose, by route id, and
	// downloadTried when each was last started; downloading, that some are
	// running (runDueDownloads).
	picks         map[string]downloadPick
	downloadTried map[string]time.Time
	downloading   bool

	// winShown is whether the main window is shown, kept from its events
	// for goroutines that may not wait on the main thread to ask.
	winShown atomic.Bool
	tray     *mygo.Tray
	quitting bool
	quitOnce sync.Once
	leaving  bool // beforeQuit is running; main thread only
	lastTick Tick
	// The gateway as last measured, and what its last change said.
	gwProbe   gwProbe
	gwProbing bool
	gwErr     error
	// Whether the system's DNS is pointed at TUN, as last asked (app_tun.go).
	tunDNS     tunDNSProbe
	tunDNSBusy bool
	upd        updateState
	// headless: no window, tray or main thread's loop (`bowel daemon`,
	// app_daemon.go). stop ends such a run.
	headless bool
	stop     chan struct{}
	// watchedAt is when a command last read the connections or the logs,
	// in Unix seconds: they are polled every second while one watches.
	watchedAt atomic.Int64
	dev       devState
	tools     toolsState
}

type switchNote struct {
	route, from, to string // route is the route's ID; from and to are the core's node names
	at              time.Time
}

// switchShow is how long the news line says a switch. It is news, not a state:
// the route has already moved, and the line must not keep the title bar for as
// long as the node it left stays down.
const switchShow = time.Minute

func newApp() (*App, error) {
	data, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return nil, err
	}
	migrateData(data)
	fresh := !exists(filepath.Join(data, "settings.json"))
	st, err := settings.Open(data)
	if err != nil {
		return nil, err
	}
	if fresh && inContainer() {
		seedContainer(st, data)
	}
	i18n.Set(st.Get().Language)
	provDir := storeDir(data, "providers", runtime.GOOS)
	ps, err := provider.Open(provDir)
	if err != nil {
		return nil, err
	}
	sets, err := ruleset.Open(storeDir(data, "rulesets", runtime.GOOS))
	if err != nil {
		return nil, err
	}
	bin := bundledCore()
	tun.CoreDir = filepath.Join(data, "core") // where the core runs, below
	tunBin := tun.Path(data, core.TunName(filepath.Base(bin)))
	if tun.RootCore() {
		tunBin = bin // root's child has TUN's rights already: no copy
	}
	run, stale := startBin(st.Get(), bin, tunBin, tun.AsksEachStart, tun.Installed, tun.Stale)
	cm, err := core.New(run, tun.CoreDir)
	if err != nil {
		return nil, err
	}
	cm.Spawn = func(b string) (core.Process, error) { return spawnTun(b, bin) }
	if tun.AsksEachStart {
		// Windows makes the copy from the bundled core as it starts it, so
		// until then the bundled one stands for it.
		cm.Source = func(b string) string {
			if b == tunBin {
				return bin
			}
			return b
		}
	}
	// By the copy TUN runs on whatever the next start runs: on Windows a
	// core left running elevated is not the app's to end as a process
	// (tun.Attach), also when it is replaced.
	cm.Attach = func(pid int) core.Process { return attachTun(tunBin, pid) }
	probe, err := core.FreePort()
	if err != nil {
		return nil, err
	}
	a := &App{
		dataDir: data, settings: st, providers: ps, ruleSets: sets, core: cm,
		mon:  monitor.New(filepath.Join(data, "stats.json")),
		loc:  locate.New(filepath.Join(data, "locations.json")),
		http: &http.Client{Transport: directWhenProxyIsDown(netproxy.Transport())}, coreBin: bin, tunBin: tunBin, probePort: probe,
		pending: map[string]bool{}, locating: map[string]bool{}, proxies: map[string]mihomo.Proxy{},
		provNodes: map[string][]string{}, lastNode: map[string]string{}, routeBytes: map[string]int64{},
		gw: gateway.New(osGateway(), filepath.Join(data, "gateway-before.json")),
	}
	ps.Vet = a.vetUpdate
	a.startSpeed()
	a.loadDownloadPicks()
	a.tunStale.Store(stale)
	cm.SafePaths = []string{provDir, sets.Dir()} // for a core that is the app's child; see storeDir
	a.keepRuleSets()
	a.mon.SkipGroup = core.ProbeGroup // the location probe's own traffic
	a.mon.FindProcess = procinfo.New().Find
	a.mon.Mine = gateway.LocalAddrs
	a.mon.Every = a.pollEvery
	a.devs.path = filepath.Join(data, "gateway-devices.json")
	a.revive = &reviver{state: func() core.State { return cm.Status().State }, quitting: a.isQuitting, apply: a.applyConfig, sleep: time.Sleep}
	cm.OnStatus = func(s core.Status) { a.onCoreStatus(s) }
	cm.OnListenError = func() { a.emit() }
	cm.OnLog = a.mon.Feed // the log of a core Spawn started: see onCoreStatus
	a.mon.OnTick(a.onTick)
	return a, nil
}

// storeDir is where the files of one kind ("providers", "rulesets") are
// kept, which the core's config names. mihomo reads nothing outside its
// own directory unless a SAFE_PATHS variable allows it, and only a core
// that is the app's child can be given one. On Windows the core TUN runs
// on is started elevated, through a UAC prompt, with the user's own
// environment and none of the app's, so there the files live inside the
// core's directory, and what an earlier version kept beside it is moved
// in once. A move that fails (a file in use) leaves the files where they
// were, for the next start to try again.
func storeDir(data, name, goos string) string {
	old := filepath.Join(data, name)
	if goos != "windows" {
		return old
	}
	dir := filepath.Join(data, "core", name)
	if _, err := os.Lstat(dir); err == nil {
		return dir
	}
	if _, err := os.Lstat(old); err != nil {
		return dir // nothing to move
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err == nil {
		err = os.Rename(old, dir)
		if err == nil {
			return dir
		}
		log.Printf("moving %s into the core's directory: %v", old, err)
	}
	return old
}

// bundledCore finds mihomo: $MOHO_CORE, else the file core.BinName in bin
// among the resources.
func bundledCore() string {
	if p := os.Getenv("MOHO_CORE"); p != "" {
		return p
	}
	name := core.BinName
	dir, _ := mygo.App.Path(mygo.PathResources)
	if p := filepath.Join(dir, "bin", name); exists(p) {
		return p
	}
	return filepath.Join(dir, runtime.GOOS+"-"+runtime.GOARCH, "bin", name)
}

// bundledFonts are the faces shipped in resources/fonts, each unmodified
// beside its license, as the licenses require: the interface face (ui.go's
// sans) under its own name, and the skins' (skins.go) under one family
// each, since their files name themselves by weight ("Jost* Semi").
var bundledFonts = []struct{ file, family, license string }{
	{"OPPOSans4.0.ttf", "", "OPPO Sans License.txt"},
	{"Jost-500-Medium.ttf", "Jost", "Jost License.txt"},
	{"Jost-600-Semi.ttf", "Jost", "Jost License.txt"},
	{"Jost-700-Bold.ttf", "Jost", "Jost License.txt"},
	{"IBMPlexMono-Regular.ttf", "IBM Plex Mono", "IBM Plex Mono License.txt"},
	{"IBMPlexMono-Medium.ttf", "IBM Plex Mono", "IBM Plex Mono License.txt"},
	{"IBMPlexMono-SemiBold.ttf", "IBM Plex Mono", "IBM Plex Mono License.txt"},
	{"IBMPlexMono-Bold.ttf", "IBM Plex Mono", "IBM Plex Mono License.txt"},
}

// registerFonts adds the bundled faces.
func registerFonts() {
	res, _ := mygo.App.Path(mygo.PathResources)
	for _, f := range bundledFonts {
		b, err := os.ReadFile(filepath.Join(res, "fonts", f.file))
		if err == nil {
			err = ui.RegisterFont(b, f.family)
		}
		if err != nil {
			log.Println("font:", err)
		}
	}
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// copyGeo puts the bundled rule data next to the core's config once.
func (a *App) copyGeo() {
	res, _ := mygo.App.Path(mygo.PathResources)
	for _, f := range []string{geodata.SiteFile, geodata.IPFile} {
		dst := filepath.Join(a.core.Dir, f)
		if exists(dst) {
			continue
		}
		if b, err := os.ReadFile(filepath.Join(res, "geo", f)); err == nil {
			_ = os.WriteFile(dst, b, 0o644)
		}
	}
}

// ---- Lifecycle ----

func (a *App) start() {
	// Here, not in newApp: SetSource waits for the main thread, which a
	// test's goroutine has no loop on.
	mygo.Theme.SetSource(mygo.ThemeSource(appearanceOf(a.settings.Get())))
	registerFonts()
	a.openAtLogin = mygo.App.OpenAtLogin()
	a.upd.notify = a.emit
	a.loadUpdateOutcome(mygo.App.Version())
	a.openWindow()
	a.setupTray()
	a.copyGeo()
	geodata.Use(a.core.Dir)
	tun.OnPrompt = a.tunPrompt
	go func() {
		tun.EndOldTasks(a.tunBin) // Windows: what an earlier version's task still runs
		a.takeOverCore()          // the core the last version left, if it left one
		a.detectDev(2 * time.Second)
		if err := a.applyConfig(); err != nil {
			log.Println("core:", err)
		}
	}()
	go a.loop()
	go a.updateLoop()
	go a.devLoop()
	go a.refreshTools()
	go a.probeSources()
	go a.debugDriver()
	a.startMCP()
	a.startCLI()
}

func (a *App) openWindow() {
	u := newUI(a)
	// Before the window, so that its first frame already knows what there
	// is: nothing to list opens on the first-run screen.
	u.setSnapshot(a.snapshot())
	u.setup.on = len(u.snap.Providers) == 0
	win := mygo.NewWindow(mygo.WindowOptions{
		Title: "bowel", Width: 1060, Height: 760, MinWidth: 960, MinHeight: 660,
		TitleBarStyle: mygo.TitleBarHidden, StateKey: "main",
		// The title bar is then as tall as the controls plus twice Y, and
		// they sit in its middle: with 14pt buttons, 50, as ui.go's
		// titleBar, so they line up with the tabs.
		TrafficLightPosition: &mygo.Point{X: 18, Y: 18},
		// The same 50 for the controls Windows and Linux put at the top
		// corner: left at their own height (32 on Windows) they sit above
		// the middle of the bar, higher than the tabs and 设置.
		TitleBarHeight:  50,
		BackgroundColor: "light-dark(#f6f4ef, #171615)",
		Content:         ui.View(u.view),
		// Opened at login, Moho can stay in the menu bar.
		Hidden: a.settings.Get().StartHidden && mygo.App.WasOpenedAtLogin(),
	})
	u.win = win
	a.mu.Lock()
	a.ui = u
	a.mu.Unlock()
	win.OnClose(func(e *mygo.CloseEvent) {
		a.mu.Lock()
		q := a.quitting
		a.mu.Unlock()
		if !q { // closing keeps Moho in the menu bar
			e.PreventDefault()
			win.Hide()
		}
	})
	// The Dock icon is there while a window is: hidden, Moho lives in
	// the menu bar alone.
	win.OnShow(func() { a.winShown.Store(true); a.mon.Wake(); a.syncDock() })
	win.OnHide(func() { a.winShown.Store(false); a.syncDock() })
	a.winShown.Store(!(a.settings.Get().StartHidden && mygo.App.WasOpenedAtLogin()))
	a.mu.Lock()
	a.win = win
	a.mu.Unlock()
	a.syncDock()
}

// syncDock shows the Dock icon while the main or settings window is
// open, and hides it otherwise. On the main thread.
func (a *App) syncDock() {
	if runtime.GOOS != "darwin" {
		return
	}
	a.mu.Lock()
	win, u := a.win, a.ui
	a.mu.Unlock()
	open := win != nil && win.IsVisible()
	if u != nil && (u.set.win.Load() != nil || u.clog.win.Load() != nil) {
		open = true
	}
	want := mygo.ActivationPolicyAccessory
	if open {
		want = mygo.ActivationPolicyRegular
	}
	if mygo.App.ActivationPolicy() != want {
		mygo.App.SetActivationPolicy(want)
	}
}

func (a *App) showWindow() {
	a.mu.Lock()
	win := a.win
	a.mu.Unlock()
	// Back in the Dock before the window shows, so it comes to the front
	// with the menu bar.
	if runtime.GOOS == "darwin" {
		mygo.App.Dock.Show()
	}
	if win != nil {
		win.Show()
		win.Focus()
	}
	mygo.App.Focus()
}

// onSecondInstance runs when bowel is started again while it runs: the
// window is most likely hidden, and that start was to get it back. On the
// main thread. An app on its way out stays hidden.
func (a *App) onSecondInstance([]string, string) {
	if a.leaving || a.isQuitting() {
		return
	}
	a.showWindow()
}

func (a *App) uiRef() *UI {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.ui
}

// pollEvery is how often the monitor reads the core's connections: every
// second while the window or the menu bar panel shows them, every 2 s
// while only the menu bar icon shows the rates (Settings.TraySpeed), every
// 5 s otherwise. The totals stay exact, as they are the core's own
// counters; what goes is the rates' detail and connections too short to
// be seen.
func (a *App) pollEvery() time.Duration {
	if a.winShown.Load() {
		return time.Second
	}
	if u := a.uiRef(); u != nil && u.panel.win.Load() != nil {
		return time.Second
	}
	if time.Now().Unix()-a.watchedAt.Load() < 5 {
		return time.Second
	}
	if st := a.settings.Get(); st.TraySpeed && (a.takeoverOn(st) || trafficNow(a.tick())) {
		return traySpeedEvery
	}
	return 5 * time.Second
}

// nodeSig tells apart the sets of names in px, and of the nodes its groups
// can use.
func nodeSig(px map[string]mihomo.Proxy) uint64 {
	names := slices.Sorted(maps.Keys(px))
	h := fnv.New64a()
	for _, n := range names {
		h.Write([]byte(n))
		h.Write([]byte{0})
		for _, m := range px[n].All {
			h.Write([]byte(m))
			h.Write([]byte{0})
		}
	}
	return h.Sum64()
}

// addNodes puts the providers' nodes into px beside the groups.
func addNodes(px, nodes map[string]mihomo.Proxy) {
	for name, n := range nodes {
		if _, group := px[name]; !group {
			px[name] = n
		}
	}
}

// loop refreshes what the core and the OS say, and runs the periodic jobs.
func (a *App) loop() {
	fast := time.NewTicker(2 * time.Second)
	slow := time.NewTicker(10 * time.Minute)
	defer fast.Stop()
	defer slow.Stop()
	a.refresh()
	for {
		select {
		case <-fast.C:
			a.refresh()
		case <-slow.C:
			a.autoUpdate()
			a.locateExits(false)
		}
	}
}

// nodesEvery is how often the providers' lists are read when nothing asks.
const nodesEvery = 15 * time.Second

func (a *App) refresh() {
	a.probeSoon()
	a.probeTunUp()
	a.probeTunDNS()
	// The system proxy is judged only while it is wanted: otherwise no
	// scutil every 2 s for nothing.
	st := a.settings.Get()
	// The lock's fallback follows TUN as it is, which no setting tells of:
	// the system proxy is taken when TUN is lost and given back when it
	// carries again. A core between two runs has said nothing yet.
	if cs := a.core.Status(); cs.State == core.Running || cs.State == core.Failed {
		if fb := a.lockFallback(st, cs); a.lockFell.Swap(fb) != fb {
			a.applySystemProxy()
		}
	}
	if a.proxyWanted(st) {
		if s, err := osProxy.check(st.MixedPort); err == nil {
			a.mu.Lock()
			a.sys = s
			if s.Ours {
				a.proxyErr = ""
			}
			a.mu.Unlock()
		}
	}
	if c, err := a.core.Client(); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		px, err := c.Proxies(ctx)
		// /proxies has the groups only. The nodes, and so their tests, are
		// in the providers' lists: read them when the names change or some
		// seconds on, since a big subscription makes a long answer.
		sig := nodeSig(px)
		a.mu.Lock()
		nodes, known, stale := a.provNodes, a.nodeProxies, sig != a.nodesSig || time.Since(a.nodesAt) > nodesEvery
		homes := a.nodeHome
		waiting := a.impWaiting
		a.mu.Unlock()
		if err == nil && stale {
			var pp map[string]mihomo.ProxyProvider
			if pp, err = c.ProxyProviders(ctx); err == nil {
				nodes, known, homes = map[string][]string{}, map[string]mihomo.Proxy{}, map[string]string{}
				im := a.importedNow()
				waiting = importedWaiting{}
				if im != nil && len(im.RuleProviders) > 0 {
					if rp, err := c.RuleProviders(ctx); err == nil {
						for name := range im.RuleProviders {
							if p, ok := rp[name]; ok && p.RuleCount == 0 {
								waiting.sets = append(waiting.sets, name)
							}
						}
						slices.Sort(waiting.sets)
					}
				}
				for name, p := range pp {
					id, ok := strings.CutPrefix(name, "p-")
					if im != nil && slices.Contains(im.ProviderNames(), name) { // the nodes of a source's own provider count as its
						id, ok = a.settings.Get().ImportedRouting, true
						if len(p.Proxies) == 0 {
							waiting.subs = append(waiting.subs, name)
						}
					}
					if !ok {
						continue
					}
					for _, n := range p.Proxies {
						nodes[id] = append(nodes[id], n.Name)
						known[n.Name] = n
						homes[n.Name] = name
					}
				}
			}
		}
		cancel()
		if err == nil {
			addNodes(px, known)
			a.refused.Note(px)
			a.mu.Lock()
			a.proxies, a.provNodes, a.nodeProxies, a.nodeHome = px, nodes, known, homes
			slices.Sort(waiting.subs)
			a.impWaiting = waiting
			if stale {
				a.nodesSig, a.nodesAt = sig, time.Now()
			}
			a.mu.Unlock()
			a.followDownloads(px)
			a.noticeSwitches()
			a.runDueDownloads()
			if stale {
				a.keepMeasured()
			}
		}
	}
	a.emit()
}

func (a *App) onCoreStatus(s core.Status) {
	if a.revive != nil {
		a.revive.note(s)
	}
	a.mu.Lock()
	if a.monCancel != nil {
		a.monCancel()
		a.monCancel = nil
	}
	a.mu.Unlock()
	if s.State == core.Running {
		if c, err := a.core.Client(); err == nil {
			ctx, cancel := context.WithCancel(context.Background())
			a.mu.Lock()
			a.monCancel = cancel
			a.mu.Unlock()
			run := a.mon.Run
			if a.core.LogStreamed() {
				// The core manager streams this core's log already (Windows'
				// TUN core, which prints to no one) and feeds the monitor
				// with it: a second stream would have the core send every
				// entry twice (#200).
				run = a.mon.RunFed
			}
			go run(ctx, c)
		}
		go func() {
			a.applySystemProxy()
			a.applySystemDNS()
			a.refresh()
			a.locateExits(true)
		}()
	}
	a.emit()
}

// tunEntry is TUN as it really is. wanted is the user's choice and on is
// whether takeover is; installed is whether its core has its
// authorisation; fault is why that core did not start, when another runs
// in its place.
func tunEntry(wanted, on, installed bool, fault string, state core.State) Entry {
	e := Entry{Wanted: wanted, Status: "off"}
	switch {
	case !installed:
		e.Status = "install"
	case !on || !wanted:
	case fault != "":
		e.Status, e.Detail = "error", fmt.Sprintf(i18n.T("TUN 的内核没有启动，其余功能照常：%s", "The core TUN runs on didn't start; everything else works: %s"), fault)
	case state == core.Running:
		e.Status = "on"
	default:
		e.Status, e.Detail = "error", i18n.T("内核没有运行", "The core isn't running")
	}
	return e
}

// ---- Config ----

func (a *App) takeoverOn(s settings.Settings) bool { return !s.Paused && (s.SystemProxy || s.Tun) }

// guarding says whether the exit lock holds while takeover is off: a route
// group of bowel's own has it, so the system proxy stays on the core, which
// lets everything out directly but that group's traffic, which it refuses
// (core.Input.Guard). Not in a container, which has no system proxy: there
// the ports keep the lock (app_container_egress.go).
func (a *App) guarding(s settings.Settings) bool {
	return !a.takeoverOn(s) && locks(s) && !inContainer()
}

// locks says whether a route group of bowel's own has the exit lock.
func locks(s settings.Settings) bool { return s.ImportedRouting == "" && s.Routing.AnyLocked() }

// tunLost says whether TUN, which takeover is to go by, is known not to
// carry traffic: it has no installation, its core did not start or is not
// running, or the core could not bring the device up. A core that is
// starting has lost nothing yet.
func (a *App) tunLost(s settings.Settings, cs core.Status) bool {
	if !s.Tun || !a.takeoverOn(s) {
		return false
	}
	return !a.tunWanted(s) || cs.State == core.Failed || a.tunDown(cs) != ""
}

// lockFallback says whether the exit lock is left to the system proxy
// though the user's takeover is by TUN alone: TUN is lost, so nothing
// would bring the locked groups' traffic to the core, and it would leave
// directly. The system proxy points at the core until TUN is back, as it
// does for the lock while takeover is off; the core's rules stay
// takeover's, so those groups go by their nodes.
func (a *App) lockFallback(s settings.Settings, cs core.Status) bool {
	return a.takeoverOn(s) && !s.SystemProxy && locks(s) && a.tunLost(s, cs)
}

// proxyWanted says whether the system proxy is to point at the core: for
// takeover, for the exit lock while takeover is off, or for the exit lock
// while the TUN that takeover goes by is lost.
func (a *App) proxyWanted(s settings.Settings) bool {
	if a.takeoverOn(s) && s.SystemProxy || a.guarding(s) {
		return true
	}
	return a.takeoverOn(s) && s.Tun && locks(s) && a.lockFallback(s, a.core.Status())
}

// applyConfig rebuilds the runtime config and makes the core use it.
func (a *App) applyConfig() error { return a.applyConfigKeep(false) }

// applyConfigIfChanged is applyConfig for a change that may leave the
// core's config as it was, as the system proxy's switch does: then the core
// keeps what it has. A reload would build every provider and group anew,
// and the core then tests every node on each route's target again, which
// with a big subscription is seconds of a busy core for nothing (#200).
func (a *App) applyConfigIfChanged() error { return a.applyConfigKeep(true) }

func (a *App) applyConfigKeep(keep bool) error {
	began := time.Now()
	a.dropImported() // a provider's file, or whether it is in use, may have changed
	s := a.settings.Get()
	a.useTunCore(s)
	a.ensureIPv6(s)
	im := a.importedNow()
	var build time.Duration
	kept, err := a.applyDev(s, func(dev devnet.Overlay) ([]byte, error) {
		defer func(t time.Time) { build += time.Since(t) }(time.Now())
		return a.buildConfig(s, dev, im)
	}, keep)
	if kept {
		a.mon.Note("bowel: config unchanged, the core keeps it (build " + strconv.FormatInt(build.Milliseconds(), 10) + "ms)")
		a.syncGateway()
		a.refresh()
		return nil
	}
	applied := time.Now()
	defer func() { a.mon.Note(applyNote(err, began, applied, time.Now(), build, a.core.Timing())) }()
	if err != nil {
		a.emit()
		return err
	}
	if s.Mode == "global" {
		a.pointGlobal()
	}
	a.adoptSelections()
	logPolicyErr(a.followPolicies())
	a.mu.Lock()
	a.nodesAt = time.Time{} // providers may hold other nodes now
	a.mu.Unlock()
	a.tests.Forget() // and the core has none of its results any more
	a.syncGateway()
	a.refresh()
	return nil
}

// applyNote is the line applyConfig leaves in the log: how long a change
// of the config took and where the time went, for a report of a slow
// switch to say which part was slow (#60). check is `mihomo -t`, which a
// running core that accepts the config spares.
func applyNote(err error, began, applied, done time.Time, build time.Duration, t core.ApplyTiming) string {
	ms := func(d time.Duration) string { return strconv.FormatInt(d.Milliseconds(), 10) + "ms" }
	what := "config applied"
	if err != nil {
		what = "config not applied"
	}
	check := "by the running core"
	if t.Check > 0 {
		check = ms(t.Check)
	}
	parts := []string{"build " + ms(build), "check " + check}
	if t.Start > 0 {
		parts = append(parts, "core start "+ms(t.Start))
	} else {
		parts = append(parts, "reload "+ms(t.Reload))
	}
	if err == nil {
		parts = append(parts, "after "+ms(done.Sub(applied)))
	}
	return "bowel: " + what + " in " + ms(done.Sub(began)) + " (" + strings.Join(parts, ", ") + ")"
}

// buildConfig is the core's config for s; im, if not nil, is the routing to
// use in place of the route groups.
func (a *App) buildConfig(s settings.Settings, dev devnet.Overlay, im *core.Imported) ([]byte, error) {
	var refs []core.ProviderRef
	for _, p := range a.providers.List() {
		// A config whose nodes are all its own proxy-providers' is no node
		// file: those come with its routing, when that is in use.
		if p.Enabled && a.providers.OwnNodes(p.ID) {
			refs = append(refs, core.ProviderRef{ID: p.ID, Path: a.providers.Path(p.ID), Hidden: p.Hidden})
		}
	}
	all := a.routes()
	scopes, orders := a.nodeLists(all)
	return core.BuildConfig(core.Input{
		Providers: refs, RuleSets: a.ruleSetRefs(s), Routes: all, UserRules: routes.Active(all, s.Rules), Scopes: scopes, Orders: orders, Mode: s.Mode, MixedPort: s.MixedPort, AllowLan: s.AllowLan,
		Tun:      a.tunOn(s),
		Guard:    a.guarding(s),
		Gateway:  a.gatewayOn(s),
		LogLevel: s.LogLevel, TestURL: s.TestURL, MainURLs: mainURLs(s), ProbePort: a.probePort, Controller: a.core.ControllerFields(),
		Options: coreOptions(s.Options), Dev: dev, IPv6Usable: a.devEnv().IPv6.Usable, Imported: im,
	})
}

// pointGlobal makes global mode use the default route.
func (a *App) pointGlobal() {
	if c, err := a.core.Client(); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = c.Select(ctx, "GLOBAL", routes.Group(a.route(routes.Default)))
	}
}

// osProxy is what changes the system proxy. Tests replace it, so that they
// can never change the machine's settings.
var osProxy = struct {
	enable    func(host string, port int, bypass []string) error
	disable   func() error
	snapshot  func() (sysproxy.Saved, error)
	restore   func(context.Context, sysproxy.Saved) error
	listening func(ctx context.Context, host string, port int) bool
	// written is what the last enable gave the OS, and what the OS refused.
	written func() sysproxy.Written
	// check reads the system proxy and tells whether it points at port.
	// It is here so that a test does not read the proxy of the machine it
	// runs on, which a running bowel may have pointed at the same port.
	check func(port int) (sysproxy.Status, error)
}{sysproxy.Enable, sysproxy.Disable, sysproxy.Snapshot, sysproxy.Restore, sysproxy.Listening, sysproxy.LastWrite, sysproxy.Check}

// applySystemProxy makes the OS match the settings. prev are mixed ports
// the system may still point at, as after the port changed.
func (a *App) applySystemProxy(prev ...int) {
	a.proxyMu.Lock()
	defer a.proxyMu.Unlock()
	if a.isQuitting() {
		return // shutdown has put the system proxy back: it stays so
	}
	s := a.settings.Get()
	want := a.proxyWanted(s)
	cur, err := osProxy.check(s.MixedPort)
	if err != nil {
		return
	}
	switch {
	case want && !cur.Ours:
		if err := a.enableProxy(s, prev...); err != nil {
			log.Println("system proxy:", err)
		}
	case !want && cur.Ours:
		if err := a.disableProxy(context.Background(), s.MixedPort); err != nil {
			log.Println("system proxy:", err)
		}
	case !want:
		a.dropProxyBackup() // left by a crash, and the proxy has moved on since
	}
}

// proxyBackup is the system proxy as it was before Moho took it over. It is
// kept on disk, so that a crash does not lose it.
func (a *App) proxyBackup() string { return filepath.Join(a.dataDir, "sysproxy-before.json") }

// enableProxy points the system proxy at the core. The first time, it
// keeps what the system had, for disableProxy to put back; while the
// system still points at Moho (s's port, or one of prev), it keeps the
// first copy.
func (a *App) enableProxy(s settings.Settings, prev ...int) error {
	ports := append([]int{s.MixedPort}, prev...)
	keep := false
	if exists(a.proxyBackup()) {
		for _, p := range ports {
			if cur, err := osProxy.check(p); err == nil && cur.Ours {
				keep = true
			}
		}
	}
	return a.pointProxy(s, ports, keep)
}

// pointProxy does enableProxy's work: unless keep says the copy on disk
// stays, it first replaces that with what the system has, without ports.
// A failure is kept for the status to tell: it is not another program's doing.
func (a *App) pointProxy(s settings.Settings, ports []int, keep bool) error {
	failed, err := a.writeProxy(s, ports, keep)
	a.mu.Lock()
	a.proxyErr, a.failed = "", failed
	if err != nil {
		a.proxyErr = failure(err)
	}
	a.mu.Unlock()
	return err
}

// writeProxy says, when the OS refused the write, what retryWrite needs to
// try it again. The copy on disk is done by then, whatever the write left.
func (a *App) writeProxy(s settings.Settings, ports []int, keep bool) (*failedWrite, error) {
	if !keep {
		saved, err := osProxy.snapshot()
		if err != nil {
			return nil, fmt.Errorf("reading the system proxy: %w", err)
		}
		saved.Forget(ports...)
		b, err := json.MarshalIndent(saved, "", "  ")
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(a.proxyBackup(), b, 0o600); err != nil {
			return nil, err
		}
	}
	bypass := a.bypassOf(s)
	err := osProxy.enable("127.0.0.1", s.MixedPort, bypass)
	a.noteWritten()
	if err != nil {
		left, serr := osProxy.snapshot()
		if serr != nil {
			return nil, err // no telling later whether another program came
		}
		return &failedWrite{left: left, inputs: a.writeInputs(s)}, err
	}
	a.dev.mu.Lock()
	a.dev.bypass = bypass
	a.dev.mu.Unlock()
	return nil, nil
}

// noteWritten puts in the log what the OS refused of the write just made,
// or what did not fit in it, once for each such outcome: the 2 s and 30 s
// rounds write the same again.
func (a *App) noteWritten() {
	w := osProxy.written()
	line := ""
	if w.Refused != nil || slices.ContainsFunc(w.Dropped, func(d sysproxy.Dropped) bool { return d.Why != "ipv6" }) {
		line = w.Line()
	}
	a.mu.Lock()
	same := line == a.writtenLine
	a.writtenLine = line
	a.mu.Unlock()
	if line != "" && !same {
		log.Println("system proxy:", line)
	}
}

// proxyRetries is how many times a write the OS refused is tried again. With
// the waits of retryWrite that is over some ten minutes, enough for what
// passes; a write that can never work costs a handful of tries, on every OS,
// without telling the errors apart.
const proxyRetries = 5

// failedWrite is a write of the system proxy that the OS refused. Fields
// change under proxyMu only.
type failedWrite struct {
	left   sysproxy.Saved // the system proxy just after it
	inputs string         // what the write was made from: see writeInputs
	tries  int            // tried again so many times
	wait   int            // devLoop ticks before the next try
}

// writeInputs is what a write depends on besides the OS: when it changes, a
// write that failed may work, and the tries start over.
func (a *App) writeInputs(s settings.Settings) string {
	env := a.devEnv()
	return strings.Join(append([]string{env.PrimaryIface, overlayOf(s, env).Fingerprint()}, s.Bypass...), "\n")
}

// coreUp says whether there is a core for the system proxy to point at.
var coreUp = func(a *App) bool { return a.core.Status().State == core.Running }

// retryWrite tries again a write of the system proxy that the OS refused.
// That is bowel finishing what it was asked to do, unlike taking the proxy
// back from another program, which only the user decides: so nothing happens
// without a failure of bowel's own on record, and once the system proxy is
// no longer as the failure left it, bowel lets it be. The caller holds proxyMu.
func (a *App) retryWrite(s settings.Settings) {
	a.mu.Lock()
	f, why := a.failed, a.proxyErr
	if why == "" {
		a.failed = nil
	}
	a.mu.Unlock()
	if f == nil || why == "" || !coreUp(a) {
		return
	}
	if in := a.writeInputs(s); in != f.inputs {
		f.inputs, f.tries, f.wait = in, 0, 0
	}
	if f.tries >= proxyRetries {
		return
	}
	if f.wait > 0 {
		f.wait--
		return
	}
	cur, err := osProxy.check(s.MixedPort)
	if err != nil || cur.Ours {
		return // the next refresh clears the failure of a proxy that is ours
	}
	now, err := osProxy.snapshot()
	if err != nil {
		return
	}
	if !reflect.DeepEqual(now, f.left) {
		a.mu.Lock()
		a.failed = nil
		a.mu.Unlock()
		log.Println("system proxy: changed since the failed write, not trying again")
		return
	}
	// The copy on disk is from before the first failure. What the system has
	// now may be half of bowel's write, so it must not replace that copy.
	err = a.pointProxy(s, []int{s.MixedPort}, exists(a.proxyBackup()))
	if err == nil {
		log.Println("system proxy: taken over on try", f.tries+2)
		return
	}
	tries := f.tries + 1
	a.mu.Lock()
	if a.failed != nil {
		a.failed.tries, a.failed.wait = tries, 1<<tries-1
	}
	a.mu.Unlock()
	if firstLine(err.Error()) != why {
		log.Println("system proxy:", err)
	}
	if tries == proxyRetries {
		log.Println("system proxy: giving up after", tries+1, "tries:", why)
	}
}

// disableProxy puts back the system proxy enableProxy kept, or, with no
// copy kept, turns the proxies off. If ctx ends first the copy stays on
// disk, for the next launch to finish from.
func (a *App) disableProxy(ctx context.Context, port int) error {
	b, err := os.ReadFile(a.proxyBackup())
	var saved sysproxy.Saved
	if err != nil || json.Unmarshal(b, &saved) != nil {
		return osProxy.disable()
	}
	saved.TurnOffDead(func(host string, port int) bool { return osProxy.listening(ctx, host, port) })
	if err := osProxy.restore(ctx, saved); err != nil {
		if ctx.Err() != nil {
			return err
		}
		// Never leave it pointing at Moho, whatever else failed.
		if cur, cerr := osProxy.check(port); cerr == nil && cur.Ours {
			_ = osProxy.disable()
		}
		a.dropProxyBackup()
		return err
	}
	a.dropProxyBackup()
	return nil
}

func (a *App) dropProxyBackup() { _ = os.Remove(a.proxyBackup()) }

// ---- Pending (loading states) ----

func (a *App) begin(key string) func() {
	a.mu.Lock()
	a.pending[key] = true
	a.mu.Unlock()
	a.emit()
	return func() {
		a.mu.Lock()
		delete(a.pending, key)
		a.mu.Unlock()
		a.emit()
	}
}

func (a *App) busy(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pending[key]
}

// ---- Actions ----

func (a *App) setTakeover(on bool) error {
	done := a.begin("takeover")
	defer done()
	_, err := a.settings.Update(func(s *settings.Settings) {
		s.Paused = !on
		if on && !s.SystemProxy && !s.Tun {
			s.SystemProxy = true
		}
	})
	if err != nil {
		return err
	}
	return a.afterTakeover()
}

func (a *App) setSystemProxy(on bool) error {
	done := a.begin("entry:sys")
	defer done()
	if _, err := a.settings.Update(func(s *settings.Settings) {
		s.SystemProxy = on
		if on && s.Paused { // both switches showed off: only this one goes on
			s.Paused, s.Tun = false, false
		}
	}); err != nil {
		return err
	}
	return a.afterTakeover()
}

func (a *App) setTun(on bool) error {
	done := a.begin("entry:tun")
	defer done()
	if on {
		if err := tun.DeviceReady(); err != nil {
			return err
		}
	}
	if on && a.tunNeedsInstall() {
		if err := a.installTun(); err != nil {
			return err
		}
	} else if on {
		a.retryTunCore()
	}
	if _, err := a.settings.Update(func(s *settings.Settings) {
		s.Tun = on
		if on && s.Paused { // both switches showed off: only this one goes on
			s.Paused, s.SystemProxy = false, false
		}
	}); err != nil {
		return err
	}
	err := a.afterTakeover()
	if on && err == nil {
		a.awaitTunUp() // so that what is answered is what the core did
	}
	return err
}

// tunNeedsInstall is whether turning TUN on takes an authorization. On
// Linux an installation from before the polkit rule, or whose rule names
// another device than the settings now do, is one that does.
func (a *App) tunNeedsInstall() bool {
	a.tellTunDevice()
	return !osTun.installed(a.tunBin) || a.tunStale.Load() || osTun.needsRule(a.tunBin)
}

// tellTunDevice gives the installation the device's name as the settings
// have it now, which the rule it leaves on Linux names.
func (a *App) tellTunDevice() {
	if a.settings != nil {
		tun.SetDevice(a.settings.Get().TunDevice)
	}
}

// leaveTunCore answers a core that did not come up: when it was the copy
// TUN runs on, the bundled core takes its place, so that one failure does
// not cost the user everything else the core does, and why is kept for
// the TUN entry to show. It reports whether the core is to be started
// again, with a config built anew (TUN is out of it now).
func (a *App) leaveTunCore(err error) bool {
	var se *core.StartError
	if !errors.As(err, &se) || a.tunBin == a.coreBin || a.core.Bin != a.tunBin {
		return false
	}
	why := err.Error()
	a.tunFault.Store(&why)
	log.Println("tun: its core did not start, running the bundled one:", why)
	a.core.Stop()
	a.core.SetBin(a.coreBin)
	return true
}

// retryTunCore goes back to the copy TUN runs on after leaveTunCore left
// it, for the next config to start; and a core that is not running has
// nothing to reload, so the next config starts it whatever it was.
func (a *App) retryTunCore() {
	if a.tunFault.Swap(nil) == nil {
		return
	}
	a.core.Stop()
	a.core.SetBin(a.tunBin)
}

// tunFailed is whether TUN is wanted and is not what the user has: asking
// for it again is then a request to try again, not one already met. So it
// is for a TUN that is on without the rule that points the system's DNS
// at it (Linux): asking again is how the authorization is given where
// there is no window to click in.
func (a *App) tunFailed() bool {
	tk := a.snapshot().Takeover.Tun
	if tk.Status == "on" {
		noRule, _ := a.tunDNSState(a.core.Status())
		return noRule
	}
	return tk.Status == "error"
}

// installTun puts this version's core in the root copy and runs on it. On
// Windows nothing is installed ahead: the copy's next start, elevated,
// makes it (tun.Spawn), and that is the only prompt.
func (a *App) installTun() error {
	a.tellTunDevice()
	err := osTun.install(a.coreBin, a.tunBin)
	a.noteTunInstall(err)
	if err != nil {
		return err
	}
	a.tunInstalled()
	return nil
}

// tunInstalled moves the core to the root copy, just (re)installed.
func (a *App) tunInstalled() {
	a.tunStale.Store(false)
	a.tunFault.Store(nil)
	a.core.Stop()
	a.core.SetBin(a.tunBin)
}

// updateTun replaces a root copy older than this version, or one whose
// installation lacks what this version leaves (Linux: the polkit rule), on
// the user's word.
func (a *App) updateTun() error {
	done := a.begin("tun-update")
	defer done()
	if err := a.installTun(); err != nil {
		return err
	}
	return a.afterTakeover()
}

func (a *App) afterTakeover() error {
	err := a.applyConfigIfChanged()
	a.applySystemProxy()
	a.applySystemDNS()
	a.refresh()
	return err
}

func (a *App) setLan(on bool) error {
	done := a.begin("lan")
	defer done()
	if _, err := a.settings.Update(func(s *settings.Settings) { s.AllowLan = on }); err != nil {
		return err
	}
	return a.applyConfig()
}

func (a *App) setMode(mode string) error {
	switch mode {
	case "rule", "global", "direct":
	default:
		return errors.New(i18n.T("未知模式: ", "Unknown mode: ") + mode)
	}
	done := a.begin("mode")
	defer done()
	if _, err := a.settings.Update(func(s *settings.Settings) { s.Mode = mode }); err != nil {
		return err
	}
	c, err := a.core.Client()
	if err != nil {
		return nil // applied at the next start
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// With the exit lock the core stays in rule mode: the mode is in the
	// rules (core.BuildConfig).
	if s := a.settings.Get(); s.ImportedRouting == "" && s.Routing.AnyLocked() {
		if err := a.applyConfig(); err != nil {
			return err
		}
		_ = c.CloseAllConnections(ctx) // old connections keep their old route otherwise
		a.refresh()
		return nil
	}
	if s := a.settings.Get(); !s.DevOff {
		// The mode is part of the rules then, so they are what changes.
		dev := overlayOf(s, a.devEnv())
		if !dev.Empty() {
			if err := a.applyConfig(); err != nil {
				return err
			}
			closeRerouted(ctx, c, dev)
			a.refresh()
			return nil
		}
	}
	if err := c.Patch(ctx, map[string]any{"mode": mode}); err != nil {
		return err
	}
	if mode == "global" {
		a.pointGlobal()
	}
	_ = c.CloseAllConnections(ctx) // old connections keep their old route otherwise
	a.refresh()
	return nil
}

// retake points the system proxy back at the core, on the user's word.
// What the system has now is another program's, so a copy already on disk
// stays: that is what the user had before bowel.
func (a *App) retake() error {
	done := a.begin("retake")
	defer done()
	err := a.pointBack()
	a.refresh()
	return err
}

func (a *App) pointBack() error {
	a.proxyMu.Lock()
	defer a.proxyMu.Unlock()
	if a.isQuitting() {
		return nil // shutdown has put the system proxy back: it stays so
	}
	s := a.settings.Get()
	return a.pointProxy(s, []int{s.MixedPort}, exists(a.proxyBackup()))
}

// updateSettings changes the core's options from the 设置 page: it checks
// them, saves them and reloads the core. The system proxy follows a new
// port or bypass list. On a failure nothing is kept.
// migrateData brings the data of the app's old name, Moho, to dir, the
// first time it runs as bowel ("Moho Dev" to "bowel Dev" in development):
// the settings, providers, stats and the core with its root copy for TUN.
// Renames keep the copy's setuid. What dir has already stays.
func migrateData(dir string) {
	base := filepath.Base(dir)
	if !strings.HasPrefix(base, "bowel") {
		return
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); err == nil {
		return // moved before, or new data of its own
	}
	old := filepath.Join(filepath.Dir(dir), "Moho"+strings.TrimPrefix(base, "bowel"))
	ents, err := os.ReadDir(old)
	if err != nil {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	for _, e := range ents {
		to := filepath.Join(dir, e.Name())
		if _, err := os.Lstat(to); err == nil {
			continue
		}
		_ = os.Rename(filepath.Join(old, e.Name()), to)
	}
	_ = os.Remove(old) // when all of it moved
}

// appOnly says whether next differs from old only in what the core does
// not see: the language, starting hidden, the menu bar's speed, the theme
// color, the appearance, the skin and the board's borders.
func appOnly(old, next settings.Settings) bool {
	next.Language, next.StartHidden, next.Accent, next.Appearance = old.Language, old.StartHidden, old.Accent, old.Appearance
	next.Skin, next.Borders, next.CommonRegions = old.Skin, old.Borders, old.CommonRegions
	next.TraySpeed, next.TraySpeedTwoLines = old.TraySpeed, old.TraySpeedTwoLines
	next.MCPPolicy = old.MCPPolicy                  // the MCP server's, not the core's
	if routingCoreSame(old.Routing, next.Routing) { // a strategy is a selector's position
		next.Routing = old.Routing
	}
	if maps.Equal(mainURLs(old), mainURLs(next)) { // only the main targets are in the config
		next.RouteTargets = old.RouteTargets
	}
	return reflect.DeepEqual(old, next)
}

func (a *App) updateSettings(fn func(*settings.Settings)) error {
	return a.updateSettingsAnd(fn, false)
}

// listenPorts are the ports the core listens on with settings s, the probe's
// aside; 0 for one that is off.
func listenPorts(s settings.Settings) []int {
	return append([]int{s.MixedPort, s.HTTPPort, s.SocksPort, s.ExtPort()}, s.ExtraPortNumbers()...)
}

// updateSettingsAnd is updateSettings; with reapply the core's config is
// built again even when the settings leave it as it was, for a change that
// comes with other providers or rule sets (importConfig).
func (a *App) updateSettingsAnd(fn func(*settings.Settings), reapply bool) error {
	a.setMu.Lock() // one change at a time: each starts from the last
	defer a.setMu.Unlock()
	done := a.begin("settings")
	defer done()
	old := a.settings.Get()
	next := old.Clone()
	fn(&next)
	if err := next.Validate(); err != nil {
		return err
	}
	// A port the core does not listen on yet must be free.
	had := listenPorts(old)
	for _, p := range listenPorts(next) {
		if p == 0 || slices.Contains(had, p) {
			continue // the core's already: it moves between listeners on reload
		}
		if p == a.probePort {
			return fmt.Errorf(i18n.T("端口 %d 已被 bowel 自己占用", "Port %d is taken by bowel itself"), p)
		}
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			return fmt.Errorf(i18n.T("端口 %d 被其他程序占用", "Port %d is taken by another program"), p)
		}
		l.Close()
	}
	// An extra port is given a route group there is, or, while a provider's
	// own routing is in use, a group of that config. One whose group is
	// deleted later stays in the settings, closed. Only a new entry is
	// looked up: no other change reads the config for it.
	added := slices.DeleteFunc(slices.Clone(next.ExtraPorts), func(e core.ExtraPort) bool { return slices.Contains(old.ExtraPorts, e) })
	if len(added) > 0 {
		im := a.importedNow()
		for _, e := range added {
			if im != nil {
				if _, ok := im.GroupOf(e.Route); ok {
					continue
				}
			}
			if !slices.ContainsFunc(routes.Build(next.Routing, nil), func(r routes.Route) bool { return r.ID == e.Route }) {
				return fmt.Errorf(i18n.T("端口 %d 指向不存在的路由组: %s", "Port %d points at a route group that doesn't exist: %s"), e.Port, e.Route)
			}
		}
	}
	if _, err := a.settings.Update(func(s *settings.Settings) { *s = next }); err != nil {
		return err
	}
	i18n.Set(next.Language)
	if appearanceOf(next) != appearanceOf(old) && !a.headless {
		mygo.Theme.SetSource(mygo.ThemeSource(appearanceOf(next)))
	}
	if next.TraySpeed != old.TraySpeed || next.TraySpeedTwoLines != old.TraySpeedTwoLines {
		a.mon.Wake() // the rates beside the icon: now, and at their own pace
		a.trayTick()
	}
	if !reapply && appOnly(old, next) { // the core does not care
		var err error
		if !reflect.DeepEqual(old.Routing, next.Routing) {
			err = a.followPolicies()
		}
		a.refresh()
		return err
	}
	if err := a.applyConfig(); err != nil {
		// Back to what worked, so that the next start does not fail too.
		_, _ = a.settings.Update(func(s *settings.Settings) { *s = old })
		i18n.Set(old.Language)
		_ = a.applyConfig()
		return err
	}
	if next.TunKeepDNS != old.TunKeepDNS || (len(next.DNSHijack) == 0) != (len(old.DNSHijack) == 0) {
		a.applySystemDNS()
	}
	if next.MixedPort != old.MixedPort {
		a.applySystemProxy(old.MixedPort) // points the system at the new port
	} else if !slices.Equal(next.Bypass, old.Bypass) && a.proxyWanted(next) {
		if cur, err := osProxy.check(next.MixedPort); err == nil && cur.Ours {
			if err := a.enableProxy(next); err != nil {
				return err
			}
		}
	}
	if cs := a.core.Status(); next.MixedPort == old.MixedPort && (a.guarding(next) != a.guarding(old) || a.lockFallback(next, cs) != a.lockFallback(old, cs)) {
		a.applySystemProxy() // the exit lock, with takeover off or TUN lost, holds by it
	}
	a.refresh()
	return nil
}

// resetSettings puts the core's options back to Moho's defaults.
func (a *App) resetSettings() error {
	return a.updateSettings((*settings.Settings).ResetCore)
}

// coreCall runs one controller request, with a timeout.
func (a *App) coreCall(timeout time.Duration, fn func(context.Context, *mihomo.Client) error) error {
	c, err := a.core.Client()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return fn(ctx, c)
}

// updateGeo asks the core to download the geo databases again. It answers
// at once and downloads in the background, then loads them.
func (a *App) updateGeo() error {
	done := a.begin("geo")
	defer done()
	return a.coreCall(2*time.Minute, func(ctx context.Context, c *mihomo.Client) error { return c.UpdateGeo(ctx) })
}

// flushDNS forgets the resolver's cache and its fake-ip mappings.
func (a *App) flushDNS() error {
	return a.coreCall(5*time.Second, func(ctx context.Context, c *mihomo.Client) error {
		if err := c.FlushDNS(ctx); err != nil {
			return err
		}
		return c.FlushFakeIP(ctx)
	})
}

// setOpenAtLogin makes Moho a login item of the user, or not.
func (a *App) setOpenAtLogin(on bool) error {
	if err := mygo.App.SetOpenAtLogin(on); err != nil {
		return err
	}
	a.mu.Lock()
	a.openAtLogin = mygo.App.OpenAtLogin()
	a.mu.Unlock()
	a.refresh()
	return nil
}

// readOpenAtLogin asks the system again: the user can change it in System
// Settings.
func (a *App) readOpenAtLogin() {
	on := mygo.App.OpenAtLogin()
	a.mu.Lock()
	changed := a.openAtLogin != on
	a.openAtLogin = on
	a.mu.Unlock()
	if changed {
		a.refresh()
	}
}

// revealConfig shows the core's runtime config where this system shows a
// file: Finder selects it, Explorer selects it, and each is asked how.
func (a *App) revealConfig() error {
	return revealFile(a.core.ConfigPath())
}

func (a *App) route(id string) routes.Route {
	for _, r := range a.routes() {
		if r.ID == id {
			return r
		}
	}
	return routes.Route{}
}

// selectNode pins a route group to node; "" sends it back to the fastest.
func (a *App) selectNode(routeID, node string) error {
	r := a.route(routeID)
	if r.ID == "" || !r.NodeGroups() {
		return errors.New(i18n.T("没有这条路由", "No such route"))
	}
	if a.importedNow() != nil {
		return a.selectMember(routes.Group(r), a.sourceName(node))
	}
	if _, err := a.core.Client(); err != nil {
		return err
	}
	done := a.begin("node:" + routeID)
	defer done()
	err := a.updateSettings(func(s *settings.Settings) { s.Routing.PickNode(routeID, node) })
	if err != nil {
		return err
	}
	a.keepMeasured()
	if node != "" {
		go a.probe(node)
	}
	return nil
}

// testRoute tests, now, what a route's page shows: each node the route can
// use on its main target, one request per node and a few at a time; the
// route's other sites through the node it uses; and those nodes' pings.
func (a *App) testRoute(ctx context.Context, routeID string) error {
	r := a.route(routeID)
	if r.ID == "" || !r.NodeGroups() {
		return errors.New(i18n.T("没有这条路由", "No such route"))
	}
	if _, err := a.core.Client(); err != nil {
		return err
	}
	done := a.begin("test:" + routeID)
	defer done()
	ctx, cancel := context.WithTimeout(ctx, testWait)
	defer cancel()
	a.mu.Lock()
	px := a.proxies
	a.mu.Unlock()
	targets := a.settings.Get().TargetsOf(routeID)
	list := speed.OfRoute(px, r, targets[0].URL)
	var nodes []string
	for _, t := range list {
		if providerOf(t.Node) != "" {
			nodes = append(nodes, t.Node)
		}
	}
	if n := currentNode(px, r); providerOf(n) != "" {
		for _, t := range targets[1:] {
			list = append(list, speed.Target{Node: n, URL: t.URL})
		}
	}
	var wg sync.WaitGroup
	wg.Go(func() { a.pinger.Now(ctx, nodes) })
	a.tests.Now(ctx, list)
	wg.Wait()
	a.readTests() // the new results are in the providers' lists
	if ctx.Err() != nil {
		return errors.New(i18n.T("还没有测完，其余的结果会陆续出来", "Not all tested yet; the rest of the results will follow"))
	}
	return nil
}

// testProvider tests, now, what a provider's row shows: each of its nodes
// on the default route's main target, a few at a time, and their pings.
func (a *App) testProvider(ctx context.Context, id string) error {
	if _, err := a.core.Client(); err != nil {
		return err
	}
	a.mu.Lock()
	px := a.proxies
	a.mu.Unlock()
	var nodes []string
	for name := range px {
		if providerOf(name) == id {
			nodes = append(nodes, name)
		}
	}
	if len(nodes) == 0 {
		return errors.New(i18n.T("这个来源没有可测的节点", "This source has no nodes to test"))
	}
	slices.Sort(nodes)
	done := a.begin("test-provider:" + id)
	defer done()
	ctx, cancel := context.WithTimeout(ctx, testWait)
	defer cancel()
	url := mainURLs(a.settings.Get())[routes.Default]
	list := make([]speed.Target, len(nodes))
	for i, n := range nodes {
		list[i] = speed.Target{Node: n, URL: url}
	}
	var wg sync.WaitGroup
	wg.Go(func() { a.pinger.Now(ctx, nodes) })
	a.tests.Now(ctx, list)
	wg.Wait()
	a.readTests()
	if ctx.Err() != nil {
		return errors.New(i18n.T("还没有测完，其余的结果会陆续出来", "Not all tested yet; the rest of the results will follow"))
	}
	return nil
}

func (a *App) locate(ctx context.Context, node string) error {
	_, err := a.probeErr(ctx, node)
	return err
}

// probe measures a node's exit in the background, showing "locating".
func (a *App) probe(node string) {
	if r := a.loc.Guess(node, display(node)); r.Source == "probe" {
		return
	}
	_, _ = a.probeErr(context.Background(), node)
}

func (a *App) probeErr(ctx context.Context, node string) (locate.Result, error) {
	c, err := a.core.Client()
	if err != nil {
		return locate.Result{}, err
	}
	a.mu.Lock()
	a.locating[node] = true
	a.mu.Unlock()
	a.emit()
	r, err := a.loc.Probe(ctx, c, a.probePort, node)
	a.mu.Lock()
	delete(a.locating, node)
	if node == "DIRECT" && err == nil {
		a.you = &r
	}
	a.mu.Unlock()
	a.emit()
	return r, err
}

// locateExits measures the user's own place and the nodes routes use now.
func (a *App) locateExits(force bool) {
	if force || a.you == nil {
		_, _ = a.probeErr(context.Background(), "DIRECT")
	}
	a.mu.Lock()
	px := a.proxies
	a.mu.Unlock()
	seen := map[string]bool{}
	for _, r := range a.routes() {
		if r.Direct {
			continue
		}
		if n := currentNode(px, r); n != "" && !seen[n] {
			seen[n] = true
			if force {
				_, _ = a.probeErr(context.Background(), n)
			} else {
				a.probe(n)
			}
		}
	}
}

// currentNode is the node a route uses now, following the groups its
// selector points through. "" if it has none: REJECT, in a range or list
// with nothing in it.
func currentNode(px map[string]mihomo.Proxy, r routes.Route) string {
	g, ok := px[routes.Group(r)]
	if !ok {
		return ""
	}
	if g.Now == "" {
		return routes.Group(r) // a rule of an imported config may name a node
	}
	n := g.Now
	for i := 0; i < 4 && routes.Internal(r, n); i++ {
		n = px[n].Now
	}
	for i := 0; i < 16 && px[n].Now != ""; i++ { // an imported group's members may be groups
		n = px[n].Now
	}
	if routes.Internal(r, n) || n == "REJECT" {
		return ""
	}
	return n
}

// noticeSwitches records a route's automatic group moving off a failed node.
// What is kept here is the core's own names: how they read is worked out when
// the note is shown (switchFor), where a.mu is not held and a name that
// changed in between shows at once.
func (a *App) noticeSwitches() {
	s := a.settings.Get()
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, r := range a.routes() {
		if r.Direct {
			continue
		}
		n := currentNode(a.proxies, r)
		prev := a.lastNode[r.ID]
		a.lastNode[r.ID] = n
		if now := a.proxies[routes.Group(r)].Now; prev == "" || prev == n || n == "" || (now != routes.Auto(r) && now != routes.Backup(r)) {
			continue
		}
		targets := s.TargetsOf(r.ID)
		if len(targets) == 0 {
			continue
		}
		if h, ok := speed.Last(a.proxies[prev].HistoryOf(targets[0].URL), speed.StaleAfter(s.HealthInterval, s.URLTestInterval), time.Now()); ok && h.Delay == 0 {
			note := &switchNote{route: r.ID, from: prev, to: n, at: time.Now()}
			a.switched = note
			a.hideSwitchAt(note)
		}
	}
}

// hideSwitchAt takes the note out when its time is up, rather than at the next
// refresh, up to two seconds later. The pointer says which note this timer is
// about: a newer switch has a timer of its own.
func (a *App) hideSwitchAt(n *switchNote) {
	time.AfterFunc(switchShow, func() {
		a.mu.Lock()
		gone := a.switched == n
		if gone {
			a.switched = nil
		}
		a.mu.Unlock()
		if gone {
			a.emit() // outside the lock: emit and snapshot take a.mu
		}
	})
}

// ---- Providers ----

func (a *App) detectProvider(input string) (string, error) {
	k, err := provider.Detect(input)
	return string(k), err
}

func (a *App) addProvider(ctx context.Context, input string) (ProviderView, error) {
	done := a.begin("add")
	defer done()
	p, err := a.providers.Add(ctx, a.http, input)
	if err != nil {
		return ProviderView{}, err
	}
	return a.afterProviderAdded(p)
}

func (a *App) addProviderFile(ctx context.Context) (*ProviderView, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: mygo.CallerWindow(ctx), Title: i18n.T("添加 Provider", "Add Provider"),
		Filters: []mygo.FileFilter{{Name: i18n.T("Clash 配置或节点列表", "Clash config or node list"), Extensions: []string{"yaml", "yml", "txt", "conf"}}},
	})
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	done := a.begin("add")
	defer done()
	p, err := a.providers.AddFile(paths[0])
	if err != nil {
		return nil, err
	}
	v, err := a.afterProviderAdded(p)
	return &v, err
}

// addProviderFileAs adds a local file under name, the name a config has for
// a proxy-provider that is a local file: the config can be used from then on.
func (a *App) addProviderFileAs(ctx context.Context, name string) error {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: mygo.CallerWindow(ctx), Title: fmt.Sprintf(i18n.T("选择「%s」的文件", "Choose the file of %s"), name),
		Filters: []mygo.FileFilter{{Name: i18n.T("Clash 配置或节点列表", "Clash config or node list"), Extensions: []string{"yaml", "yml", "txt", "conf"}}},
	})
	if err != nil || len(paths) == 0 {
		return err
	}
	return a.addFileAs(paths[0], name)
}

func (a *App) addFileAs(path, name string) error {
	done := a.begin("add")
	defer done()
	p, err := a.providers.AddFile(path)
	if err != nil {
		return err
	}
	if p, err = a.providers.Rename(p.ID, name); err != nil {
		_ = a.providers.Delete(p.ID)
		return err
	}
	_, err = a.afterProviderAdded(p)
	return err
}

// reloadProvider reads a local file's Provider again from the file it came
// from. That is a file chosen on this machine; when there is none to go
// back to (pasted text, a configuration imported from elsewhere), it asks.
func (a *App) reloadProvider(ctx context.Context, id string) error {
	p, ok := a.providers.Get(id)
	if !ok {
		return fmt.Errorf(i18n.T("没有这个 Provider: %s", "No such Provider: %s"), id)
	}
	path := ""
	if p.Source == "" {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: mygo.CallerWindow(ctx), Title: fmt.Sprintf(i18n.T("选择「%s」的文件", "Choose the file of %s"), p.Name),
			Filters: []mygo.FileFilter{{Name: i18n.T("Clash 配置或节点列表", "Clash config or node list"), Extensions: []string{"yaml", "yml", "txt", "conf"}}},
		})
		if err != nil || len(paths) == 0 {
			return err
		}
		path = paths[0]
	}
	return a.reloadFile(id, path)
}

func (a *App) reloadFile(id, path string) error {
	done := a.begin("provider:" + id)
	defer done()
	if _, err := a.providers.Reload(id, path); err != nil {
		a.emit()
		return err
	}
	err := a.applyConfig()
	go a.healthCheck(id)
	return err
}

func (a *App) afterProviderAdded(p provider.Provider) (ProviderView, error) {
	if err := a.applyConfig(); err != nil {
		_ = a.providers.Delete(p.ID)
		_ = a.applyConfig()
		return ProviderView{}, fmt.Errorf(i18n.T("没有添加：加上这个 Provider 后，内核不接受新的配置。%w", "Not added: with this Provider, the core refuses the new config. %w"), err)
	}
	go a.healthCheck(p.ID)
	return a.providerView(p), nil
}

// healthCheck tests a provider's nodes and locates the ones in use. A
// provider being tested already is waited for, not tested again.
func (a *App) healthCheck(id string) {
	a.sweeps.Now(context.Background(), []string{id})
	a.refresh()
	a.locateExits(false)
}

func (a *App) updateProvider(ctx context.Context, id string) error {
	done := a.begin("provider:" + id)
	defer done()
	if _, err := a.providers.Update(ctx, a.http, id); err != nil {
		a.emit()
		return err
	}
	err := a.applyConfig()
	go a.healthCheck(id)
	return err
}

func (a *App) updateAll(ctx context.Context) error {
	done := a.begin("providers")
	defer done()
	var errs []error
	for _, p := range a.providers.List() {
		if p.Kind == provider.Subscription && p.Enabled {
			if _, err := a.providers.Update(ctx, a.http, p.ID); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", p.Name, err))
			}
		}
	}
	if err := a.applyConfig(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (a *App) autoUpdate() {
	ids := a.providers.Due(time.Now())
	for _, id := range ids {
		if _, err := a.providers.Update(context.Background(), a.http, id); err != nil {
			log.Println("auto update:", err)
		}
	}
	if len(ids) > 0 {
		_ = a.applyConfig()
	}
	a.autoUpdateRuleSets()
}

func (a *App) setProviderEnabled(id string, on bool) error {
	done := a.begin("provider:" + id)
	defer done()
	if _, err := a.providers.SetEnabled(id, on); err != nil {
		return err
	}
	return a.applyConfig() // a disabled source's routing stops, and comes back with it
}

func (a *App) renameProvider(id, name string) error {
	p, err := a.providers.Rename(id, name)
	// A config's local file is found by its name: one in use may have just
	// gained or lost its file.
	if err == nil && p.Kind == provider.File && a.settings.Get().ImportedRouting != "" {
		return a.applyConfig()
	}
	a.emit()
	return err
}

// moveProvider gives provider id the place of provider to. Their order is
// that of the nodes everywhere, the core's groups too.
func (a *App) moveProvider(id, to string) error {
	moved, err := a.providers.Move(id, to)
	if err != nil || !moved {
		return err
	}
	return a.applyConfig()
}

func (a *App) setAutoUpdate(id string, hours int) error {
	_, err := a.providers.SetAutoUpdate(id, hours)
	a.emit()
	return err
}

func (a *App) deleteProvider(id string) error {
	done := a.begin("provider:" + id)
	defer done()
	if err := a.providers.Delete(id); err != nil {
		return err
	}
	if a.settings.Get().ImportedRouting == id {
		if _, err := a.settings.Update(func(s *settings.Settings) { s.ImportedRouting = "" }); err != nil {
			return err
		}
	}
	return a.applyConfig()
}

// hideNode removes node (its full name) from its provider, or with hide
// false brings it back. It stays removed through the provider's updates.
// The route groups that picked it by hand are left as they are: one whose
// range or list it was all of shows as broken, as when a subscription drops
// a node, and is whole again once the node is restored. pickedBy names them.
// When the provider's own routing is in use, the node leaves that too
// (core.Imported.Without).
func (a *App) hideNode(node string, hide bool) (pickedBy []string, err error) {
	i := strings.LastIndex(node, core.NodeSep)
	if i < 0 {
		return nil, fmt.Errorf(i18n.T("「%s」不是来自 Provider 的节点", "%s is no Provider's node"), node)
	}
	id, name := node[i+len(core.NodeSep):], node[:i]
	done := a.begin("provider:" + id)
	defer done()
	if _, err := a.providers.Hide(id, name, hide); err != nil {
		return nil, err
	}
	a.dropImported() // the routing in use may be this provider's: read it again without the node
	if hide {
		pickedBy = a.pickedBy(node)
	}
	return pickedBy, a.applyConfig()
}

// pickedBy are the titles of the route groups that name node in their
// strategy: as their fixed node, on their list, or in their range.
func (a *App) pickedBy(node string) []string {
	var out []string
	for _, r := range a.routes() {
		q := r.Policy
		if q.Fixed == node || slices.Contains(q.Order, node) || slices.Contains(q.Nodes, node) {
			out = append(out, routes.Title(r))
		}
	}
	return out
}

// ---- The user's rules ----

// addRule sends r's traffic to its route, in place of an older rule for
// the same traffic. Connections it matches that went elsewhere are closed,
// so that the change shows at once: their apps connect again, by the new
// way. It returns how many were closed.
func (a *App) addRule(r routes.UserRule) (int, error) {
	if err := a.rulesEditable(); err != nil {
		return 0, err
	}
	if a.route(r.Route).ID == "" {
		return 0, errors.New(i18n.T("没有这条路由", "No such route"))
	}
	if err := appRuleWorks(r); err != nil {
		return 0, err
	}
	if err := a.updateSettings(func(s *settings.Settings) {
		s.Routing.Claim(s.Rules, r)
		s.Rules = routes.Upsert(s.Rules, r)
	}); err != nil {
		return 0, err
	}
	return a.reroute(r, r.Route), nil
}

// appRuleWorks refuses a new rule for an application where the core cannot
// tell applications apart (routes.AppRules): it would be kept and never
// match. One that came with imported settings stays, and can be removed.
func appRuleWorks(r routes.UserRule) error {
	if r.Kind == routes.KindApp && !routes.AppRules {
		return errors.New(i18n.T("这个系统上内核认不出连接来自哪个应用，按应用的规则不会生效", "On this system the core can't tell which app opened a connection, so a rule for an app would never match"))
	}
	return nil
}

// removeRule takes out the rule for r's traffic, and closes the
// connections it matched.
func (a *App) removeRule(r routes.UserRule) (int, error) {
	if err := a.updateSettings(func(s *settings.Settings) { s.Rules = routes.Remove(s.Rules, r) }); err != nil {
		return 0, err
	}
	return a.reroute(r, ""), nil
}

// reroute closes the open connections r matches that are not on route
// keep. A failure only leaves them open: the rule holds for new ones.
func (a *App) reroute(r routes.UserRule, keep string) int {
	if r.Kind == routes.KindApp {
		return a.rerouteWhere(func(_, _, process, path string) bool { return r.MatchesApp(process, path) }, keep)
	}
	return a.rerouteBy(r.Matches, keep)
}

// closeUnmatched closes the connections no rule matched, so that a change
// of where they go shows. It reports how many it closed.
func (a *App) closeUnmatched() int {
	c, err := a.core.Client()
	if err != nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	snap, err := c.Connections(ctx)
	if err != nil {
		return 0
	}
	var ids []string
	for _, cn := range snap.Connections {
		if cn.Rule == "Match" {
			ids = append(ids, cn.ID)
		}
	}
	if len(ids) > 0 && a.closeConns(ids) == nil {
		a.mon.Wake()
		return len(ids)
	}
	return 0
}

// setLocked turns route group id's exit lock on or off, and closes its
// connections, so that they open again the way the lock says.
func (a *App) setLocked(id string, on bool) error {
	if !routes.Lockable(id) {
		return fmt.Errorf(i18n.T("路由组 %s 不能锁定出口", "Route group %s can't have its exit locked"), id)
	}
	if err := a.updateSettings(func(s *settings.Settings) { s.Routing.SetLocked(id, on) }); err != nil {
		return err
	}
	a.closeRoute(id)
	return nil
}

// closeRoute closes the connections on route group id, so that a change of
// where it leaves shows. It reports how many it closed.
func (a *App) closeRoute(id string) int {
	if id == routes.Default {
		return a.closeUnmatched()
	}
	c, err := a.core.Client()
	if err != nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	snap, err := c.Connections(ctx)
	if err != nil {
		return 0
	}
	var ids []string
	for _, cn := range snap.Connections {
		if len(cn.Chains) > 0 && a.routeOfGroup(cn.Chains[len(cn.Chains)-1], cn.Rule) == id {
			ids = append(ids, cn.ID)
		}
	}
	if len(ids) > 0 && a.closeConns(ids) == nil {
		a.mon.Wake()
		return len(ids)
	}
	return 0
}

// rerouteBy is reroute for whatever match catches.
func (a *App) rerouteBy(match func(host, ip string) bool, keep string) int {
	return a.rerouteWhere(func(host, ip, _, _ string) bool { return match(host, ip) }, keep)
}

// rerouteWhere is rerouteBy with the program that connected, as the core
// names it, and its path.
func (a *App) rerouteWhere(match func(host, ip, process, path string) bool, keep string) int {
	c, err := a.core.Client()
	if err != nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	snap, err := c.Connections(ctx)
	if err != nil {
		return 0
	}
	var ids []string
	for _, cn := range snap.Connections {
		host := cn.Metadata.Host
		if host == "" {
			host = cn.Metadata.SniffHost
		}
		if !match(host, cn.Metadata.DestinationIP, cn.Metadata.Process, cn.Metadata.ProcessPath) {
			continue
		}
		group := ""
		if len(cn.Chains) > 0 {
			group = cn.Chains[len(cn.Chains)-1]
		}
		if keep == "" || a.routeOfGroup(group, cn.Rule) != keep {
			ids = append(ids, cn.ID)
		}
	}
	if len(ids) > 0 && a.closeConns(ids) == nil {
		a.mon.Wake()
		return len(ids)
	}
	return 0
}

// ---- Activity ----

// routeOfGroup maps a mihomo group name (or DIRECT, REJECT) to a route id.
func (a *App) routeOfGroup(group, rule string) string {
	for _, r := range a.routes() {
		if (!r.Direct || r.OwnExit()) && (group == routes.Group(r) || routes.Internal(r, group)) {
			return r.ID
		}
	}
	if group == "REJECT" { // a route's empty range shows as the route's group
		return routes.Reject
	}
	if group != "DIRECT" {
		return ""
	}
	if rule == "Match" { // what no rule matched, when the default route goes direct
		if a.route(routes.Default).Policy.Direct {
			return routes.Default
		}
		return ""
	}
	if rule != "" {
		for _, r := range a.routes() {
			if r.Direct && !r.Reject && r.ID != routes.Default && !r.OwnExit() {
				return r.ID
			}
		}
	}
	return ""
}

// display is a node's name without Moho's provider marker.
func display(node string) string {
	if i := strings.Index(node, core.NodeSep); i >= 0 {
		return node[:i]
	}
	return node
}

// label is what people read for a node: its own name, but a node that is
// all of a pasted link provider goes by the provider's name. That name was
// taken from the link's #fragment, so renaming the provider renames the node
// for people; mihomo keeps the node's name, and with it the choices made.
// Either goes without the characters providers hide in a name
// (geo.Visible), so that a name is found by what it reads as.
// provNodes is the providers' nodes, as a.provNodes.
func (a *App) label(provNodes map[string][]string, node string) string {
	id := providerOf(node)
	if len(provNodes[id]) == 1 {
		if p, ok := a.providers.Get(id); ok && p.Kind == provider.Link {
			return trimFlag(geo.Visible(p.Name))
		}
	}
	return trimFlag(geo.Visible(display(node)))
}

// trimFlag drops a flag emoji leading a name ("🇺🇸美国纽约" is "美国纽约"):
// the interface draws the place's flag before the name itself. A name that
// is nothing but a flag stays.
func trimFlag(s string) string {
	t := strings.TrimLeftFunc(s, func(r rune) bool {
		return r >= 0x1F1E6 && r <= 0x1F1FF || unicode.IsSpace(r)
	})
	if t == "" {
		return s
	}
	return t
}

// display is a node's label; not with a.mu held.
func (a *App) display(node string) string {
	a.mu.Lock()
	pn := a.provNodes
	a.mu.Unlock()
	return a.label(pn, node)
}

// nodeShown is a node as the news line names it: "[Provider]Node" when the
// app has more than one provider, the node alone otherwise, both names read as
// the node list reads them. A node a name leaves nothing of still reads as
// something.
func (a *App) nodeShown(provNodes map[string][]string, node string) string {
	shown := a.label(provNodes, node) // as NodeView.Display is made
	if shown == "" {
		shown = "…"
	}
	id := providerOf(node)
	if id == "" || !severalProviders(provNodes) {
		return shown
	}
	p, ok := a.providers.Get(id)
	if !ok {
		return shown // the provider is gone: the node alone is truer than a bare id
	}
	name := trimFlag(geo.Visible(p.Name))
	if name == "" || name == shown {
		return shown // a link provider's only node already goes by its name
	}
	return "[" + name + "]" + shown
}

// severalProviders says whether naming a node's provider adds anything: with
// one provider, its name is the same on every line.
func severalProviders(provNodes map[string][]string) bool {
	seen := 0
	for _, nodes := range provNodes {
		if len(nodes) > 0 {
			seen++
			if seen > 1 {
				return true
			}
		}
	}
	return false
}

func providerOf(node string) string {
	if i := strings.Index(node, core.NodeSep); i >= 0 {
		return node[i+len(core.NodeSep):]
	}
	return ""
}

// groupTitle is a chain's step as the interface shows it: a route's group
// by the route's title, anything else (a node, DIRECT) as it is.
func (a *App) groupTitle(name string) string {
	for _, r := range a.routes() {
		switch name {
		case routes.Group(r):
			return routes.Title(r)
		case routes.Auto(r):
			return routes.Title(r) + i18n.T("·自动", " · Auto")
		case routes.Backup(r):
			return routes.Title(r) + i18n.T("·备用", " · Backup")
		}
	}
	return name
}

func (a *App) isBackupStep(name string) bool {
	for _, r := range a.routes() {
		if strings.HasPrefix(name, routes.Backup(r)+"·") {
			return true
		}
	}
	return false
}

func (a *App) convConn(c monitor.Conn) Conn {
	group := ""
	if len(c.Chains) > 0 {
		group = c.Chains[len(c.Chains)-1]
	}
	v := Conn{
		ID: c.ID, App: c.App, Process: c.Process, Host: c.Host, IP: c.IP, Port: c.Port, Network: c.Network,
		Rule: c.Rule, Payload: c.Payload, Route: a.routeOfGroup(group, c.Rule),
		Down: c.Down, Up: c.Up, DownRate: c.DownRate, UpRate: c.UpRate, Start: c.Start, End: c.End,
		Status: c.Status, Error: c.Error,
	}
	for i := len(c.Chains) - 1; i >= 0; i-- {
		if a.isBackupStep(c.Chains[i]) {
			continue // one node's own group: says nothing the node doesn't
		}
		v.Chain = append(v.Chain, a.groupTitle(a.display(c.Chains[i])))
	}
	if c.Status == "blocked" {
		v.Chain = []string{routes.Title(a.route(routes.Reject))}
	}
	if len(c.Chains) > 1 || (len(c.Chains) == 1 && c.Chains[0] != "DIRECT" && group != c.Chains[0]) {
		v.Node = a.display(c.Chains[0])
	}
	return v
}

func (a *App) connections() []Conn {
	src := a.mon.Conns()
	out := make([]Conn, 0, len(src))
	for _, c := range src {
		out = append(out, a.convConn(c))
	}
	return out
}

func (a *App) closeConns(ids []string) error {
	c, err := a.core.Client()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var errs []error
	for _, id := range ids {
		if err := c.CloseConnection(ctx, id); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (a *App) logs(after int64) []LogLine {
	src := a.mon.Logs(after)
	out := make([]LogLine, len(src))
	for i, l := range src {
		out[i] = LogLine{Seq: l.Seq, Time: l.Time, Level: l.Level, Text: l.Text}
	}
	return out
}

// ---- Tick ----

func (a *App) onTick() {
	t := a.buildTick()
	a.mu.Lock()
	a.lastTick = t
	a.mu.Unlock()
	if u := a.uiRef(); u != nil {
		u.setTick(t)
	}
	a.trayTick()
}

func (a *App) tick() Tick {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastTick
}

func (a *App) buildTick() Tick {
	n := a.mon.Now()
	t := Tick{Down: n.Down, Up: n.Up, ByRoute: map[string]int64{}, Active: n.Active,
		TodayDown: n.TodayDown, TodayUp: n.TodayUp, MonthDown: n.MonthDown, Apps: []AppRate{}, Events: []Event{},
		// Only "always" asks the core to look every connection up.
		AppsUnknown: n.Unnamed && a.settings.Get().FindProcess == "always"}
	for g, v := range n.ByGroup {
		// A DIRECT group can be the direct route or the default's DIRECT; count it as direct.
		id := a.routeOfGroup(g, "x")
		if id != "" {
			t.ByRoute[id] += v
		}
	}
	// Today's share per route: the bytes since the last tick, as the
	// monitor counted them, however long ago that was.
	day := time.Now().Format("2006-01-02")
	a.mu.Lock()
	if a.shareDay != day {
		a.shareDay, a.routeBytes = day, map[string]int64{}
	}
	for g, v := range n.GroupBytes {
		if d := v - a.groupSeen[g]; d > 0 {
			if id := a.routeOfGroup(g, "x"); id != "" {
				a.routeBytes[id] += d
			}
		}
	}
	a.groupSeen = n.GroupBytes
	a.mu.Unlock()

	// Each app's busiest connection names its host: the fastest, the
	// oldest of equals, so that it holds still from one tick to the next.
	type top struct {
		rate  int64
		start time.Time
		id    string
	}
	apps := map[string]*AppRate{}
	best := map[string]top{}
	for _, c := range a.mon.Rates() {
		r := apps[c.App]
		if r == nil {
			r = &AppRate{App: c.App}
			apps[c.App] = r
		}
		r.Down += c.DownRate
		r.Up += c.UpRate
		r.Conns++
		b, seen := best[c.App]
		if !seen || c.DownRate > b.rate || c.DownRate == b.rate &&
			(c.Start.Before(b.start) || c.Start.Equal(b.start) && c.ID > b.id) {
			best[c.App] = top{c.DownRate, c.Start, c.ID}
			r.Host, r.Route = c.Host, a.routeOfGroup(c.Group, c.Rule)
		}
	}
	for _, r := range apps {
		t.Apps = append(t.Apps, *r)
	}
	sort.Slice(t.Apps, func(i, j int) bool {
		if t.Apps[i].Down != t.Apps[j].Down {
			return t.Apps[i].Down > t.Apps[j].Down
		}
		return t.Apps[i].Conns > t.Apps[j].Conns
	})
	if len(t.Apps) > 8 {
		t.Apps = t.Apps[:8]
	}
	for _, e := range a.mon.Events(30) {
		group := ""
		if len(e.Chains) > 0 {
			group = e.Chains[len(e.Chains)-1]
		}
		ev := Event{Time: e.Time, Text: e.App + " → " + e.Host, Route: a.routeOfGroup(group, "x"), Failed: e.Failed}
		if len(e.Chains) > 1 {
			ev.Node = a.display(e.Chains[0])
		}
		t.Events = append(t.Events, ev)
	}
	return t
}

// ---- Snapshot ----

// emit sends the snapshot when it changed, at most every 100 ms.
func (a *App) emit() {
	a.mu.Lock()
	if a.emitQueued {
		a.mu.Unlock()
		return
	}
	a.emitQueued = true
	a.mu.Unlock()
	time.AfterFunc(100*time.Millisecond, func() {
		a.mu.Lock()
		a.emitQueued = false
		a.mu.Unlock()
		s := a.snapshot()
		b, _ := json.Marshal(s)
		h := sha256.Sum256(b)
		a.mu.Lock()
		same := h == a.lastHash
		a.lastHash = h
		a.mu.Unlock()
		if !same {
			if u := a.uiRef(); u != nil {
				u.setSnapshot(s)
			}
			a.refreshTray(s)
		}
	})
}

func (a *App) snapshot() Snapshot {
	s := a.settings.Get()
	cs := a.core.Status()
	a.mu.Lock()
	sys, proxyErr, px, provNodes, you := a.sys, a.proxyErr, a.proxies, a.provNodes, a.you
	note := a.switched // taken with px: a refresh may record a newer switch
	pings, bandwidth, picks := a.pings, a.bandwidth, a.picks
	openAtLogin := a.openAtLogin
	gwp, gwErr := a.gwProbe, a.gwErr
	pending := make([]string, 0, len(a.pending))
	for k := range a.pending {
		pending = append(pending, k)
	}
	locating := copyMap(a.locating)
	routeBytes := copyMap(a.routeBytes)
	a.mu.Unlock()
	slices.Sort(pending)

	on := a.takeoverOn(s)
	// After the lock: a node's name is read through the interface's own rules,
	// and the note is dropped here once it no longer tells what happens.
	sw := a.switchFor(note, px, provNodes, cs, s, on)
	snap := Snapshot{
		Update: a.updateView(), UpdateInfo: a.updateInfo(),
		Mode: s.Mode, Pending: pending, Version: mygo.App.Version(), RuleCount: a.ruleCount(s),
		Settings: s, ConfigPath: a.core.ConfigPath(), OpenAtLogin: openAtLogin, RuleSets: a.ruleSetViews(s),
		ControllerTCP: a.core.TCPAddr(),
		CanUndoImport: a.canUndoImport(),
		Core:          Core{State: string(cs.State), Version: cs.Version, Error: cs.Error},
		Routes:        []RouteView{},
		Nodes:         []NodeView{},
		Providers:     []ProviderView{},
	}

	// Takeover, as it really is.
	tk := Takeover{On: on, Port: s.MixedPort, Lan: s.AllowLan, Bypass: s.Bypass}
	written := osProxy.written()
	if !written.Empty() {
		tk.Written = &written
	}
	tk.System = Entry{Wanted: s.SystemProxy, Status: "off"}
	if on && s.SystemProxy {
		switch {
		case sys.Ours:
			// A takeover that works, with less than was asked: say so.
			tk.System.Status, tk.System.Detail = "on", written.Note()
		case proxyErr != "": // bowel's own failure, whatever the system has
			tk.System.Status, tk.System.Detail = "error", proxyErr
		case sys.On:
			tk.System.Status, tk.System.Detail = "hijacked", fmt.Sprintf(i18n.T("现在指向 %s:%d", "Now points at %s:%d"), sys.Host, sys.Port)
		default:
			tk.System.Status, tk.System.Detail = "hijacked", i18n.T("系统代理被关掉了", "The system proxy was turned off")
		}
	}
	guard := a.guarding(s)
	fault := ""
	if f := a.tunFault.Load(); f != nil {
		fault = *f
	}
	tk.Tun = tunEntry(s.Tun, on, osTun.installed(a.tunBin), fault, cs.State)
	tk.Tun.Asking = a.tunAsking.Load()
	if tk.Tun.Status == "on" {
		// The setting is not the device: a core that could not bring it up
		// goes on without it.
		if why := a.tunDown(cs); why != "" {
			tk.Tun.Status, tk.Tun.Detail = "error", why
		} else {
			// TUN is on, and may still not be where the system's lookups go,
			// or the firewall may drop what its stack hands back.
			tk.Tun.Detail = tunDNSDetail(a.tunDNSState(cs))
			if tk.Tun.Detail == "" {
				tk.Tun.Detail = tunStackDetail(s.TunStack, a.tunFirewall.Load())
			}
		}
	}
	// The exit lock, as it really holds: by TUN while that carries traffic,
	// by the system proxy otherwise, and not at all when neither brings
	// the locked groups' traffic to the core.
	held := a.proxyWanted(s) && sys.Ours || on && s.Tun && !a.tunLost(s, cs) || portsHold(s)
	tk.Lock = containerLock(lockEntry(locks(s), guard, a.lockFallback(s, cs), held, proxyErr))
	if s.AllowLan {
		tk.LanAddr = lanAddr(s.MixedPort)
	}
	tk.Gateway = gatewayView(s, on, tk.Tun.Status, a.gw.System().Ready(), a.gw.Awake(), gwp, gwErr)
	tk.Gateway.Auto = a.autoView(s, tk.Gateway.Status == "on")
	if s.Gateway {
		snap.Devices = a.devs.view(a.mon.Conns())
	}
	snap.Takeover = tk
	snap.Dev = a.devReport(s, tk.Tun.Status == "on")
	snap.Tools = a.toolsView()
	snap.Sources = a.sourcesView()

	// Nodes, in their providers' order.
	mains := mainURLs(s)
	stale, now := speed.StaleAfter(s.HealthInterval, s.URLTestInterval), time.Now()
	view := func(name string) NodeView {
		p := px[name]
		v := NodeView{Name: name, Display: a.label(provNodes, name), Provider: providerOf(name), Type: p.Type, UDP: p.UDP, History: []int{}}
		v.Routes = make(map[string]NodeTest, len(mains))
		for id, u := range mains {
			v.Routes[id] = nodeTest(&a.refused, name, u, p, stale, now)
		}
		t := v.Routes[routes.Default]
		v.Delay, v.Tested, v.Refused = t.Delay, t.Tested, t.Refused
		if t.History != nil {
			v.History = t.History
		}
		v.Ping = pings[name].ms
		bw, tested := bandwidth[name]
		v.Download = downloadView(bw, tested)
		// A `dialer-proxy` names a proxy or group at the top level of the
		// running config. A config's own nodes and groups are there only
		// while its routing is in use; its nodes are listed either way.
		if via := p.DialerProxy; via != "" {
			if _, there := px[via]; !there {
				v.MissingDialer = via
			}
		}
		v.Where = located(a.loc.Guess(name, display(name)))
		if locating[name] {
			v.Where.Source = "locating"
		}
		return v
	}
	for _, p := range a.providers.List() {
		if !p.Enabled {
			continue
		}
		for _, n := range provNodes[p.ID] {
			snap.Nodes = append(snap.Nodes, view(n))
		}
	}

	// Routes.
	im := a.importedNow()
	snap.Imported = a.importedView(s, im, px, provNodes)
	var total int64
	for _, v := range routeBytes {
		total += v
	}
	for _, r := range a.routes() {
		if r.ID == routes.Default && r.Policy.Direct && s.Mode == "global" {
			r.Direct = false // global mode has no rules to miss: it goes by the nodes
		}
		rv := RouteView{ID: r.ID, Name: routes.Title(r), Color: r.Color, Summary: routes.Describe(r), Rules: len(r.Rules), Direct: r.Direct, Reject: r.Reject}
		if r.ID == routes.Default {
			rv.Rules = 1
		}
		for _, u := range s.Rules {
			if u.Route == r.ID && im == nil { // the user's rules are paused under an imported config
				rv.Mine++
			}
		}
		rv.Rules += rv.Mine
		if im != nil {
			rv.Group = routes.Group(r)
		} else if r.Locked {
			rv.Locked, rv.Refused, rv.Unheld = true, guard && held, !held
		}
		if !r.Direct {
			rv.Policy, rv.Auto = r.Policy, r.Policy.Mode == routes.ModeAuto
			rv.Node = a.listedNode(currentNode(px, r), provNodes)
			if rv.Node == "DIRECT" {
				rv.Node = ""
			}
			rv.Targets = s.TargetsOf(r.ID)
			var through mihomo.Proxy // sites are timed through a node of a provider only
			if providerOf(rv.Node) != "" {
				through = px[rv.Node]
			}
			rv.Main, rv.Sites = rv.Targets[0], siteViews(&a.refused, rv.Targets[1:], rv.Node, through, stale, now)
			if r.Policy.ByDownload() {
				pk, ok := picks[r.ID]
				d := &DownloadPickView{Node: pk.Node, Mbps: float64(pk.Bps) / 1e6, At: pk.At, Next: now}
				if ok {
					d.Next = pk.At.Add(time.Duration(r.Policy.DownloadEvery) * time.Hour)
				}
				if d.Node != "" {
					d.Node = a.listedNode(d.Node, provNodes)
				}
				rv.Download = d
			}
			rv.Broken = brokenWhy(r.Policy, snap.Nodes, r.ID)
			if rv.Broken == "" && rv.Locked && noneAlive(snap.Nodes, r.ID) {
				rv.Broken = "all" // the lock leaves it REJECT where another group would go direct
			}
		}
		if total > 0 {
			rv.Share = float64(routeBytes[r.ID]) / float64(total)
		}
		snap.Routes = append(snap.Routes, rv)
	}
	// What the rest fall through to is listed after them.
	if i := slices.IndexFunc(snap.Routes, func(r RouteView) bool { return r.ID == routes.Default }); i >= 0 {
		last := snap.Routes[i]
		snap.Routes = append(slices.Delete(snap.Routes, i, i+1), last)
	}

	// Exit and the user's place.
	if def := a.route(routes.Default); !def.Direct || def.Policy.Direct && s.Mode == "global" {
		if n := a.listedNode(currentNode(px, def), provNodes); n != "" && n != "DIRECT" {
			snap.Exit = &Exit{Node: n, Where: view(n).Where}
		}
	}
	if you != nil {
		l := located(*you)
		snap.You = &l
	}

	// Providers.
	for _, p := range a.providers.List() {
		v := a.providerView(p)
		v.Nodes = len(provNodes[p.ID])
		snap.Providers = append(snap.Providers, v)
	}

	// The one light, and the news line.
	snap.State, snap.Issue = a.judge(on, s, cs, tk, snap, sw)
	return snap
}

// switchSay is a switch as the news line says it: the names shown, not the
// core's.
type switchSay struct {
	title, from, to string
}

// switchFor is the switch, if the news line should still say it: nil once its
// time is up, the route is on another node, its group was pinned by hand, the
// node it left answers again, or traffic is not being taken over. The names
// are made here rather than where the note is recorded, which runs under a.mu.
// switchFor renders n, or nothing when it no longer tells what happens. n is
// read together with px, so both describe the same moment: a refresh that
// records a newer switch while a snapshot is being built would otherwise be
// judged against the nodes of the snapshot before it, and dropped as stale.
func (a *App) switchFor(n *switchNote, px map[string]mihomo.Proxy, provNodes map[string][]string, cs core.Status, s settings.Settings, on bool) *switchSay {
	if n == nil {
		return nil
	}
	note := *n // the fields are written once, before the note is published
	if !on || cs.State != core.Running || switchExpired(&note, time.Now()) || a.switchStale(&note, px, s) {
		a.dropSwitch(n)
		return nil
	}
	return &switchSay{
		title: routes.Title(a.route(note.route)),
		from:  a.nodeShown(provNodes, note.from),
		to:    a.nodeShown(provNodes, note.to),
	}
}

// dropSwitch forgets n, unless a newer switch replaced it meanwhile.
func (a *App) dropSwitch(n *switchNote) {
	a.mu.Lock()
	if a.switched == n {
		a.switched = nil
	}
	a.mu.Unlock()
}

func switchExpired(n *switchNote, now time.Time) bool { return now.Sub(n.at) >= switchShow }

// switchStale says whether the note no longer tells what happens: the route is
// on another node, its group was given a node by hand, or the node it left
// answers again. The group check mirrors noticeSwitches'.
func (a *App) switchStale(n *switchNote, px map[string]mihomo.Proxy, s settings.Settings) bool {
	r := a.route(n.route)
	if r.ID == "" || r.Direct {
		return true
	}
	g, ok := px[routes.Group(r)]
	if !ok || (g.Now != routes.Auto(r) && g.Now != routes.Backup(r)) {
		return true // a fixed pick, or another strategy: the user chose
	}
	if currentNode(px, r) != n.to {
		return true // the group moved again
	}
	if targets := s.TargetsOf(r.ID); len(targets) > 0 {
		if h, ok := speed.Last(px[n.from].HistoryOf(targets[0].URL), speed.StaleAfter(s.HealthInterval, s.URLTestInterval), time.Now()); ok && h.Delay > 0 {
			return true // the node it left works again
		}
	}
	return false
}

func (a *App) judge(on bool, s settings.Settings, cs core.Status, tk Takeover, snap Snapshot, sw *switchSay) (State, *Issue) {
	if !on {
		return StateOff, nil
	}
	if cs.State == core.Failed {
		return StateError, &Issue{Level: "error", Text: i18n.T("内核未运行", "The core isn't running"), Detail: firstLine(cs.Error)}
	}
	if cs.State == core.Running && cs.Error != "" {
		return StateError, &Issue{Level: "error", Text: i18n.T("代理端口打不开", "The proxy port can't be opened"), Detail: firstLine(cs.Error)}
	}
	if tk.System.Status == "error" {
		return StateError, &Issue{Level: "error", Text: i18n.T("没能接管系统代理", "Couldn't take over the system proxy"), Detail: tk.System.Detail, Action: "retake", Label: i18n.T("重新接管", "Take over again")}
	}
	if tk.System.Status == "hijacked" {
		return StateError, &Issue{Level: "error", Text: i18n.T("系统代理被其他程序改掉了", "Another program changed the system proxy"), Detail: i18n.T("网页流量现在没有经过 bowel。", "Web traffic isn't going through bowel now."), Action: "retake", Label: i18n.T("重新接管", "Take over again")}
	}
	if is := lockIssue(tk.Lock); is != nil {
		if is.Level == "error" {
			return StateError, is
		}
		return StateDegraded, is
	}
	if tk.Tun.Wanted && tk.Tun.Status == "error" && a.tunDown(cs) != "" {
		return StateDegraded, &Issue{Level: "warn", Text: i18n.T("TUN 没有生效", "TUN isn't working"), Detail: tk.Tun.Detail}
	}
	if g := tk.Gateway; g.Status == "error" {
		return StateDegraded, &Issue{Level: "warn", Text: i18n.T("网关模式没有生效", "Gateway mode isn't working"), Detail: g.Detail}
	}
	if names := lockedNames(snap.Routes); len(snap.Nodes) == 0 && (s.Mode != "direct" || names != "") {
		is := &Issue{Level: "warn", Text: i18n.T("还没有节点，流量正在直连", "No nodes yet; traffic is going direct"), Action: "providers", Label: i18n.T("添加提供商", "Add Provider")}
		if names != "" {
			// Not all of it: what the lock holds has nowhere to go.
			is.Detail = fmt.Sprintf(i18n.T("出口锁定正在拦下 %s 的连接，不会改走直连。添加节点，或解除出口锁定。", "The exit lock is refusing %s's connections rather than sending them direct. Add nodes, or unlock."), names)
		}
		return StateDegraded, is
	}
	// A plan that has ended is why its nodes are down: said before the
	// groups and switches that follow from it.
	if is := a.renewIssue(snap.Providers, provider.Expired, provider.Spent); is != nil {
		return StateDegraded, is
	}
	if sw != nil {
		return StateDegraded, &Issue{Level: "warn", Text: fmt.Sprintf(i18n.T("%s：%s 不可用，已换到 %s", "%s: %s is down, switched to %s"), sw.title, sw.from, sw.to), Action: "activity", Label: i18n.T("查看", "View")}
	}
	var broken []RouteView
	for _, r := range snap.Routes {
		// A locked group goes by its own node in every mode.
		if r.Broken != "" && (s.Mode == "rule" || r.ID == routes.Default || r.Locked) {
			broken = append(broken, r)
		}
	}
	// What the lock refuses outright is said before what merely can't connect.
	slices.SortStableFunc(broken, func(x, y RouteView) int {
		switch {
		case x.Locked == y.Locked:
			return 0
		case x.Locked:
			return -1
		}
		return 1
	})
	if len(broken) > 0 {
		return StateDegraded, a.brokenIssue(broken[0], snap.Nodes, len(broken)-1)
	}
	for _, r := range snap.Routes {
		if r.Direct || r.Node == "" {
			continue
		}
		for _, n := range snap.Nodes {
			if n = n.On(r.ID); n.Name == r.Node && n.Tested && n.Delay == 0 && (s.Mode == "rule" || r.ID == routes.Default) {
				text := fmt.Sprintf(i18n.T("%s：%s 测速失败", "%s: %s failed the speed test"), r.Name, n.Display)
				if n.Refused { // the node works, and the route's main site won't have it
					text = fmt.Sprintf(i18n.T("%s：%s 被 %s 拒绝", "%s: %s is refused by %s"), r.Name, n.Display, r.Main.Name)
				}
				return StateDegraded, &Issue{Level: "warn", Text: text, Action: "", Label: ""}
			}
		}
	}
	if is := a.renewIssue(snap.Providers, provider.Quota, provider.Expire); is != nil {
		return StateDegraded, is
	}
	if is := a.importedIssue(s, snap); is != nil {
		return StateOK, is
	}
	if is := a.dnsIssue(s, cs); is != nil {
		return StateDegraded, is
	}
	// The takeover works, so the light stays green; the user's list does not.
	if tk.System.Status == "on" && tk.System.Detail != "" {
		return StateOK, &Issue{Level: "warn", Text: tk.System.Detail, Detail: i18n.T("被拒绝的内容在日志和 bowel status --json 的 takeover.written 里。", "What was refused is in the log and in takeover.written of bowel status --json.")}
	}
	// Nothing is wrong yet, so the light stays green.
	if s.Tun && a.tunStale.Load() {
		return StateOK, &Issue{Level: "warn", Text: i18n.T("TUN 内核需要更新", "The TUN core needs updating"), Detail: i18n.T("bowel 升级后带了新内核，TUN 还在用旧的。授权一次就能换上，网络会中断几秒。", "This version of bowel has a newer core and TUN still runs the old one. One authorization replaces it; the network drops for a few seconds."), Action: "tun-update", Label: i18n.T("更新", "Update")}
	}
	if is := a.tunDNSIssue(s, cs, tk); is != nil {
		return StateOK, is
	}
	return StateOK, nil
}

func (a *App) providerView(p provider.Provider) ProviderView {
	v := ProviderView{ID: p.ID, Name: p.Name, Kind: string(p.Kind), Source: maskSource(p), Enabled: p.Enabled,
		AutoUpdate: p.AutoUpdate, UpdatedAt: p.UpdatedAt, Error: p.Error, Hidden: p.Hidden}
	v.Routing = a.providerRouting(p)
	if u := p.Usage; u != nil {
		v.Usage = &Usage{Upload: u.Upload, Download: u.Download, Total: u.Total, Expire: u.Expire}
		v.Warn = u.Warn(time.Now())
	}
	return v
}

// renewIssue is the reminder about the first enabled provider whose plan
// warns of one of warns, with the way to its website when there is one.
func (a *App) renewIssue(provs []ProviderView, warns ...string) *Issue {
	for _, pv := range provs {
		if !pv.Enabled || pv.Usage == nil || !slices.Contains(warns, pv.Warn) {
			continue
		}
		u := provider.Usage{Upload: pv.Usage.Upload, Download: pv.Usage.Download, Total: pv.Usage.Total, Expire: pv.Usage.Expire}
		text, detail := provider.Reminder(pv.Name, pv.Warn, &u, time.Now())
		is := &Issue{Level: "warn", Text: text, Detail: detail, Action: "providers", Label: i18n.T("查看", "View")}
		if a.providers == nil {
			return is
		}
		if p, ok := a.providers.Get(pv.ID); ok {
			if site := a.providers.Site(p); site != "" {
				is.Action, is.Label = "site:"+site, i18n.T("去续费", "Renew")
			}
		}
		return is
	}
	return nil
}

// maskSource hides a subscription's token: only scheme and host are shown.
func maskSource(p provider.Provider) string {
	switch p.Kind {
	case provider.Subscription:
		if u, err := url.Parse(p.Source); err == nil {
			return u.Scheme + "://" + u.Host + "/…"
		}
	case provider.File:
		return p.Source
	}
	return ""
}

func located(r locate.Result) Located {
	l := Located{Source: r.Source, IP: r.IP}
	if r.Place != nil {
		l.Place = &Place{Country: r.Place.Country, City: geo.Display(r.Place.City), Flag: r.Place.Flag, Lat: r.Place.Lat, Lon: r.Place.Lon}
	}
	return l
}

var _ = geo.Place{} // locate's places are geo.Places

// lanAddr is where other devices on the LAN reach the mixed port.
func lanAddr(port int) string {
	ifs, _ := net.Interfaces()
	for _, ifc := range ifs {
		addrs, _ := ifc.Addrs()
		if ip := lanIP(ifc.Flags, addrs); ip != "" {
			return fmt.Sprintf("%s:%d", ip, port)
		}
	}
	return ""
}

// lanIP is an interface's first private IPv4 address, "" when the
// interface is down: one whose cable is out can keep its address, on
// Windows, and nothing answers there.
func lanIP(flags net.Flags, addrs []net.Addr) string {
	if flags&net.FlagUp == 0 {
		return ""
	}
	for _, ad := range addrs {
		if ip, ok := ad.(*net.IPNet); ok && ip.IP.To4() != nil && !ip.IP.IsLoopback() && ip.IP.IsPrivate() {
			return ip.IP.String()
		}
	}
	return ""
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return s
}

func copyMap[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
