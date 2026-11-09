// Command launcher hands the VPN's TUN descriptor to mihomo on Android.
//
// A Java child process has every descriptor above stderr closed, so the
// service cannot pass the TUN straight to mihomo. It starts this launcher
// instead and sends the descriptor over a unix socket in the app's private
// folder. The launcher writes the descriptor's number into a copy of the
// config and replaces itself with mihomo, which keeps its process ID, its
// stdout and the descriptor.
package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	fdSocket     = "tunfd.sock"
	ctlSocket    = "c.sock"
	sourceConfig = "config.yaml"
	runConfig    = "run.yaml"
	tunFDFile    = "tun.fd"
	waitForFD    = 15 * time.Second

	prSetPdeathsig = 1
)

func init() {
	// PR_SET_PDEATHSIG belongs to a thread: set it and exec on the same one.
	runtime.LockOSThread()
}

func main() {
	if len(os.Args) != 3 {
		fail(fmt.Errorf("usage: %s <dir> <core>", os.Args[0]))
	}
	dir, core := os.Args[1], os.Args[2]
	fd, err := receiveFD(filepath.Join(dir, fdSocket))
	if err != nil {
		fail(fmt.Errorf("receive tun descriptor: %w", err))
	}
	// The app names the same descriptor when it reloads the config.
	if err := os.WriteFile(filepath.Join(dir, tunFDFile), []byte(strconv.Itoa(fd)), 0o600); err != nil {
		fail(err)
	}
	run := filepath.Join(dir, runConfig)
	if err := writeRunConfig(filepath.Join(dir, sourceConfig), run, fd); err != nil {
		fail(err)
	}
	ctl := filepath.Join(dir, ctlSocket)
	_ = os.Remove(ctl)
	// If the app dies, its core goes with it instead of holding the TUN open.
	if _, _, e := syscall.RawSyscall(syscall.SYS_PRCTL, prSetPdeathsig, uintptr(syscall.SIGTERM), 0); e != 0 {
		fail(fmt.Errorf("prctl: %w", e))
	}
	args := []string{"mihomo", "-d", dir, "-f", run, "-ext-ctl-unix", ctl}
	if err := syscall.Exec(core, args, os.Environ()); err != nil {
		fail(fmt.Errorf("exec core: %w", err))
	}
}

// receiveFD listens once and returns the descriptor the service sends. Go
// receives descriptors as close-on-exec, so that flag is cleared here for the
// descriptor to survive the exec.
func receiveFD(path string) (int, error) {
	_ = os.Remove(path)
	old := syscall.Umask(0o177)
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	syscall.Umask(old)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	if err := l.SetDeadline(time.Now().Add(waitForFD)); err != nil {
		return 0, err
	}
	c, err := l.AcceptUnix()
	if err != nil {
		return 0, err
	}
	defer c.Close()
	if err := c.SetDeadline(time.Now().Add(waitForFD)); err != nil {
		return 0, err
	}
	oob := make([]byte, syscall.CmsgSpace(4))
	buf := make([]byte, 1)
	_, oobn, _, _, err := c.ReadMsgUnix(buf, oob)
	if err != nil {
		return 0, err
	}
	msgs, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return 0, err
	}
	for i := range msgs {
		fds, err := syscall.ParseUnixRights(&msgs[i])
		if err != nil {
			return 0, err
		}
		if len(fds) > 0 {
			if _, _, e := syscall.RawSyscall(syscall.SYS_FCNTL, uintptr(fds[0]), syscall.F_SETFD, 0); e != 0 {
				return 0, fmt.Errorf("keep tun descriptor open: %w", e)
			}
			return fds[0], nil
		}
	}
	return 0, fmt.Errorf("the message carried no descriptor")
}

// writeRunConfig copies the config and sets what only this process knows.
func writeRunConfig(from, to string, fd int) error {
	raw, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	tun, _ := cfg["tun"].(map[string]any)
	if tun == nil {
		return fmt.Errorf("config has no tun section")
	}
	tun["file-descriptor"] = fd
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(to, out, 0o600)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "launcher:", err)
	os.Exit(1)
}
