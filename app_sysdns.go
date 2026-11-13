package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/devnet"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/settings"
	"github.com/oboo/moho/internal/sysdns"
)

// osDNS is what reads and sets the system's DNS servers. Tests replace it,
// so that they can never change the machine's settings.
var osDNS = struct {
	// supported: the system's DNS servers are bowel's to set under TUN.
	supported bool
	snapshot  func(context.Context) (sysdns.Saved, error)
	write     func(context.Context, []sysdns.Service) error
	// dhcp are the servers the network hands out on an interface.
	dhcp func(ctx context.Context, iface string) []string
}{sysdns.Supported, sysdns.Snapshot, sysdns.Write, sysdns.DHCP}

// dnsBackup is every network service's DNS servers as they were before
// bowel set them for TUN. It is kept on disk, so that a crash does not
// lose it: the next launch puts them back from it.
func (a *App) dnsBackup() string { return filepath.Join(a.dataDir, "dns-before.json") }

func (a *App) readDNSBackup() (sysdns.Saved, bool) {
	var saved sysdns.Saved
	b, err := os.ReadFile(a.dnsBackup())
	if err != nil || json.Unmarshal(b, &saved) != nil {
		return sysdns.Saved{}, false
	}
	return saved, true
}

// tunOn says whether TUN is in the core's config with settings s.
func (a *App) tunOn(s settings.Settings) bool {
	return s.Tun && a.takeoverOn(s) && osTun.installed(a.tunBin) && a.tunFault.Load() == nil
}

// dnsWanted says whether the system's DNS servers are to point into TUN:
// where bowel can set them, while TUN is on with something to hijack, and
// unless the user keeps them.
func (a *App) dnsWanted(s settings.Settings) bool {
	return osDNS.supported && !s.TunKeepDNS && len(s.DNSHijack) > 0 && a.tunOn(s)
}

// applySystemDNS makes the system's DNS servers match the settings: the
// placeholder while TUN runs, what they were otherwise (also after a crash
// left the placeholder there). It comes round with devLoop too, for a
// service that appeared since, and a restore that did not finish.
func (a *App) applySystemDNS() {
	if !osDNS.supported {
		return
	}
	a.dnsMu.Lock()
	defer a.dnsMu.Unlock()
	if a.isQuitting() {
		return // shutdown has put the servers back: they stay so
	}
	s := a.settings.Get()
	if a.syncDNS(context.Background(), a.dnsWanted(s), a.core.Status().State == core.Running) {
		// What the system asks now is part of what the status tells.
		go func() {
			a.detectDev(8 * time.Second)
			a.refresh()
		}()
	}
}

// syncDNS does applySystemDNS's work, and reports whether it wrote
// anything: wanted is whether the servers are to point into TUN, running
// whether the core, and so its TUN, is there to answer.
func (a *App) syncDNS(ctx context.Context, wanted, running bool) (changed bool) {
	var err error
	switch {
	case !wanted:
		a.dnsHeld.Store(nil)
		if exists(a.dnsBackup()) {
			changed, err = a.restoreDNS(ctx)
		}
	case running:
		if changed, err = a.takeDNS(ctx); err == nil && (changed || a.dnsHeld.Load() == nil) {
			now := time.Now()
			a.dnsHeld.Store(&now)
		}
	default:
		return false // the core is on its way: its TUN is not there to answer yet
	}
	var why *string
	if err != nil {
		log.Println("system dns:", err)
		line := firstLine(err.Error())
		why = &line
	}
	a.dnsErr.Store(why)
	return changed
}

// takeDNS gives every enabled service the placeholder, after keeping what
// each had; a service that has the placeholder already keeps its first
// copy. It reports whether it wrote anything.
func (a *App) takeDNS(ctx context.Context) (bool, error) {
	cur, err := osDNS.snapshot(ctx)
	if err != nil {
		return false, fmt.Errorf("reading the DNS servers: %w", err)
	}
	old, had := a.readDNSBackup()
	keep := sysdns.Keep(old, cur)
	if !had || !reflect.DeepEqual(old, keep) {
		b, err := json.MarshalIndent(keep, "", "  ")
		if err != nil {
			return false, err
		}
		if err := os.WriteFile(a.dnsBackup(), b, 0o600); err != nil {
			return false, err
		}
	}
	steps := sysdns.TakeSteps(cur)
	if len(steps) == 0 {
		return false, nil
	}
	if err := osDNS.write(ctx, steps); err != nil {
		return true, err
	}
	// networksetup can refuse without saying so: what counts is what is set.
	if now, err := osDNS.snapshot(ctx); err == nil && !sysdns.HeldAll(now) {
		var left []string
		for _, sv := range now.Services {
			if !sysdns.Held(sv) {
				left = append(left, sv.Name)
			}
		}
		return true, fmt.Errorf(i18n.T("系统没有接受新的 DNS 服务器：%s", "The system didn't take the new DNS servers: %s"), strings.Join(left, ", "))
	}
	return true, nil
}

// restoreDNS puts back what takeDNS kept, on the services that still have
// the placeholder, and reports whether it wrote anything. The copy goes
// once that is done; after a failure it stays, for the next round or the
// next launch to finish from.
func (a *App) restoreDNS(ctx context.Context) (bool, error) {
	saved, _ := a.readDNSBackup() // unreadable: back to what the network hands out
	cur, err := osDNS.snapshot(ctx)
	if err != nil {
		return false, fmt.Errorf("reading the DNS servers: %w", err)
	}
	steps := sysdns.RestoreSteps(saved, cur)
	if len(steps) > 0 {
		if err := osDNS.write(ctx, steps); err != nil {
			return true, err
		}
	}
	return len(steps) > 0, os.Remove(a.dnsBackup())
}

// quitSystemDNS puts the system's DNS servers back before bowel goes: TUN
// goes with it, and nothing would answer for the placeholder in its name.
func (a *App) quitSystemDNS(ctx context.Context) {
	if !osDNS.supported {
		return
	}
	a.dnsMu.Lock()
	defer a.dnsMu.Unlock()
	if !exists(a.dnsBackup()) {
		return
	}
	if _, err := a.restoreDNS(ctx); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Println("system dns:", err)
	}
}

// localDNS are the network's own DNS servers, for the names that are the
// system's to resolve (devnet's SystemDNS): with the system's servers set
// to the placeholder, the core's "system" would ask the placeholder. They
// are what the system asks before the placeholder is set, and after that
// what the copy on disk or the network's lease says. fresh reads the lease
// again.
func (a *App) localDNS(s settings.Settings, env devnet.Env, fresh bool) []string {
	if !a.dnsWanted(s) {
		return nil
	}
	if !slices.Equal(env.DefaultDNS, sysdns.Placeholder) {
		return sysdns.Usable(env.DefaultDNS)
	}
	a.dev.mu.Lock()
	lease := a.dev.lease
	a.dev.mu.Unlock()
	if fresh || lease.iface != env.PrimaryIface {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		lease = dnsLease{env.PrimaryIface, osDNS.dhcp(ctx, env.PrimaryIface)}
		cancel()
		a.dev.mu.Lock()
		a.dev.lease = lease
		a.dev.mu.Unlock()
	}
	saved, _ := a.readDNSBackup()
	return sysdns.Local(saved, env.PrimaryIface, lease.servers)
}

// dnsLease is what an interface's network hands out as DNS servers.
type dnsLease struct {
	iface   string
	servers []string
}

// dnsOutside says whether the system's first DNS server is one TUN leaves
// out: a query to it goes by the physical interface, past dns-hijack.
func dnsOutside(servers, exclude []string) bool {
	if len(servers) == 0 {
		return false
	}
	ip, err := netip.ParseAddr(servers[0])
	if err != nil {
		return false
	}
	ip = ip.WithZone("")
	for _, c := range exclude {
		if p, err := netip.ParsePrefix(c); err == nil && p.Contains(ip) {
			return true
		}
	}
	return false
}

// dnsIssue is the news when names are resolved past TUN: every rule that
// names a domain then sees a bare address and does not match (#117).
func (a *App) dnsIssue(s settings.Settings, cs core.Status) *Issue {
	if cs.State != core.Running || len(s.DNSHijack) == 0 || !a.tunOn(s) {
		return nil
	}
	text := i18n.T("系统 DNS 没有经过 TUN，按域名的规则不会生效", "System DNS goes around TUN, so rules by domain don't match")
	if why := a.dnsErr.Load(); why != nil && a.dnsWanted(s) {
		return &Issue{Level: "warn", Text: text, Detail: fmt.Sprintf(i18n.T("没能把系统的 DNS 服务器指向 TUN：%s", "Couldn't point the system's DNS servers into TUN: %s"), *why), Action: "take-dns", Label: i18n.T("重试", "Retry")}
	}
	a.dev.mu.Lock()
	env, at, outside := a.dev.env, a.dev.at, a.dev.dnsOutside
	a.dev.mu.Unlock()
	if !outside || len(env.DefaultDNS) == 0 {
		return nil
	}
	server := env.DefaultDNS[0]
	switch {
	case !osDNS.supported:
		return &Issue{Level: "warn", Text: text, Detail: fmt.Sprintf(i18n.T("系统的 DNS 服务器 %s 是局域网地址，查询不进 TUN。把它改成公网地址（如 223.5.5.5），或启用 systemd-resolved。", "The system's DNS server %s is on the LAN, so queries don't enter TUN. Set it to a public address (as 223.5.5.5), or turn on systemd-resolved."), server)}
	case s.TunKeepDNS:
		return &Issue{Level: "warn", Text: text, Detail: fmt.Sprintf(i18n.T("系统的 DNS 服务器 %s 是局域网地址，查询不进 TUN，而「接管系统 DNS」关着。", "The system's DNS server %s is on the LAN, so queries don't enter TUN, and “Take over system DNS” is off."), server), Action: "take-dns", Label: i18n.T("接管", "Take over")}
	}
	// Taken over, and read again since: something other than the network
	// services decides what the system asks, as a VPN or a profile does.
	if held := a.dnsHeld.Load(); held != nil && at.After(*held) {
		return &Issue{Level: "warn", Text: text, Detail: fmt.Sprintf(i18n.T("bowel 已经设置了各网络服务的 DNS，但系统仍在用 %s（可能来自 VPN 或描述文件）。", "bowel has set every network service's DNS, yet the system still asks %s (a VPN or a profile may set it)."), server)}
	}
	return nil
}

// takeDNSNow is the issue's button: it turns the takeover of the system's
// DNS on if it was off, and tries again.
func (a *App) takeDNSNow() error {
	if a.settings.Get().TunKeepDNS {
		return a.updateSettings(func(s *settings.Settings) { s.TunKeepDNS = false })
	}
	a.applySystemDNS()
	a.refresh()
	return nil
}
