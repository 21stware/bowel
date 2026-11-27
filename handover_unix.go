//go:build !windows

package main

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
)

// startGuard runs this executable as the guard (runHandoverGuard), in a
// session of its own so that it outlives the app and whatever started it.
func startGuard(dataDir string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "--data-dir", dataDir, guardCommand)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// askStopLeftCore asks with what the desktop has for it. Anything but the
// answer to stop, including no way to ask, leaves the proxy running.
func askStopLeftCore() bool {
	msg, stop, keep := stopLeftCoreText()
	ctx := context.Background()
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.CommandContext(ctx, "/usr/bin/osascript",
			"-e", "on run argv",
			"-e", `display dialog (item 1 of argv) with title "bowel" buttons {item 2 of argv, item 3 of argv} default button 2 with icon caution`,
			"-e", "button returned of result",
			"-e", "end run", msg, stop, keep).Output()
		return err == nil && strings.TrimSpace(string(out)) == stop
	case "linux":
		// Stopping is the affirmative answer in both, so that Esc, closing
		// the window and a failure all keep the proxy.
		if bin, err := exec.LookPath("zenity"); err == nil {
			return exec.CommandContext(ctx, bin, "--question", "--title", "bowel", "--text", msg,
				"--ok-label", stop, "--cancel-label", keep, "--default-cancel", "--width", "440").Run() == nil
		}
		if bin, err := exec.LookPath("kdialog"); err == nil {
			return exec.CommandContext(ctx, bin, "--title", "bowel", "--warningyesno", msg,
				"--yes-label", stop, "--no-label", keep).Run() == nil
		}
	}
	return false
}
