package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestStartCoreDetached: on macOS the core is not the app's child (launchd
// is its parent, or it could not see who opens connections), and still its
// output is read, its arguments arrive whole, its exit is noticed, and it
// can be stopped.
func TestStartCoreDetached(t *testing.T) {
	var out lockedBuf
	arg := `a "b" $HOME; c`
	p, err := startCore("/bin/sh", []string{"-c", `echo "got:$1"; echo oops >&2; exec sleep 30`, "sh", arg}, t.TempDir(), nil, &out)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(out.String(), "oops") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := out.String(); !strings.Contains(got, "got:"+arg+"\n") || !strings.Contains(got, "oops") {
		t.Errorf("output: %q", got)
	}
	ppid, err := exec.Command("ps", "-o", "ppid=", "-p", strconv.Itoa(p.Pid())).Output()
	if err != nil || strings.TrimSpace(string(ppid)) != "1" {
		t.Errorf("the core's parent is %q (%v), not launchd", strings.TrimSpace(string(ppid)), err)
	}
	done := make(chan struct{})
	go func() { p.Wait(); close(done) }()
	select {
	case <-done:
		t.Fatal("Wait returned while the core runs")
	case <-time.After(300 * time.Millisecond):
	}
	p.Stop()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Wait did not return after Stop")
	}
}

// TestStartCoreDetachedInsists: a core that ignores being asked is killed
// when asked again, as Manager.Stop does after its grace.
func TestStartCoreDetachedInsists(t *testing.T) {
	var out lockedBuf
	p, err := startCore("/bin/sh", []string{"-c", `trap "" TERM; echo ready; while :; do sleep 1; done`}, t.TempDir(), nil, &out)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100 && !strings.Contains(out.String(), "ready"); i++ {
		time.Sleep(20 * time.Millisecond)
	}
	done := make(chan struct{})
	go func() { p.Wait(); close(done) }()
	p.Stop()
	select {
	case <-done:
		t.Fatal("it went at the first asking; the test proves nothing")
	case <-time.After(400 * time.Millisecond):
	}
	p.Stop()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("still there after being killed")
	}
}

// TestStartCoreDetachedMissing: a program that isn't there is an exit that
// is noticed, with the shell's word on it in the output.
func TestStartCoreDetachedMissing(t *testing.T) {
	var out lockedBuf
	p, err := startCore("/nonexistent/mihomo", nil, t.TempDir(), nil, &out)
	if err != nil {
		return // refused outright is as good
	}
	done := make(chan struct{})
	go func() { p.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("no exit noticed")
	}
	if !strings.Contains(out.String(), "nonexistent") {
		t.Errorf("no reason in the output: %q", out.String())
	}
}

// TestStartCoreDetachedToFile: given a file, the core prints into it
// itself, with no pipe that would end it at its next line once the app
// is gone.
func TestStartCoreDetachedToFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "core.log")
	f, err := openLog(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := startCore("/bin/sh", []string{"-c", `echo one; echo two >&2; exec sleep 30`}, t.TempDir(), nil, f)
	f.Close() // as the manager does: the core has its own
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	var got []byte
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if got, _ = os.ReadFile(path); strings.Contains(string(got), "two") {
			break
		}
	}
	if string(got) != "one\ntwo\n" {
		t.Errorf("the file has %q", got)
	}
	if fds, err := exec.Command("lsof", "-a", "-p", strconv.Itoa(p.Pid()), "-d", "1", "-F", "t").Output(); err == nil && !strings.Contains(string(fds), "tREG") {
		t.Errorf("the core's output is not the file: %q", fds)
	}
}
