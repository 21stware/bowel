package main

// The gateway's second step. Manual is the first: the gateway answers, and
// devices are pointed at it one by one. Automatic is bowel handing out the
// LAN's addresses itself (DHCP), with this computer as every device's
// router and DNS, so that all of them come by themselves.
//
// Two servers handing out addresses on one LAN fight over every device,
// so bowel starts its own only after a check found no other, and the check
// is what the interface shows between the two steps.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/oboo/moho/internal/dhcp"
	"github.com/oboo/moho/internal/gateway"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/settings"
)

const (
	// dhcpWait is how long a check listens for other servers. Many routers
	// (dnsmasq) ping an address for three seconds before they offer it, so
	// anything shorter finds nobody.
	dhcpWait    = 5 * time.Second
	dhcpRecheck = 5 * time.Minute // while serving: has another server come back?
	// dhcpClear is how long a clear answer stands before serving on it.
	dhcpClear = 30 * time.Second
)

// dhcpRetry is how often a LAN that was not clear to serve is asked again.
var dhcpRetry = 30 * time.Second

// osDHCP is the LAN as the automatic step sees it. Tests replace it, so
// that none sends a packet.
var osDHCP = struct {
	supported bool
	probe     func(ctx context.Context, iface string, self netip.Addr, wait time.Duration) ([]netip.Addr, error)
	listen    func(*dhcp.Server) (net.PacketConn, error)
	// lan is this computer's address on the network n is, and that network.
	lan func(n gateway.Network) (self netip.Addr, lan netip.Prefix, ok bool)
	// neighbours is the system's ARP table.
	neighbours func() map[netip.Addr]string
}{dhcp.Supported, dhcp.Probe, (*dhcp.Server).Listen, lanPrefix, gateway.Neighbours}

// gwAuto is the automatic step as it runs.
//
// mu is never held across a call into the server, and the server calls
// back (InUse) while it holds its own lock: what it calls takes neighMu
// only, which is never held while the system is asked for anything. So
// neither waits for the other, nor the server for a slow ARP table.
type gwAuto struct {
	mu     sync.Mutex
	srv    *dhcp.Server
	cancel context.CancelFunc
	// starts counts the servers started: a check that saw none when it
	// began knows by it whether one answered while it listened.
	starts   int
	key      string       // the interface and addresses srv serves
	lan      string       // the router identity srv was started for
	err      error        // why it is wanted and does not serve
	others   []netip.Addr // the other servers the last successful check found
	checked  time.Time
	check    dhcpCheck
	checking bool
	// epoch changes when automatic is requested or stopped, so a pending
	// enable or resume cannot undo a later choice.
	epoch uint64

	// Before the server starts anywhere, the LAN is asked whether someone
	// hands out addresses there already: clear is the LAN (key) a check
	// just found free, and tried when a LAN was last asked for this.
	clear    string
	clearLAN string
	clearAt  time.Time
	starting bool
	tried    time.Time

	// The ARP table as last read: neighMu is for the copy kept here, and
	// neighRead is held while the system is asked, one reader at a time.
	neighRead sync.Mutex
	neighMu   sync.Mutex
	neighAt   time.Time
	neigh     map[netip.Addr]string
}

// errLeased is why automatic is not for a computer whose own address comes
// from DHCP: the router's DHCP has to be off for it, and then nobody renews
// this computer's lease. When it ends the address is gone, and with it the
// LAN's router, DNS and DHCP all at once.
func errLeased() error {
	return errors.New(i18n.T("这台电脑的地址是自动获取的（DHCP）。先在系统的网络设置里给它设一个手动的固定地址，再开启：路由器的 DHCP 关掉后，自动获取的地址到期就会丢，整个局域网跟着断网",
		"This computer gets its address from DHCP. Give it a fixed (manual) address in the system's network settings first: with the router's DHCP off, a leased address is lost when the lease ends, and the whole LAN goes down with it"))
}

// errNoRouter is why automatic is not for a LAN whose router is not known:
// the router is where every device's traffic leaves, the one address never
// to hand out, and what tells this LAN from another with the same numbers.
func errNoRouter() error {
	return errors.New(i18n.T("找不到这个局域网的路由器（默认网关），没法分配地址：先在这台电脑的网络设置里填上路由器地址",
		"This LAN's router (the default gateway) isn't known, so no addresses can be handed out: set the router's address in this computer's network settings first"))
}

func errOtherLAN() error {
	return errors.New(i18n.T("这不是开启时的那个局域网，bowel 在这里不分配地址。要在这里用，先关闭再重新开启", "This isn't the LAN it was turned on for, so bowel hands out no addresses here. To use it here, turn it off and on again"))
}

// lanID names a LAN so that another one is told from it: its network, its
// router and the router's hardware address, which is what tells apart the
// many LANs that are all 192.168.1.0/24 behind 192.168.1.1.
func (a *App) lanID(n gateway.Network, lan netip.Prefix) string {
	id := lan.String() + " " + n.Router
	if r, err := netip.ParseAddr(n.Router); err == nil {
		if mac := a.neighbours()[r]; mac != "" {
			id += " " + mac
		}
	}
	return id
}

// sameLAN compares two lanIDs. One without the router's hardware address
// (not learnt yet) matches by network and router alone.
func sameLAN(a, b string) bool {
	fa, fb := strings.Fields(a), strings.Fields(b)
	if len(fa) < 2 || len(fb) < 2 || fa[0] != fb[0] || fa[1] != fb[1] {
		return false
	}
	return len(fa) < 3 || len(fb) < 3 || fa[2] == fb[2]
}

// lanPrefix is the LAN n is on: the network of its address on its interface.
func lanPrefix(n gateway.Network) (self netip.Addr, lan netip.Prefix, ok bool) {
	ifc, err := net.InterfaceByName(n.Iface)
	if err != nil || n.IP == "" {
		return
	}
	self, err = netip.ParseAddr(n.IP)
	if err != nil {
		return
	}
	addrs, _ := ifc.Addrs()
	for _, ad := range addrs {
		if p, err := netip.ParsePrefix(ad.String()); err == nil && p.Addr().Unmap() == self {
			return self, netip.PrefixFrom(self, p.Bits()).Masked(), true
		}
	}
	return
}

// dhcpCheck carries the network that was actually asked with its answer,
// including when a caller joins a check already in progress.
type dhcpCheck struct {
	key, lan string
	others   []netip.Addr
	err      error
}

// checkDHCP asks the LAN who hands out addresses, and keeps the answer.
func (a *App) checkDHCP() ([]netip.Addr, error) {
	c := a.probeDHCP()
	return c.others, c.err
}

func (a *App) probeDHCP() dhcpCheck {
	au := &a.auto
	au.mu.Lock()
	if au.checking { // one at a time; the other's answer will do
		au.mu.Unlock()
		for range 100 {
			time.Sleep(100 * time.Millisecond)
			au.mu.Lock()
			done := !au.checking
			c := au.check
			au.mu.Unlock()
			if done {
				return c
			}
		}
		return dhcpCheck{err: errors.New(i18n.T("检测还没有结束", "The check hasn't finished yet"))}
	}
	au.checking = true
	serving, starts := au.srv != nil, au.starts
	au.mu.Unlock()
	a.emit()

	var others []netip.Addr
	var err error
	n := a.gw.System().Network()
	self, lan, ok := osDHCP.lan(n)
	c := dhcpCheck{}
	if ok {
		c.key, c.lan = lanKey(n, self, lan), a.lanID(n, lan)
	}
	me := self
	switch {
	case !osDHCP.supported:
		err = errors.New(i18n.T("这个系统上还不能分配地址", "Handing out addresses isn't available on this system yet"))
	case !ok:
		err = errors.New(i18n.T("这台电脑没有连上局域网", "This computer isn't on a LAN"))
	default:
		// This computer's address is left out only while bowel's own server
		// answers there: at any other time an answer from it is another
		// program's server on this computer, which is one to report.
		if !serving {
			self = netip.Addr{}
		}
		ctx, cancel := context.WithTimeout(context.Background(), dhcpWait+2*time.Second)
		others, err = osDHCP.probe(ctx, n.Iface, self, dhcpWait)
		cancel()
		if err != nil {
			err = fmt.Errorf(i18n.T("没法检测局域网里的 DHCP：%s", "Couldn't check the LAN for DHCP: %s"), dhcpWhy(err))
		}
	}
	au.mu.Lock()
	// A server of bowel's that started while the check listened answered it
	// too: that one is not another server.
	if !serving && (au.srv != nil || au.starts != starts) {
		others = slices.DeleteFunc(others, func(x netip.Addr) bool { return x == me })
	}
	c.others, c.err = others, err
	au.checking, au.checked, au.check = false, time.Now(), c
	if err == nil {
		au.others = others
	}
	au.mu.Unlock()
	a.emit()
	return c
}

// dhcpWhy is an error of the DHCP sockets in the user's words.
func dhcpWhy(err error) string {
	s := err.Error()
	if strings.Contains(s, "permission denied") || strings.Contains(s, "operation not permitted") {
		return i18n.T("需要以 root 运行（例如把 bowel daemon 作为系统服务）", "it needs to run as root (bowel daemon as a system service, for one)")
	}
	if strings.Contains(s, "address already in use") {
		return i18n.T("这台电脑上已经有程序占用了 DHCP 端口", "another program on this computer holds the DHCP port")
	}
	return s
}

// setGatewayAuto turns the automatic step on or off. On needs the gateway
// in effect and no other server on the LAN: it checks first and says who
// still hands out addresses.
func (a *App) setGatewayAuto(on bool) error {
	done := a.begin("gateway:auto")
	defer done()
	au := &a.auto
	au.mu.Lock()
	au.epoch++
	epoch := au.epoch
	au.clear, au.clearLAN = "", ""
	au.mu.Unlock()
	var check dhcpCheck
	if on {
		if !a.gatewayOn(a.settings.Get()) {
			return errors.New(i18n.T("先打开网关模式", "Turn gateway mode on first"))
		}
		// Before anything is asked of the LAN: what about this computer
		// rules it out whoever else answers.
		if n := a.gw.System().Network(); n.IP != "" {
			if n.DHCP {
				return errLeased()
			}
			if n.Router == "" {
				return errNoRouter()
			}
		}
		check = a.probeDHCP()
		if check.err != nil {
			return check.err
		}
		if len(check.others) > 0 {
			return fmt.Errorf(i18n.T("%s 还在分配地址：先关闭它的 DHCP 服务，再重新检测", "%s still hands out addresses: turn its DHCP service off, then check again"), joinAddrs(check.others))
		}
	}
	id := ""
	if on {
		// The LAN the user said yes on: automatic serves nowhere else.
		n := a.gw.System().Network()
		self, lan, ok := osDHCP.lan(n)
		if !ok {
			return errors.New(i18n.T("这台电脑没有连上局域网", "This computer isn't on a LAN"))
		}
		// Once more, on the network it is about to be remembered for.
		if n.DHCP {
			return errLeased()
		}
		if n.Router == "" {
			return errNoRouter()
		}
		id = a.lanID(n, lan)
		if check.key != lanKey(n, self, lan) || !sameLAN(check.lan, id) {
			return errOtherLAN()
		}
	}
	au.mu.Lock()
	if au.epoch != epoch || on && a.isQuitting() {
		au.mu.Unlock()
		return context.Canceled
	}
	stopped := false
	_, err := a.settings.Update(func(s *settings.Settings) {
		if on && !a.gatewayOn(*s) {
			stopped = true
			return
		}
		s.GatewayAuto, s.GatewayAutoLAN = on, id
	})
	if err == nil && !stopped && on {
		au.clear, au.clearLAN, au.clearAt = check.key, check.lan, time.Now()
	}
	au.mu.Unlock()
	if err != nil {
		return err
	}
	if stopped {
		return context.Canceled
	}
	a.syncDHCP()
	a.emit()
	a.auto.mu.Lock()
	defer a.auto.mu.Unlock()
	return a.auto.err
}

func joinAddrs(as []netip.Addr) string {
	ss := make([]string, len(as))
	for i, x := range as {
		ss[i] = x.String()
	}
	return strings.Join(ss, ", ")
}

// lanKey is what a server serves: the interface, this computer's address
// there and the network.
func lanKey(n gateway.Network, self netip.Addr, lan netip.Prefix) string {
	return n.Iface + "|" + self.String() + "|" + lan.String()
}

// syncDHCP makes the server follow the settings and the LAN: serving while
// the automatic step is wanted and the gateway in effect, on the LAN this
// computer is on now.
//
// It never starts unasked on a LAN where it could do harm. The LAN must be
// the one the user turned automatic on for (a laptop carried to another
// network offers itself to nobody there), and each time before it starts
// (at launch, after a pause, when this computer's address changed) the LAN
// is asked again whether someone hands out addresses: while one answers,
// bowel stays out and says so. And this computer's own address must be a
// fixed one: once it is found to come from DHCP the server stops, since it
// would go down with the lease anyway and must not answer its own client.
func (a *App) syncDHCP() {
	s := a.settings.Get()
	want := s.GatewayAuto && a.gatewayOn(s) && !a.isQuitting()
	au := &a.auto
	var n gateway.Network
	var self netip.Addr
	var lan netip.Prefix
	key, id := "", ""
	if want {
		n = a.gw.System().Network()
		var ok bool
		if self, lan, ok = osDHCP.lan(n); ok {
			key, id = lanKey(n, self, lan), a.lanID(n, lan)
		}
	}
	au.mu.Lock()
	defer au.mu.Unlock()
	// Network reads can wait behind a stop. Decide from the settings now,
	// under the same lock that the stop takes, not from the earlier copy.
	s = a.settings.Get()
	want = s.GatewayAuto && a.gatewayOn(s) && !a.isQuitting()
	if au.srv != nil && (!want || key != au.key || n.DHCP || n.Router == "" || !sameLAN(au.lan, id) || !sameLAN(s.GatewayAutoLAN, id)) {
		au.cancel()
		au.srv, au.cancel, au.key, au.lan = nil, nil, "", ""
		au.clear, au.clearLAN = "", ""
	}
	if !want {
		au.epoch++
		au.clear, au.clearLAN = "", ""
		au.err = nil
		return
	}
	if au.srv != nil {
		return
	}
	if key == "" {
		au.err = errors.New(i18n.T("这台电脑没有连上局域网", "This computer isn't on a LAN"))
		return
	}
	if n.Router == "" {
		au.err = errNoRouter()
		return
	}
	if !sameLAN(s.GatewayAutoLAN, id) {
		au.err = errOtherLAN()
		return
	}
	if n.DHCP {
		au.err = errLeased()
		return
	}
	if au.clear != key || !sameLAN(au.clearLAN, id) || time.Since(au.clearAt) > dhcpClear {
		// Not known to be free: ask, and come back with the answer.
		if !au.starting && time.Since(au.tried) > dhcpRetry {
			au.starting, au.tried = true, time.Now()
			go a.clearToServe(key)
		}
		return
	}
	au.clear = "" // good for this start only
	cfg := dhcp.Config{Iface: n.Iface, Self: self, Prefix: lan, Path: filepath.Join(a.dataDir, "gateway-leases.json"),
		InUse: a.addressInUse, Refresh: func() { a.neighbours() }, OnChange: func() { a.updateDevices(); a.emit() }}
	if ifc, err := net.InterfaceByName(n.Iface); err == nil {
		cfg.Own = ifc.HardwareAddr
	}
	if r, err := netip.ParseAddr(n.Router); err == nil {
		cfg.Avoid = []netip.Addr{r}
	}
	srv, err := dhcp.New(cfg)
	var conn net.PacketConn
	if err == nil {
		conn, err = osDHCP.listen(srv)
	}
	if err != nil {
		au.err = fmt.Errorf(i18n.T("没法开始分配地址：%s", "Couldn't start handing out addresses: %s"), dhcpWhy(err))
		log.Println("gateway:", au.err)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	au.srv, au.cancel, au.key, au.lan, au.err = srv, cancel, key, id, nil
	au.starts++
	go func() {
		if err := srv.Serve(ctx, conn); err != nil {
			log.Println("gateway: handing out addresses stopped:", err)
			au.mu.Lock()
			if au.srv == srv {
				au.srv, au.cancel, au.key, au.lan = nil, nil, "", ""
				au.err = fmt.Errorf(i18n.T("地址分配意外停止：%s", "Handing out addresses stopped unexpectedly: %s"), err)
			}
			au.mu.Unlock()
			a.emit()
		}
	}()
	first, last := srv.Pool()
	log.Printf("gateway: handing out addresses on %s (%s), new devices from %s to %s", n.Iface, lan, first, last)
}

// clearToServe asks the LAN whether someone hands out addresses, for the
// server that is about to start on key, and has syncDHCP start it if not.
func (a *App) clearToServe(key string) {
	au := &a.auto
	au.mu.Lock()
	epoch := au.epoch
	au.mu.Unlock()
	check := a.probeDHCP()
	au.mu.Lock()
	au.starting = false
	if au.epoch != epoch {
		au.mu.Unlock()
		a.emit()
		return
	}
	au.clear, au.clearLAN = "", ""
	switch {
	case check.err != nil:
		au.err = check.err
	case check.key != key || !sameLAN(a.settings.Get().GatewayAutoLAN, check.lan):
		au.err = errOtherLAN()
	case len(check.others) > 0:
		au.err = fmt.Errorf(i18n.T("%s 在分配地址，bowel 没有开始分配。关闭它的 DHCP 后会自动继续", "%s hands out addresses, so bowel didn't start to. It goes on by itself once that DHCP is off"), joinAddrs(check.others))
	default:
		au.clear, au.clearLAN, au.clearAt = check.key, check.lan, time.Now()
	}
	clear := au.clear == key
	au.mu.Unlock()
	if clear {
		a.syncDHCP()
	}
	a.emit()
}

// stopDHCP ends the server, for when bowel quits.
func (a *App) stopDHCP() {
	au := &a.auto
	au.mu.Lock()
	defer au.mu.Unlock()
	au.epoch++
	au.clear, au.clearLAN = "", ""
	if au.srv != nil {
		au.cancel()
		au.srv, au.cancel, au.key, au.lan = nil, nil, "", ""
	}
}

// addressInUse says whether a device other than mac's answers at ip, by
// the ARP table as it was last read. The server calls it with its lock
// held, so it asks the system nothing: the server has the table read
// (Refresh, which is neighbours) before it takes its lock.
func (a *App) addressInUse(ip netip.Addr, mac string) bool {
	au := &a.auto
	au.neighMu.Lock()
	have, ok := au.neigh[ip]
	au.neighMu.Unlock()
	return ok && have != mac
}

// neighbours is the ARP table, read at most every few seconds. Reading it
// can take a while (macOS runs arp for it), so it is not to be called
// under a lock that others wait for; while one caller reads, the others
// wait for that reading.
func (a *App) neighbours() map[netip.Addr]string {
	au := &a.auto
	au.neighRead.Lock()
	defer au.neighRead.Unlock()
	au.neighMu.Lock()
	m, fresh := au.neigh, !au.neighAt.IsZero() && time.Since(au.neighAt) < 10*time.Second
	au.neighMu.Unlock()
	if fresh {
		return m
	}
	m = osDHCP.neighbours()
	au.neighMu.Lock()
	au.neigh, au.neighAt = m, time.Now()
	au.neighMu.Unlock()
	return m
}

// leases are the server's, none when it does not run.
func (a *App) leases() []dhcp.Lease {
	a.auto.mu.Lock()
	srv := a.auto.srv
	a.auto.mu.Unlock()
	if srv == nil {
		return nil
	}
	return srv.Leases()
}

// updateDevices brings the record of devices up to now.
func (a *App) updateDevices() {
	if !a.settings.Get().Gateway {
		return
	}
	locals := gateway.LocalAddrs()
	var sources []netip.Addr
	for _, s := range a.mon.Sources() {
		ip, err := netip.ParseAddr(s)
		if err != nil {
			continue
		}
		ip = ip.Unmap()
		if ip.IsLoopback() || ip.IsUnspecified() || containsAddr(locals, ip) {
			continue
		}
		sources = append(sources, ip)
	}
	a.devs.update(time.Now(), sources, a.leases(), a.neighbours())
}

func containsAddr(list []netip.Addr, a netip.Addr) bool {
	for _, x := range list {
		if x == a {
			return true
		}
	}
	return false
}

// tendAuto is the automatic step's share of the gateway's regular look: a
// server that should run and does not is started (the LAN came up later,
// or its address changed), and while it serves the LAN is checked now and
// then for a second server, which would be the router's switched back on.
func (a *App) tendAuto() {
	s := a.settings.Get()
	if !a.gatewayOn(s) {
		return
	}
	// The first look at the LAN, so that the interface can say at once
	// what stands between manual and automatic.
	a.auto.mu.Lock()
	first := osDHCP.supported && a.auto.checked.IsZero() && !a.auto.checking
	a.auto.mu.Unlock()
	if first {
		go func() { _, _ = a.checkDHCP() }()
	}
	if !s.GatewayAuto {
		return
	}
	a.syncDHCP()
	a.auto.mu.Lock()
	due := a.auto.srv != nil && !a.auto.checking && time.Since(a.auto.checked) > dhcpRecheck
	a.auto.mu.Unlock()
	if due {
		go func() { _, _ = a.checkDHCP() }()
	}
}

// autoView is the automatic step as the interface shows it.
func (a *App) autoView(s settings.Settings, gatewayOn bool) AutoView {
	au := &a.auto
	au.mu.Lock()
	srv, serveErr, starting := au.srv, au.err, au.starting
	others := slices.Clone(au.others)
	v := AutoView{Supported: osDHCP.supported, Wanted: s.GatewayAuto && s.Gateway, Status: "off", Servers: []string{}, Checked: au.checked, Checking: au.checking}
	for _, o := range others {
		v.Servers = append(v.Servers, o.String())
	}
	if au.check.err != nil {
		v.CheckError = au.check.err.Error()
	}
	// The server is asked with au.mu let go: it holds its own lock while
	// it asks who answers at an address, which comes back to this side.
	au.mu.Unlock()
	if !v.Wanted {
		return v
	}
	switch {
	case srv != nil:
		v.Status = "on"
		first, last := srv.Pool()
		v.Pool = first.String() + " – " + last.String()
		now := time.Now()
		for _, l := range srv.Leases() {
			if l.Active(now) {
				v.Leases++
			}
		}
		if len(others) > 0 {
			v.Status = "error"
			v.Detail = fmt.Sprintf(i18n.T("%s 也在分配地址，设备会在两边之间来回", "%s hands out addresses too, so devices go back and forth between the two"), joinAddrs(others))
		}
	case !gatewayOn:
		v.Detail = i18n.T("网关暂停了，地址分配也暂停", "The gateway is paused, and so is handing out addresses")
		v.Status = "error"
	case starting && serveErr == nil:
		v.Status = "starting"
		v.Detail = i18n.T("正在确认局域网里没有别的 DHCP 服务器…", "Making sure no other DHCP server is on the LAN…")
	default:
		v.Status = "error"
		if serveErr != nil {
			v.Detail = serveErr.Error()
		}
	}
	return v
}
