//go:build !windows

package core

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func interrupt(cmd *exec.Cmd) { _ = cmd.Process.Signal(syscall.SIGTERM) }

// liveCore is the pid the pid file names while that is a running mihomo,
// else 0.
func liveCore(pidFile string) int {
	b, err := os.ReadFile(pidFile)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 1 {
		return 0
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil || !strings.Contains(string(out), "mihomo") {
		return 0
	}
	return pid
}

// adopted is a core another run of the app started: not a child, so it is
// known by its pid alone.
type adopted struct{ pid int }

func (a adopted) Pid() int { return a.pid }

// Wait returns once the process is gone. Asking is all there is for a
// process that is not a child.
func (a adopted) Wait() error {
	for alive(a.pid) {
		time.Sleep(200 * time.Millisecond)
	}
	return nil
}

func (a adopted) Stop() { _ = syscall.Kill(a.pid, syscall.SIGTERM) }
func (a adopted) kill() { _ = syscall.Kill(a.pid, syscall.SIGKILL) }

// alive: the process exists. One this user may not signal exists too.
func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
