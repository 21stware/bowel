package main

import (
	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/tun"
)

// spawnTun starts the core the way TUN's installation needs where that is
// not as a child of the app: on Windows, through the scheduled task that
// runs the protected copy elevated. For any other core, and on the other
// systems always, it starts nothing and the manager does.
// attachTun is spawnTun's other half: a core that is already running, left
// by the last version for this one, as the task's when the task runs it.
func attachTun(bin string, pid int) core.Process {
	if t := tun.Attach(bin, pid); t != nil {
		return t
	}
	return nil
}

func spawnTun(bin string) (core.Process, error) {
	t, err := tun.Spawn(bin)
	if t == nil {
		return nil, err
	}
	return t, err
}
