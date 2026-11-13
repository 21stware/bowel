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
)

// osDHCP is the LAN as the automatic step sees it. Tests replace it, so
// that none sends a packet.
var osDHCP = struct {
	supported bool
	probe     func(ctx context.Context, iface string, self netip.Addr, wait time.Duration) ([]netip.Addr, error)
	listen    func(*dhcp.Server) (net.PacketConn, error)
	// lan is this computer's address on the network n is, and that network.
	lan func(n gateway.Network) (self netip.Addr, lan netip.Prefix, ok bool)
}{dhcp.Supported, dhcp.Probe, (*dhcp.Server).Listen, lanPrefix}

// gwAuto is the automatic step as it runs.
type gwAuto struct {
	mu       sync.Mutex
	srv      *dhcp.Server
	cancel   context.CancelFunc
	key      string       // the interface and addresses srv serves
	err      error        // why it is wanted and does not serve
	others   []netip.Addr // the other servers the last check found
	checked  time.Time
	checkErr error
	checking bool

	neighAt time.Time
	neigh   map[netip.Addr]string
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

// checkDHCP asks the LAN who hands out addresses, and keeps the answer.
func (a *App) checkDHCP() ([]netip.Addr, error) {
	au := &a.auto
	au.mu.Lock()
	if au.checking { // one at a time; the other's answer will do
		au.mu.Unlock()
		for range 100 {
			time.Sleep(100 * time.Millisecond)
			au.mu.Lock()
			done := !au.checking
			others, err := au.others, au.checkErr
			au.mu.Unlock()
			if done {
				return others, err
			}
		}
		return nil, errors.New(i18n.T("检测还没有结束", "The check hasn't finished yet"))
	}
	au.checking = true
	serving := au.srv != nil
	au.mu.Unlock()
	a.emit()

	var others []netip.Addr
	var err error
	n := a.gw.System().Network()
	self, _, ok := osDHCP.lan(n)
	switch {
	case !osDHCP.supported:
		err = errors.New(i18n.T("这个系统上还不能分配地址", "Handing out addresses isn't available on this system yet"))
	case !ok:
		err = errors.New(i18n.T("这台电脑没有连上局域网", "This computer isn't on a LAN"))
	default:
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
	au.checking, au.checked, au.checkErr = false, time.Now(), err
	if err == nil {
		au.others = others
	}
	au.mu.Unlock()
	a.emit()
	return others, err
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
	if on {
		if !a.gatewayOn(a.settings.Get()) {
			return errors.New(i18n.T("先打开网关模式", "Turn gateway mode on first"))
		}
		others, err := a.checkDHCP()
		if err != nil {
			return err
		}
		if len(others) > 0 {
			return fmt.Errorf(i18n.T("%s 还在分配地址：先关闭它的 DHCP 服务，再重新检测", "%s still hands out addresses: turn its DHCP service off, then check again"), joinAddrs(others))
		}
	}
	if _, err := a.settings.Update(func(s *settings.Settings) { s.GatewayAuto = on }); err != nil {
		return err
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

// syncDHCP makes the server follow the settings and the LAN: serving while
// the automatic step is wanted and the gateway in effect, on the LAN this
// computer is on now.
func (a *App) syncDHCP() {
	s := a.settings.Get()
	want := s.GatewayAuto && a.gatewayOn(s) && !a.isQuitting()
	au := &a.auto
	var n gateway.Network
	var self netip.Addr
	var lan netip.Prefix
	key := ""
	if want {
		n = a.gw.System().Network()
		var ok bool
		if self, lan, ok = osDHCP.lan(n); ok {
			key = n.Iface + "|" + self.String() + "|" + lan.String()
		}
	}
	au.mu.Lock()
	defer au.mu.Unlock()
	if au.srv != nil && (!want || key != au.key) {
		au.cancel()
		au.srv, au.cancel, au.key = nil, nil, ""
	}
	if !want {
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
	cfg := dhcp.Config{Iface: n.Iface, Self: self, Prefix: lan, Path: filepath.Join(a.dataDir, "gateway-leases.json"),
		InUse: a.addressInUse, OnChange: func() { a.updateDevices(); a.emit() }}
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
	au.srv, au.cancel, au.key, au.err = srv, cancel, key, nil
	go func() {
		if err := srv.Serve(ctx, conn); err != nil {
			log.Println("gateway: handing out addresses stopped:", err)
			au.mu.Lock()
			if au.srv == srv {
				au.srv, au.cancel, au.key = nil, nil, ""
				au.err = fmt.Errorf(i18n.T("地址分配意外停止：%s", "Handing out addresses stopped unexpectedly: %s"), err)
			}
			au.mu.Unlock()
			a.emit()
		}
	}()
	first, last := srv.Pool()
	log.Printf("gateway: handing out addresses on %s (%s), new devices from %s to %s", n.Iface, lan, first, last)
}

// stopDHCP ends the server, for when bowel quits.
func (a *App) stopDHCP() {
	au := &a.auto
	au.mu.Lock()
	defer au.mu.Unlock()
	if au.srv != nil {
		au.cancel()
		au.srv, au.cancel, au.key = nil, nil, ""
	}
}

// addressInUse says whether some device answers at ip, by the system's ARP
// table, read at most every few seconds.
func (a *App) addressInUse(ip netip.Addr) bool {
	_, ok := a.neighbours()[ip]
	return ok
}

func (a *App) neighbours() map[netip.Addr]string {
	au := &a.auto
	au.mu.Lock()
	fresh := time.Since(au.neighAt) < 10*time.Second
	m := au.neigh
	au.mu.Unlock()
	if fresh {
		return m
	}
	m = gateway.Neighbours()
	au.mu.Lock()
	au.neigh, au.neighAt = m, time.Now()
	au.mu.Unlock()
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
	defer au.mu.Unlock()
	v := AutoView{Supported: osDHCP.supported, Wanted: s.GatewayAuto && s.Gateway, Status: "off", Servers: []string{}, Checked: au.checked, Checking: au.checking}
	for _, o := range au.others {
		v.Servers = append(v.Servers, o.String())
	}
	if au.checkErr != nil {
		v.CheckError = au.checkErr.Error()
	}
	if !v.Wanted {
		return v
	}
	switch {
	case au.srv != nil:
		v.Status = "on"
		first, last := au.srv.Pool()
		v.Pool = first.String() + " – " + last.String()
		now := time.Now()
		for _, l := range au.srv.Leases() {
			if l.Active(now) {
				v.Leases++
			}
		}
		if len(au.others) > 0 {
			v.Status = "error"
			v.Detail = fmt.Sprintf(i18n.T("%s 也在分配地址，设备会在两边之间来回", "%s hands out addresses too, so devices go back and forth between the two"), joinAddrs(au.others))
		}
	case !gatewayOn:
		v.Detail = i18n.T("网关暂停了，地址分配也暂停", "The gateway is paused, and so is handing out addresses")
		v.Status = "error"
	default:
		v.Status = "error"
		if au.err != nil {
			v.Detail = au.err.Error()
		}
	}
	return v
}
