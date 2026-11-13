package main

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// startGuard runs this executable as the guard (runHandoverGuard), with no
// console and in a process group of its own, so that it outlives the app.
// The executable's path is the one the app was started by: after an update
// that is the new version's file, which is a guard just as well.
func startGuard(dataDir string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "--data-dir", dataDir, guardCommand)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// askYesNo asks in a message box, on top of the other windows, with No as
// the button Enter presses, and is true only for Yes.
func askYesNo(msg string) bool {
	text, err := windows.UTF16PtrFromString(msg)
	if err != nil {
		return false
	}
	title, _ := windows.UTF16PtrFromString("bowel")
	const idYes = 6
	r, err := windows.MessageBox(0, text, title,
		windows.MB_YESNO|windows.MB_ICONWARNING|windows.MB_DEFBUTTON2|windows.MB_SETFOREGROUND|windows.MB_TOPMOST)
	return err == nil && r == idYes
}
