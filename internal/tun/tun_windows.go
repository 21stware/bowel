//go:build windows

// Package tun installs the core so that it can open a TUN device.
package tun

// On Windows, opening the TUN device is what needs the administrator,
// not installing the copy: the copy is a plain file, and the consent is
// asked once, when the core starts (proc_windows's runas), the way the
// setuid copy asks once on macOS.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/oboo/moho/internal/i18n"
)

var (
	shell32         = windows.NewLazySystemDLL("shell32.dll")
	procShellExecEx = shell32.NewProc("ShellExecuteExW")
)

// seeMaskNoCloseProcess is SEE_MASK_NOCLOSEPROCESS: ShellExecuteExW
// closes the process handle on return unless this bit is set.
const seeMaskNoCloseProcess = 0x40

// shellExecuteInfo is SHELLEXECUTEINFOW, as the 64-bit ABI lays it out.
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
	icon     windows.Handle
	process  windows.Handle
}

// ErrCanceled is returned by Install and Uninstall when the user dismissed
// the administrator prompt.
var ErrCanceled error = canceled{}

// canceled says it in the interface's language when it is said.
type canceled struct{}

func (canceled) Error() string {
	return i18n.T("用户取消了授权", "The authorization was canceled")
}

var errUnsupported = errors.New("not supported")

// Installed reports whether the core copy for TUN is there.
func Installed(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// Install copies the core, which needs no consent here; the core asks
// for the administrator itself when it starts.
func Install(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// Uninstall removes the copy.
func Uninstall(dst string) error {
	return os.Remove(dst)
}

// Step is one shell command run as the administrator.
type Step []string

// Prompt is what the administrator dialog says for TUN alone.
func Prompt() string {
	return i18n.T("bowel 需要管理员权限以启用 TUN 模式", "bowel needs the administrator's consent to turn on TUN mode")
}

// InstallStep is not used on Windows: the consent is asked at the core's
// start, not here.
func InstallStep(src, dst string) (Step, error) { return nil, errUnsupported }

// Admin runs the steps in one elevated cmd, asking the administrator
// once, and says whether the steps succeeded. Each step's pieces are
// joined with spaces; the whole runs under && so that a failure stops
// the rest, and cmd echoes nothing when told not to.
func Admin(prompt string, steps ...Step) error {
	if len(steps) == 0 {
		return nil
	}
	parts := make([]string, 0, len(steps)+2)
	parts = append(parts, "@echo off")
	for _, st := range steps {
		parts = append(parts, strings.Join(st, " "))
	}
	script := strings.Join(parts, " && ")
	f, err := os.CreateTemp("", "bowel-admin-*.cmd")
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
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	var si shellExecuteInfo
	si.cbSize = uint32(unsafe.Sizeof(si))
	si.fMask = seeMaskNoCloseProcess
	si.verb, si.file = verb, file
	r, _, callErr := procShellExecEx.Call(uintptr(unsafe.Pointer(&si)))
	if r == 0 {
		if errno, ok := callErr.(syscall.Errno); ok && errno == windows.ERROR_CANCELLED {
			return ErrCanceled
		}
		return fmt.Errorf("runas: %v", callErr)
	}
	h := si.process
	defer windows.CloseHandle(h)
	if ev, err := windows.WaitForSingleObject(h, windows.INFINITE); err != nil || ev != windows.WAIT_OBJECT_0 {
		if err != nil {
			return err
		}
		return fmt.Errorf("wait: %d", ev)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("%s (exit status %d)", i18n.T("管理员命令没有成功", "the administrator's commands didn't succeed"), code)
	}
	return nil
}
