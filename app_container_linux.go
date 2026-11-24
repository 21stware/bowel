package main

import (
	"os"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
)

// reapOrphans waits for the zombies PID 1 inherits: in a container bowel
// is PID 1, every process whose parent ended becomes its child, and no
// one else would wait for one that ends. Its own children are waited for
// where they are started, at once, so a zombie is taken only once it has
// been seen twice, a scan apart: it is no one's to wait for then.
func reapOrphans(stop <-chan struct{}) {
	if os.Getpid() != 1 {
		return
	}
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	seen := map[int]bool{}
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}
		now := map[int]bool{}
		for _, pid := range zombieChildren() {
			if !seen[pid] {
				now[pid] = true
				continue
			}
			var ws unix.WaitStatus
			_, _ = unix.Wait4(pid, &ws, unix.WNOHANG, nil)
		}
		seen = now
	}
}

// zombieChildren are this process's children that are zombies.
func zombieChildren() []int {
	stats, _ := filepath.Glob("/proc/[0-9]*/stat")
	me := os.Getpid()
	var out []int
	for _, p := range stats {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if z, ppid, ok := statZombie(string(b)); ok && z && ppid == me {
			if pid, err := strconv.Atoi(filepath.Base(filepath.Dir(p))); err == nil {
				out = append(out, pid)
			}
		}
	}
	return out
}
