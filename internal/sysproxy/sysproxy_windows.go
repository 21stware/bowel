// Package sysproxy points the system's HTTP, HTTPS and SOCKS proxies at the
// core.
package sysproxy

// On Windows the system proxy is WinINet's, which most programs follow
// (browsers, the Store, WinHTTP clients that ask for the user's settings).
// It is read and written through InternetQueryOption and InternetSetOption
// with per-connection options, which keep the registry and Windows'
// Settings page in step, and then announced so that running programs pick
// it up. Only the LAN settings are changed: they serve every connection
// but dial-up and VPN entries that have their own.

import (
	"context"
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DefaultBypass is what a service gets back when Forget clears one that
// was all Moho's: Windows starts with none.
var DefaultBypass []string

var (
	wininet         = windows.NewLazySystemDLL("wininet.dll")
	procSetOption   = wininet.NewProc("InternetSetOptionW")
	procQueryOption = wininet.NewProc("InternetQueryOptionW")
	procOpen        = wininet.NewProc("InternetOpenW")
	procCloseHandle = wininet.NewProc("InternetCloseHandle")
	procGlobalFree  = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalFree")
)

const (
	optSettingsChanged = 39 // INTERNET_OPTION_SETTINGS_CHANGED
	optRefresh         = 37 // INTERNET_OPTION_REFRESH
	optPerConnection   = 75 // INTERNET_OPTION_PER_CONNECTION_OPTION

	connFlags   = 1  // INTERNET_PER_CONN_FLAGS
	connServer  = 2  // INTERNET_PER_CONN_PROXY_SERVER
	connBypass  = 3  // INTERNET_PER_CONN_PROXY_BYPASS
	connPAC     = 4  // INTERNET_PER_CONN_AUTOCONFIG_URL
	connFlagsUI = 10 // INTERNET_PER_CONN_FLAGS_UI: the flags as the user set them
)

const ptrSize = unsafe.Sizeof(uintptr(0))

// perConnOption is INTERNET_PER_CONN_OPTIONW. Its value is a union of a
// DWORD, a string and a FILETIME, eight bytes on every platform.
type perConnOption struct {
	option uint32
	value  [8 / ptrSize]uintptr // value[0] holds the DWORD or the string
}

// perConnOptionList is INTERNET_PER_CONN_OPTION_LISTW.
type perConnOptionList struct {
	size       uint32
	connection *uint16 // nil: the LAN settings
	count      uint32
	failed     uint32
	options    *perConnOption
}

// The layouts of the C structs: 16 and 32 bytes on 64-bit Windows, 12 and
// 20 on 32-bit. A mismatch fails the build.
var (
	_ [0]struct{} = [unsafe.Sizeof(perConnOption{}) - (8 + ptrSize)]struct{}{}
	_ [0]struct{} = [unsafe.Sizeof(perConnOptionList{}) - (8 + 3*ptrSize)]struct{}{}
)

// str reads a string option's value that WinINet filled in. Writes go
// through strOption.
func (o *perConnOption) str() **uint16 { return (**uint16)(unsafe.Pointer(&o.value[0])) }

func setOption(option uintptr, buf unsafe.Pointer, size uintptr) error {
	if r, _, err := procSetOption.Call(0, option, uintptr(buf), size); r == 0 {
		return fmt.Errorf("InternetSetOption %d: %w", option, err)
	}
	return nil
}

// write sets the LAN settings' options and tells running programs.
func write(opts []perConnOption, keep ...[]uint16) error {
	list := perConnOptionList{count: uint32(len(opts)), options: &opts[0]}
	list.size = uint32(unsafe.Sizeof(list))
	err := setOption(optPerConnection, unsafe.Pointer(&list), unsafe.Sizeof(list))
	runtime.KeepAlive(opts)
	runtime.KeepAlive(keep)
	if err != nil {
		return err
	}
	_ = setOption(optSettingsChanged, nil, 0)
	_ = setOption(optRefresh, nil, 0)
	return nil
}

func strOption(option uint32, s string) (perConnOption, []uint16) {
	u, err := windows.UTF16FromString(s)
	if err != nil { // a NUL in s; none of Moho's strings has one
		u = []uint16{0}
	}
	// The address goes in as the integer the field is: a store through
	// str()'s **uint16 view of a local is dropped by the compiler, and
	// WinINet got a NULL string (the proxy on, with no server). The
	// caller keeps u alive, and heap memory doesn't move.
	o := perConnOption{option: option}
	o.value[0] = uintptr(unsafe.Pointer(&u[0]))
	return o, u
}

// apply writes the options of ws that set names, and leaves the others as
// Windows has them. Windows refuses a write whole: winRefusal names what
// one held.
func apply(ws winSettings, set winSet) error {
	var opts []perConnOption
	var keep [][]uint16
	if set&setFlags != 0 {
		opts = append(opts, perConnOption{option: connFlags, value: [8 / ptrSize]uintptr{uintptr(ws.flags)}})
	}
	for _, o := range []struct {
		in     winSet
		option uint32
		s      string
	}{{setServer, connServer, ws.server}, {setBypass, connBypass, ws.override}, {setPAC, connPAC, ws.pac}} {
		if set&o.in != 0 {
			opt, u := strOption(o.option, o.s)
			opts, keep = append(opts, opt), append(keep, u)
		}
	}
	if len(opts) == 0 {
		return nil
	}
	return write(opts, keep...)
}

// openSession opens a WinINet session that takes the system's settings,
// for read to ask on when Windows refuses to answer without one.
func openSession() (uintptr, func(), error) {
	agent, _ := windows.UTF16PtrFromString("bowel")
	const preconfig = 0 // INTERNET_OPEN_TYPE_PRECONFIG
	h, _, err := procOpen.Call(uintptr(unsafe.Pointer(agent)), preconfig, 0, 0, 0)
	runtime.KeepAlive(agent)
	if h == 0 {
		return 0, nil, fmt.Errorf("InternetOpen: %w", err)
	}
	return h, func() { procCloseHandle.Call(h) }, nil
}

// read reads the LAN settings.
func read() (winSettings, error) {
	return winQuery(readOn, openSession)
}

// readOn reads the LAN settings on WinINet handle h, 0 for none.
func readOn(h uintptr) (winSettings, error) {
	query := func(flagsOption uint32) ([]perConnOption, error) {
		opts := []perConnOption{{option: flagsOption}, {option: connServer}, {option: connBypass}, {option: connPAC}}
		list := perConnOptionList{count: uint32(len(opts)), options: &opts[0]}
		list.size = uint32(unsafe.Sizeof(list))
		n := list.size
		r, _, err := procQueryOption.Call(h, optPerConnection, uintptr(unsafe.Pointer(&list)), uintptr(unsafe.Pointer(&n)))
		runtime.KeepAlive(opts)
		if r == 0 {
			return nil, fmt.Errorf("InternetQueryOption: %w", err)
		}
		return opts, nil
	}
	// FLAGS_UI is the real switch since Windows 7; older systems refuse it.
	opts, err := query(connFlagsUI)
	if err != nil {
		if opts, err = query(connFlags); err != nil {
			return winSettings{}, err
		}
	}
	take := func(o *perConnOption) string {
		p := *o.str()
		if p == nil {
			return ""
		}
		s := windows.UTF16PtrToString(p)
		procGlobalFree.Call(o.value[0])
		return s
	}
	return winSettings{
		flags:    uint32(opts[0].value[0]),
		server:   take(&opts[1]),
		override: take(&opts[2]),
		pac:      take(&opts[3]),
	}, nil
}

// Enable points the LAN settings' proxies at host:port, with the hosts and
// networks in bypass left out of them. It turns automatic configuration
// off, which would come before them, and leaves the script's address for
// Windows' settings page; Restore turns it back on. If Windows refuses the
// bypass list the takeover is made with a shorter one or none: LastWrite
// tells which, and what was refused.
func Enable(host string, port int, bypass []string) error {
	e := Endpoint{On: true, Host: host, Port: port}
	w, err := winTakeover(apply, winServer(e, e, Endpoint{}), bypass)
	setLastWrite(w)
	return err
}

// Disable turns the proxies off and leaves the rest as it is.
func Disable() error {
	cur, err := read()
	if err != nil {
		return err
	}
	ws := winSettings{flags: cur.flags&^winProxy | winDirect}
	if err := apply(ws, setFlags); err != nil {
		return winRefusal(ws, setFlags, err)
	}
	return nil
}

// Snapshot reads the LAN settings. It changes nothing.
func Snapshot() (Saved, error) {
	ws, err := read()
	if err != nil {
		return Saved{}, err
	}
	return Saved{Services: []Service{winService(ws)}}, nil
}

// Restore puts the LAN settings back as saved has them; with none saved
// it turns the proxies off.
func Restore(_ context.Context, saved Saved) error {
	if len(saved.Services) == 0 {
		return Disable()
	}
	ws := saved.Services[0].winSettings()
	set := setFlags | setServer | setBypass | setPAC
	if cur, err := read(); err == nil {
		set = winRestoreSet(ws, cur)
	}
	if err := apply(ws, set); err != nil {
		return winRefusal(ws, set, err)
	}
	return nil
}

// Check reads the LAN settings and tells whether they point at the core's
// mixed port. It changes nothing.
func Check(port int) (Status, error) {
	ws, err := read()
	if err != nil {
		return Status{}, err
	}
	return winStatus(ws, port), nil
}
