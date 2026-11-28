package uwploop

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/nowindow"
)

// Supported is whether there are packaged apps kept from the loopback:
// Windows alone has them.
const Supported = true

var (
	firewallAPI     = windows.NewLazySystemDLL("FirewallAPI.dll")
	procEnum        = firewallAPI.NewProc("NetworkIsolationEnumAppContainers")
	procFree        = firewallAPI.NewProc("NetworkIsolationFreeAppContainers")
	procGetConfig   = firewallAPI.NewProc("NetworkIsolationGetAppContainerConfig")
	procShellExecEx = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")
)

func init() {
	containers = enumContainers
	exempted = readConfig
	direct = runDirect
	elevate = runas
	tool = func() string {
		sys, err := windows.GetSystemDirectory()
		if err != nil {
			sys = `C:\Windows\System32`
		}
		return filepath.Join(sys, "CheckNetIsolation.exe")
	}
}

// fwAppContainer is INET_FIREWALL_APP_CONTAINER.
type fwAppContainer struct {
	appContainerSid  *windows.SID
	userSid          *windows.SID
	appContainerName *uint16
	displayName      *uint16
	description      *uint16
	capCount         uint32
	caps             uintptr
	binCount         uint32
	bins             uintptr
	workingDirectory *uint16
	packageFullName  *uint16
}

// enumContainers is the SIDs of the app containers of the computer's
// packaged apps, each once.
func enumContainers() ([]string, error) {
	var n uint32
	var arr *fwAppContainer
	if r, _, _ := procEnum.Call(0, uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&arr))); r != 0 {
		return nil, fmt.Errorf(i18n.T("读不到应用容器的列表：%s", "Couldn't list the app containers: %s"), windows.Errno(r))
	}
	if arr == nil {
		return nil, nil
	}
	defer procFree.Call(uintptr(unsafe.Pointer(arr)))
	items := unsafe.Slice(arr, n)
	var out []string
	for _, it := range items {
		if it.appContainerSid == nil {
			continue
		}
		s := it.appContainerSid.String()
		if validSID(s) && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out, nil
}

// readConfig is the SIDs on the loopback exemption list, which reading
// asks no administrator for.
func readConfig() ([]string, error) {
	var n uint32
	var arr *windows.SIDAndAttributes
	if r, _, _ := procGetConfig.Call(uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&arr))); r != 0 {
		return nil, fmt.Errorf(i18n.T("读不到本机回环的例外列表：%s", "Couldn't read the loopback exemptions: %s"), windows.Errno(r))
	}
	if arr == nil {
		return nil, nil
	}
	// The function's documentation says nothing of how its memory is
	// given back, and a wrong free would bring the app down: the few
	// hundred bytes a read takes, at the user's look at the setting, are
	// left.
	items := unsafe.Slice(arr, n)
	out := make([]string, 0, n)
	for _, it := range items {
		if it.Sid != nil {
			out = append(out, it.Sid.String())
		}
	}
	return out, nil
}

// shellExecuteInfo is SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	size       uint32
	mask       uint32
	hwnd       windows.Handle
	verb       *uint16
	file       *uint16
	parameters *uint16
	directory  *uint16
	show       int32
	instApp    windows.Handle
	idList     uintptr
	class      *uint16
	keyClass   windows.Handle
	hotKey     uint32
	icon       windows.Handle
	process    windows.Handle
}

const (
	seeNoCloseProcess = 0x00000040 // SEE_MASK_NOCLOSEPROCESS
	seeNoUI           = 0x00000400 // SEE_MASK_FLAG_NO_UI
)

// runas runs cmd in an elevated cmd.exe, hidden, after Windows' prompt,
// and waits for it. Whether it did what it was for is read from the
// exemption list after, not from its exit code: CheckNetIsolation's says
// little, and the loop's only the last one's.
func runas(cmd string) error {
	sys, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, err := windows.UTF16PtrFromString(filepath.Join(sys, "cmd.exe"))
	if err != nil {
		return err
	}
	// /s: what is between the outer quotes is the command, as written.
	params, err := windows.UTF16PtrFromString(`/d /s /c "` + cmd + `"`)
	if err != nil {
		return err
	}
	info := shellExecuteInfo{mask: seeNoCloseProcess | seeNoUI, verb: verb, file: file, parameters: params, show: windows.SW_HIDE}
	info.size = uint32(unsafe.Sizeof(info))
	if r, _, err := procShellExecEx.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return ErrCanceled
		}
		return fmt.Errorf(i18n.T("授权失败：%s", "Authorization failed: %s"), err)
	}
	if info.process == 0 {
		return errors.New(i18n.T("授权失败：没有启动提权的进程", "Authorization failed: no elevated process was started"))
	}
	defer windows.CloseHandle(info.process)
	_, err = windows.WaitForSingleObject(info.process, windows.INFINITE)
	return err
}

// runDirect runs CheckNetIsolation on sid as the user is, hidden.
func runDirect(sid string, add bool) error {
	op := "-a"
	if !add {
		op = "-d"
	}
	return nowindow.Command(tool(), "LoopbackExempt", op, "-p="+sid).Run()
}
