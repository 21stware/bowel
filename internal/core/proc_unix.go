//go:build !windows

package core

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func interrupt(cmd *exec.Cmd) { _ = cmd.Process.Signal(syscall.SIGTERM) }

// killStale ends a core a previous run of the app left behind (after a
// crash), when the pid file still names a core: a program called mihomo,
// as the copy TUN runs on and the cores of earlier versions are, or
// BinName.
func killStale(pidFile string) {
	b, err := os.ReadFile(pidFile)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 1 {
		return
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil || !isCore(string(out)) {
		return
	}
	_ = syscall.Kill(pid, syscall.SIGTERM)
}

// isCore reports whether comm, as ps prints a process's command, is a core.
func isCore(comm string) bool {
	return strings.Contains(comm, "mihomo") || strings.Contains(comm, BinName)
}
