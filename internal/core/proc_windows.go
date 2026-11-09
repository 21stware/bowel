//go:build windows

package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const createNoWindow = 0x08000000

func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

func interrupt(cmd *exec.Cmd) { _ = cmd.Process.Kill() }

// killStale ends a core a previous run of the app left behind (after a
// crash), when the pid file still names a mihomo process. The pid may
// have been reused since, so the image name decides.
func killStale(pidFile string) {
	b, err := os.ReadFile(pidFile)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 1 {
		return
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	var name [windows.MAX_PATH]uint16
	n := uint32(len(name))
	if err := windows.QueryFullProcessImageName(h, 0, &name[0], &n); err != nil {
		return
	}
	if !strings.EqualFold(filepath.Base(windows.UTF16ToString(name[:n])), "mihomo.exe") {
		return
	}
	_ = windows.TerminateProcess(h, 1)
}

// The core runs elevated (TUN) through ShellExecuteEx's runas, which asks
// the administrator once, and is kept by its process handle: it is no
// child of ours, so it has no pipes (the controller serves the logs) and
// its exit is watched on the handle rather than waited.
type elevatedProc struct {
	handle windows.Handle
	pid    uint32
}

var (
	shell32 = windows.NewLazySystemDLL("shell32.dll")
	// procShellExecEx is ShellExecuteExW, which returns the process
	// handle when asked to with seeMaskNoCloseProcess.
	procShellExecEx = shell32.NewProc("ShellExecuteExW")
)

// seeMaskNoCloseProcess is SEE_MASK_NOCLOSEPROCESS: ShellExecuteExW
// closes the process handle on return unless this bit is set.
const seeMaskNoCloseProcess = 0x40

// shellExecuteInfo is SHELLEXECUTEINFOW, laid out as the 64-bit ABI has
// it: the union after dwHotKey is a pointer-sized field.
type shellExecuteInfo struct {
	cbSize   uint32
	fMask    uint32
	hwnd     windows.HWND
	verb     *uint16
	file     *uint16
	params   *uint16
	dir      *uint16
	show     int32
	instApp  windows.Handle
	idList   *uint16
	class    *uint16
	keyClass windows.Handle
	hotKey   uint32
	icon     windows.Handle // the union's hIcon
	process  windows.Handle
}

// startElevatedProc runs `bin -d dir -f config` as the administrator,
// waiting for the prompt to be answered, and reports the core by the
// coreProc interface with its pid. The elevated core's environment comes
// from the shell this opens, not from us, so safePaths (the SAFE_PATHS
// value, "" for none) goes on the command line and cmd sets it before
// the core runs. show is SW_HIDE, so the console window stays off.
func startElevatedProc(bin, dir, config, safePaths string) (coreProc, int, error) {
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return nil, 0, err
	}
	comspec := os.Getenv("ComSpec")
	if comspec == "" {
		comspec = "cmd.exe"
	}
	file, err := windows.UTF16PtrFromString(comspec)
	if err != nil {
		return nil, 0, err
	}
	set := ""
	if safePaths != "" {
		set = "set \"SAFE_PATHS=" + safePaths + "\" && "
	}
	params, err := windows.UTF16PtrFromString(`/c ` + set + `"` + bin + `" -d "` + dir + `" -f "` + config + `"`)
	if err != nil {
		return nil, 0, err
	}
	dirP, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return nil, 0, err
	}
	var si shellExecuteInfo
	si.cbSize = uint32(unsafe.Sizeof(si))
	si.fMask = seeMaskNoCloseProcess
	si.verb, si.file, si.params, si.dir = verb, file, params, dirP
	r, _, callErr := procShellExecEx.Call(uintptr(unsafe.Pointer(&si)))
	if r == 0 {
		if callErr.(syscall.Errno) == windows.ERROR_CANCELLED {
			return nil, 0, errCanceled
		}
		return nil, 0, fmt.Errorf("runas: %v", callErr)
	}
	pid, err := windows.GetProcessId(si.process)
	if err != nil || pid == 0 {
		return nil, 0, fmt.Errorf("runas: no process id: %v", err)
	}
	return &elevatedProc{handle: si.process, pid: pid}, int(pid), nil
}

// kill ends the elevated core the way Process.Kill would. The handle is
// the cmd that set the environment and ran the core; its child dies
// with it only when cmd sees the kill, which TerminateProcess denies it.
// The core is killed by its image name among cmd's children instead.
func (p *elevatedProc) kill() error {
	core := p.corePID()
	if core != 0 {
		if h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, core); err == nil {
			_ = windows.TerminateProcess(h, 1)
			_ = windows.CloseHandle(h)
		}
	}
	return windows.TerminateProcess(p.handle, 1)
}

// corePID finds the mihomo among the cmd's children, by image name.
func (p *elevatedProc) corePID() uint32 {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	parent := uint32(p.pid)
	for err := windows.Process32Next(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if pe.ParentProcessID == parent && strings.EqualFold(
			windows.UTF16ToString(pe.ExeFile[:]), "mihomo.exe") {
			return pe.ProcessID
		}
	}
	return 0
}

// wait blocks until the elevated core exits, and reports a nonzero code
// as an error, the way cmd.Wait would: cmd returns the core's code.
func (p *elevatedProc) wait() error {
	ev, err := windows.WaitForSingleObject(p.handle, windows.INFINITE)
	if err != nil {
		return err
	}
	if ev != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("wait: %d", ev)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(p.handle, &code); err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("exit status %d", code)
	}
	return nil
}
