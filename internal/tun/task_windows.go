package tun

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/nowindow"
)

// Task is the core as the scheduled task runs it: elevated, not a child
// of the app, and in no session of the user's.
type Task struct {
	name string // the task's
	exe  string // the core's file name, as the process list has it
	pid  int

	once sync.Once
	done chan struct{}
}

// Spawn starts the core through the task when bin is the copy the
// installation made, and returns nil for any other: that one runs as a
// child. An instance a crash of the app left running is ended first.
func Spawn(bin string) (*Task, error) {
	if !Installed(bin) {
		return nil, nil
	}
	t := &Task{name: taskName(), exe: filepath.Base(bin), done: make(chan struct{})}
	_ = schtasks("/End", "/TN", t.name)
	before := pids(t.exe)
	if len(before) > 0 {
		// A core just ended may take a moment to go, and while it is
		// there the task starts no other.
		time.Sleep(500 * time.Millisecond)
		before = pids(t.exe)
	}
	if err := schtasks("/Run", "/TN", t.name); err != nil {
		return nil, fmt.Errorf(i18n.T("无法通过计划任务启动内核（重新开启 TUN 可以再授权一次）：%s",
			"Couldn't start the core through its scheduled task (turning TUN on again asks for the authorization once more): %s"), err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		for pid := range pids(t.exe) {
			if !before[pid] {
				t.pid = pid
				return t, nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, errors.New(i18n.T("计划任务没有把内核启动起来", "The scheduled task didn't start the core"))
}

func (t *Task) Pid() int { return t.pid }

// Wait returns once the core's process is gone. An elevated process's
// handle may be refused to the app, and then the process list is asked
// twice a second.
func (t *Task) Wait() error {
	t.once.Do(func() {
		defer close(t.done)
		if h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(t.pid)); err == nil {
			defer windows.CloseHandle(h)
			if _, err := windows.WaitForSingleObject(h, windows.INFINITE); err == nil {
				return
			}
		}
		for pids(t.exe)[t.pid] {
			time.Sleep(500 * time.Millisecond)
		}
	})
	<-t.done
	return nil
}

// Stop ends the task's running instance.
func (t *Task) Stop() { _ = schtasks("/End", "/TN", t.name) }

// schtasks runs the Task Scheduler's command, hidden.
func schtasks(args ...string) error {
	sys, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	cmd := nowindow.Command(filepath.Join(sys, "schtasks.exe"), args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
	}
	return err
}

// pids are the processes whose executable is named exe.
func pids(exe string) map[int]bool {
	out := map[int]bool{}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return out
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), exe) {
			out[int(e.ProcessID)] = true
		}
	}
	return out
}
