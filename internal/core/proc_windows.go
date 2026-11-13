//go:build windows

package core

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const createNoWindow = 0x08000000

func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

func interrupt(cmd *exec.Cmd) { _ = cmd.Process.Kill() }

// liveCore is the pid the pid file names while that is a running mihomo,
// else 0.
func liveCore(pidFile string) int {
	b, err := os.ReadFile(pidFile)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 4 || !strings.Contains(strings.ToLower(exeOf(pid)), "mihomo") {
		return 0
	}
	return pid
}

// exeOf is the file name of the process's executable, "" when there is no
// such process. From the process list, which also has the processes whose
// handle the app is refused: the core that TUN's task runs elevated.
func exeOf(pid int) string {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if int(e.ProcessID) == pid {
			return windows.UTF16ToString(e.ExeFile[:])
		}
	}
	return ""
}

// adopted is a core another run of the app started: not a child, so it is
// known by its pid alone.
type adopted struct{ pid int }

func (a adopted) Pid() int { return a.pid }

// Wait returns once the process is gone: by its handle, and by asking the
// process list where the handle is refused.
func (a adopted) Wait() error {
	if h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(a.pid)); err == nil {
		defer windows.CloseHandle(h)
		if _, err := windows.WaitForSingleObject(h, windows.INFINITE); err == nil {
			return nil
		}
	}
	for alive(a.pid) {
		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

// Stop ends the process: Windows has no asking first, as for a child.
func (a adopted) Stop() {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(a.pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	_ = windows.TerminateProcess(h, 1)
}

func (a adopted) kill() { a.Stop() }

// alive: the process exists. A process that has ended stays in the list
// while a handle to it is open, so the handle is asked where there is one.
func alive(pid int) bool {
	if h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid)); err == nil {
		defer windows.CloseHandle(h)
		ev, err := windows.WaitForSingleObject(h, 0)
		return err == nil && ev == uint32(windows.WAIT_TIMEOUT)
	}
	return exeOf(pid) != ""
}
