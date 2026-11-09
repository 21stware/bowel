//go:build windows

package gateway

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/tun"
)

// OS is Windows.
//
// LAN devices point their gateway and DNS at this computer; the packets
// arrive on the LAN's interface and leave through TUN's. Two things make
// that work: the LAN interface must forward (a per-interface switch), and
// the packets leaving through TUN must be NATed to TUN's address, or the
// answers have no way back. The NAT is a New-NetNat rule, which lasts
// until it is removed, so the administrator is asked once, to create it
// and to turn forwarding on; Ready says whether it is there.
func OS() System { return &windows_{} }

type windows_ struct {
	mu       sync.Mutex
	slowAt   time.Time // when wifi and firewall were read
	wifi     map[string]bool
	firewall bool
	ready    bool      // the NetNat rule was there, as of readyAt
	readyAt  time.Time // when ready was last read
	natOK    bool      // this Windows knows NetNat, as of natAt
	natAt    time.Time // when natOK was last read
}

// natName is the NetNat rule this package creates.
const natName = "bowel-gw"

// readyTTL is how long Ready trusts its last look. The snapshot asks
// once a second, and each look is a PowerShell process, so the answer
// is kept briefly; the state changes only under Prepare and
// SetForwarding, which drop the cache.
const readyTTL = 5 * time.Second

// runPowerShell runs a PowerShell one-liner and returns its output.
func runPowerShell(args ...string) (string, error) {
	argv := append([]string{"-NoProfile", "-NonInteractive", "-Command"}, args...)
	out, err := exec.Command("powershell", argv...).CombinedOutput()
	return string(out), err
}

// psTrue says whether the command's output says so.
func psTrue(out string) bool {
	return strings.Contains(strings.ToLower(out), "true")
}

func (w *windows_) Forwarding() (bool, error) {
	iface := lanInterface()
	if iface == "" {
		return false, errors.New(i18n.T("没有局域网接口", "no LAN interface"))
	}
	out, err := runPowerShell(
		`(Get-NetIPInterface -InterfaceAlias '` + psQuote(iface) + `' -AddressFamily IPv4).Forwarding`)
	if err != nil {
		return false, fmt.Errorf("%s: %s", err, strings.TrimSpace(out))
	}
	return psTrue(out), nil
}

// SetForwarding turns the LAN interface's forwarding on or off, which the
// administrator's consent makes possible (Prepare). The core, which runs
// elevated for TUN, does it instead when it can: this falls back to it.
func (w *windows_) SetForwarding(on bool) error {
	iface := lanInterface()
	if iface == "" {
		return errors.New(i18n.T("没有局域网接口", "no LAN interface"))
	}
	v := "Disabled"
	if on {
		v = "Enabled"
	}
	out, err := runPowerShell(
		`Set-NetIPInterface -InterfaceAlias '` + psQuote(iface) + `' -AddressFamily IPv4 -Forwarding ` + v)
	if err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(out))
	}
	if cur, err := w.Forwarding(); err == nil && cur != on {
		return errors.New(i18n.T("IP 转发没有改过来", "IP forwarding didn't change"))
	}
	w.forgetReady()
	return nil
}

// readyOut says whether the NAT rule is there, as PowerShell answers.
func (w *windows_) readyOut() string {
	out, _ := runPowerShell(`(Get-NetNat -Name '` + natName + `' -ErrorAction SilentlyContinue) -ne $null`)
	return out
}

// natSupported says whether this Windows knows NetNat at all: some
// builds answer Get-NetNat's queries with an invalid-class error, the
// WMI class being absent, and no amount of consenting changes that. The
// answer is kept for a minute.
func (w *windows_) natSupported() bool {
	w.mu.Lock()
	if time.Since(w.natAt) < time.Minute {
		v := w.natOK
		w.mu.Unlock()
		return v
	}
	w.mu.Unlock()
	out, err := runPowerShell(`(Get-CimClass -ClassName MSFT_NetNat -Namespace root/StandardCimv2) -ne $null`)
	v := err == nil && psTrue(out)
	w.mu.Lock()
	w.natOK, w.natAt = v, time.Now()
	w.mu.Unlock()
	return v
}

// Ready says whether the NAT rule is there, trusting readyTTL's last
// look. askReady reads it afresh.
func (w *windows_) Ready() bool {
	w.mu.Lock()
	if time.Since(w.readyAt) < readyTTL {
		v := w.ready
		w.mu.Unlock()
		return v
	}
	w.mu.Unlock()
	return w.askReady()
}

// askReady reads whether the NAT rule is there, keeping the answer.
func (w *windows_) askReady() bool {
	v := psTrue(w.readyOut())
	w.mu.Lock()
	w.ready, w.readyAt = v, time.Now()
	w.mu.Unlock()
	return v
}

// forgetReady drops the cached answer, for whoever has just changed it.
func (w *windows_) forgetReady() {
	w.mu.Lock()
	w.readyAt = time.Time{}
	w.mu.Unlock()
}

func (w *windows_) Prepare(prompt string, before ...tun.Step) error {
	iface := lanInterface()
	if iface == "" {
		return errors.New(i18n.T("这台电脑没有连上局域网", "This computer isn't on a LAN"))
	}
	if !w.natSupported() {
		return errors.New(i18n.T("这台 Windows 没有 NetNat（网络地址转换），网关模式需要它。请在「启用或关闭 Windows 功能」里补齐后重试",
			"This Windows has no NetNat (network address translation), which gateway mode needs. Fill it in under Turn Windows features on or off, and try again"))
	}
	// One script does all of it: the NAT rule, which lasts, and the LAN
	// interface's forwarding, which SetForwarding could also do but only
	// with the same consent. A failure inside the elevated cmd would
	// otherwise reach the app as a bare exit status, so the script's
	// error is written to a file this side reads back.
	errPath := filepath.Join(os.TempDir(), "bowel-gateway-error.txt")
	_ = os.Remove(errPath)
	script := `$ErrorActionPreference = 'Stop'
try {
	if ((Get-NetNat -Name '` + natName + `' -ErrorAction SilentlyContinue) -eq $null) {
		New-NetNat -Name '` + natName + `' -InternalIPInterfaceAddressPrefix 198.18.0.0/15 | Out-Null
	}
	Set-NetIPInterface -InterfaceAlias '` + psQuote(iface) + `' -AddressFamily IPv4 -Forwarding Enabled
} catch {
	$_ | Out-File -FilePath '` + errPath + `' -Encoding utf8
	exit 1
}`
	f, err := os.CreateTemp("", "bowel-gateway-*.ps1")
	if err != nil {
		return err
	}
	path := f.Name()
	defer os.Remove(path)
	if _, err := f.WriteString(script + "\r\n"); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	steps := append(append([]tun.Step{}, before...),
		tun.Step{"powershell", " -NoProfile -ExecutionPolicy Bypass -File ", path})
	if err := tun.Admin(prompt, steps...); err != nil {
		if b, rerr := os.ReadFile(errPath); rerr == nil && len(bytes.TrimSpace(b)) > 0 {
			return fmt.Errorf("%s: %s", i18n.T("启用网关的命令失败了", "the gateway's commands failed"), strings.TrimSpace(string(b)))
		}
		return err
	}
	_ = os.Remove(errPath)
	w.forgetReady()
	if !w.askReady() {
		return errors.New(i18n.T("授权后仍然无法启用网关", "the gateway still can't be turned on after the authorization"))
	}
	return nil
}

// KeepAwake keeps the system awake until stop: one thread keeps saying
// that it is in use, as long as this process lives. The call needs no
// consent and nothing to clean up if the process goes away.
func (w *windows_) KeepAwake() (func(), error) {
	const (
		esSystemRequired = 0x1
		esContinuous     = 0x80000000
	)
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	proc := kernel32.NewProc("SetThreadExecutionState")
	done := make(chan struct{})
	stop := make(chan struct{})
	go func() {
		for {
			r, _, _ := proc.Call(uintptr(esSystemRequired | esContinuous))
			if r == 0 { // the call failed: no point repeating it
				close(done)
				return
			}
			select {
			case <-stop:
				close(done)
				return
			case <-time.After(30 * time.Second):
			}
		}
	}()
	return func() {
		close(stop)
		<-done
		proc.Call(0) // esContinuous, cleared
	}, nil
}

func (w *windows_) AnswersDNS(addr string) bool { return answersDNS(addr, time.Second) }

// Network is what this computer's LAN looks like: the interface that has
// the default route and is not TUN's, its address, the router, whether
// it is Wi-Fi, whether the address was leased, and the firewall.
func (w *windows_) Network() Network {
	n := Network{}
	// The system's primary interface, unless it is a tunnel: TUN (this
	// app's own, or another VPN's) is often the default route's, and not
	// on the LAN.
	if iface, router := defaultRouteInterface(); iface != "" && physical(iface) {
		if ifc, err := net.InterfaceByName(iface); err == nil {
			n.IP, n.Iface, n.Router = ifaceIPv4(ifc), iface, router
		}
	}
	if n.IP == "" {
		n = scanLAN()
	}
	if n.Iface == "" {
		return n
	}
	if n.Router == "" {
		n.Router = gatewayOf(n.Iface)
	}
	w.mu.Lock()
	if time.Since(w.slowAt) > 30*time.Second {
		w.wifi, w.firewall, w.slowAt = wifiAdapters(), firewallOn(), time.Now()
	}
	n.WiFi, n.Firewall = w.wifi[friendlyName(n.Iface)], w.firewall
	w.mu.Unlock()
	n.DHCP = leased(n.Iface)
	return n
}

// lanInterface is the LAN's interface as PowerShell names it, which is
// the connection's name, not the adapter's.
func lanInterface() string {
	if iface, _ := defaultRouteInterface(); iface != "" && physical(iface) {
		return iface
	}
	if n := scanLAN(); n.Iface != "" {
		return friendlyName(n.Iface)
	}
	return ""
}

// defaultRouteInterface is the interface and the next hop of the default
// route that is not a tunnel's.
func defaultRouteInterface() (iface, router string) {
	ifs, err := net.Interfaces()
	if err != nil {
		return "", ""
	}
	var best *net.Interface
	var bestRouter string
	for i := range ifs {
		ifc := &ifs[i]
		if ifc.Flags&net.FlagUp == 0 || !physical(ifc.Name) {
			continue
		}
		if r, ok := routeOf(uint32(ifc.Index)); ok {
			best, bestRouter = ifc, r
			break // the first physical interface with a route will do
		}
	}
	if best == nil {
		return "", ""
	}
	return best.Name, bestRouter
}

// routeOf is iface's default route's next hop, from the system's route
// table, which GetIpForwardTable2 keeps.
func routeOf(iface uint32) (string, bool) {
	// Read the route table through PowerShell: Get-NetRoute has what the
	// API returns, and this runs at most a few times a minute.
	out, err := runPowerShell(
		`(Get-NetRoute -DestinationPrefix '0.0.0.0/0' -InterfaceIndex ` + fmt.Sprint(iface) +
			` -ErrorAction SilentlyContinue | Select-Object -First 1 -ExpandProperty NextHop)`)
	if err != nil {
		return "", false
	}
	r := strings.TrimSpace(out)
	if r == "" || !net.ParseIP(r).IsGlobalUnicast() {
		return "", false
	}
	return r, true
}

// gatewayOf is the interface's router, as the ARP table has it or as the
// route table says.
func gatewayOf(iface string) string {
	out, err := runPowerShell(
		`(Get-NetRoute -DestinationPrefix '0.0.0.0/0' -InterfaceAlias '` + psQuote(iface) +
			`' -ErrorAction SilentlyContinue | Select-Object -First 1 -ExpandProperty NextHop)`)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// friendlyName is the connection name PowerShell knows an adapter by,
// from the interface's name Go shows, which is the adapter's.
func friendlyName(name string) string {
	out, err := runPowerShell(
		`(Get-NetAdapter -InterfaceDescription '` + psQuote(name) + `' -ErrorAction SilentlyContinue | Select-Object -First 1 -ExpandProperty Name)`)
	if err != nil || strings.TrimSpace(out) == "" {
		return name
	}
	return strings.TrimSpace(out)
}

// wifiAdapters are the names of the Wi-Fi adapters, from the adapter
// list, where a Wi-Fi one says what it is.
func wifiAdapters() map[string]bool {
	out, _ := runPowerShell(
		`Get-CimInstance Win32_NetworkAdapter -Filter 'PhysicalAdapter=True' | Where-Object {$_.Name -match 'Wi-?Fi|Wireless|802\.11|WLAN'} | ForEach-Object {$_.NetConnectionID}`)
	m := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		if v := strings.TrimSpace(line); v != "" {
			m[v] = true
		}
	}
	return m
}

// leased says whether the interface's address was leased, so it may change.
func leased(iface string) bool {
	out, _ := runPowerShell(
		`(Get-NetIPAddress -InterfaceAlias '` + psQuote(iface) + `' -AddressFamily IPv4 -ErrorAction SilentlyContinue | Where-Object {$_.IPAddress -notlike '169.254.*'} | Select-Object -First 1 -ExpandProperty PrefixOrigin)`)
	return strings.TrimSpace(out) == "Dhcp"
}

// firewallOn says whether any firewall profile is on, which may block
// port 53 from the LAN.
func firewallOn() bool {
	out, _ := runPowerShell(`(Get-NetFirewallProfile | Where-Object {$_.Enabled}).Count -gt 0`)
	return psTrue(out)
}

// psQuote is s in a single-quoted PowerShell string, ' doubled.
func psQuote(s string) string { return strings.ReplaceAll(s, "'", "''") }

// PrimaryInterface is the interface this computer reaches the network
// through, "" if it has none.
func PrimaryInterface() string {
	if iface, _ := defaultRouteInterface(); iface != "" {
		return iface
	}
	if n := scanLAN(); n.Iface != "" {
		return n.Iface
	}
	return ""
}

// keep unused imports honest: syscall and unsafe serve a later use.
var _ = syscall.Stdout
var _ = unsafe.Sizeof(0)
