//go:build windows

package core

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

func interrupt(cmd *exec.Cmd) { _ = cmd.Process.Kill() }

// killStale is not implemented on Windows yet.
func killStale(string) {}
