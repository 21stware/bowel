package main

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"

	"github.com/oboo/moho/internal/i18n"
)

// startGuard runs this executable as the guard (runHandoverGuard), with no
// console and in a process group of its own, so that it outlives the app.
// The executable's path is the one the app was started by: after an
// update that is the new version's file, which is a guard just as well.
// Not hidden as other programs are (nowindow): the hide flag would open
// its question hidden.
func startGuard(dataDir string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "--data-dir", dataDir, guardCommand)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// askStopLeftCore asks in a message box, on top of the other windows, with
// No as the button Enter presses, and is true only for Yes: a box that
// could not be shown keeps the proxy.
func askStopLeftCore() bool {
	msg, _, _ := stopLeftCoreText()
	text, err := windows.UTF16PtrFromString(msg + "\n\n" + i18n.T("选「是」停止代理，选「否」让它继续运行。", "Choose Yes to stop the proxy, No to keep it running."))
	if err != nil {
		return false
	}
	title, _ := windows.UTF16PtrFromString("bowel")
	const idYes = 6
	r, err := windows.MessageBox(0, text, title,
		windows.MB_YESNO|windows.MB_ICONWARNING|windows.MB_DEFBUTTON2|windows.MB_SETFOREGROUND|windows.MB_TOPMOST)
	return err == nil && r == idYes
}
