//go:build !windows

package main

import (
	"os"
	"os/exec"
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

// askYesNo is Windows's way of asking.
func askYesNo(string) bool { return false }
