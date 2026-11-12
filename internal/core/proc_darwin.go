package core

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/oboo/moho/internal/nowindow"
)

// detached is the core started so that it is not the app's child.
//
// On macOS 27 the kernel's socket list holds only the asking process's own
// sockets when its parent is an app (or any program that is not Apple's),
// however either is signed, and the core needs that list to tell which
// program opened a connection: without it no rule for an application
// matches, and the core names no process. What counts is the parent the
// core has now. So a shell starts it in the background and exits, which
// leaves the core to launchd. That is one of two conditions, measured on
// macOS 27.2; the other is how the core is signed, which BinName sees to
// in a release (a `mygo dev` build's signature never passes).
//
// It is the same process otherwise: its output still comes on a pipe it
// inherited, so a core that outlives the app ends at its next line, as a
// child's would; and signals reach it by its pid. Only waiting is
// different, since only a parent can wait: the pid is polled.
type detached struct {
	pid   int
	gone  chan struct{} // closed once the process is gone and its output read
	stops atomic.Int32
}

// startCore runs the core detached from the app, its output going to out.
func startCore(bin string, args []string, dir string, env []string, out io.Writer) (Process, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer w.Close()
	// $0 and "$@" are the core and its arguments, so nothing of theirs is
	// read as shell. The pipe is descriptor 3 here, 1 and 2 in the core.
	c := nowindow.Command("/bin/sh", append([]string{"-c", `"$0" "$@" </dev/null >&3 2>&3 3>&- & echo $!`, bin}, args...)...)
	c.Dir = dir
	c.Env = env
	c.ExtraFiles = []*os.File{w}
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	b, err := c.Output()
	if err != nil {
		r.Close()
		return nil, fmt.Errorf("start %s: %w", bin, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 1 {
		r.Close()
		return nil, errors.New("start " + bin + ": no pid from the shell")
	}
	d := &detached{pid: pid, gone: make(chan struct{})}
	var read sync.WaitGroup
	read.Add(1)
	go func() {
		defer read.Done()
		defer r.Close()
		_, _ = io.Copy(out, r)
	}()
	go func() {
		for d.alive() {
			time.Sleep(100 * time.Millisecond)
		}
		read.Wait() // what it printed last is the reason it went
		close(d.gone)
	}()
	return d, nil
}

// alive reports whether the process is still there; one that may not be
// signalled is.
func (d *detached) alive() bool { return syscall.Kill(d.pid, 0) != syscall.ESRCH }

func (d *detached) Pid() int { return d.pid }

// Wait returns once the core has exited. How it exited is the parent's to
// know, which the app is not: the core's last lines say.
func (d *detached) Wait() error {
	<-d.gone
	return nil
}

// Stop asks the core to end; asked again, it kills it.
func (d *detached) Stop() {
	sig := syscall.SIGTERM
	if d.stops.Add(1) > 1 {
		sig = syscall.SIGKILL
	}
	_ = syscall.Kill(d.pid, sig)
}
