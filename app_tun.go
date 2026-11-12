package main

import (
	"errors"
	"log"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/tun"
)

// spawnTun starts the core the way TUN's installation needs where that is
// not as a child of the app: on Windows, through the scheduled task that
// runs the protected copy elevated. For any other core, and on the other
// systems always, it starts nothing and the manager does.
func spawnTun(bin string) (core.Process, error) {
	t, err := tun.Spawn(bin)
	if t == nil {
		return nil, err
	}
	return t, err
}

// noteTunInstall puts how TUN's installation went among the logs, where
// there is something to say: why it failed (on Windows, the part of the
// elevated command that stopped and what Windows said), or what an
// installation that was made is without. A prompt the user dismissed is
// not a failure.
func (a *App) noteTunInstall(err error) {
	text := ""
	switch {
	case err == nil:
		text = osTun.note(a.tunBin)
	case !errors.Is(err, tun.ErrCanceled):
		text = err.Error()
	}
	if text == "" {
		return
	}
	log.Println("tun: installation:", text)
	a.mon.Note("bowel: tun: " + text)
}
