package main

// A core that quits by itself is started again. With the system proxy, TUN
// or the gateway pointing at it, a core that stays down is a computer, or a
// whole LAN, without a network until someone looks; nothing but bowel is
// there to start it on a machine that runs as a gateway.
//
// Only a core that had come to run counts: a start that fails is told to
// whoever asked for it, and is not tried again behind their back. A core
// that keeps quitting is tried less and less often, up to once a minute,
// and never given up on.

import (
	"log"
	"sync"
	"time"

	"github.com/oboo/moho/internal/core"
)

// reviveDelays are the waits before each try in a row; the last one repeats.
var reviveDelays = []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 15 * time.Second, 30 * time.Second, time.Minute}

// reviveCalm is how long a core must have run for its quitting to count as
// the first in a row again.
const reviveCalm = time.Minute

// reviver starts a core that quit. What it asks of the app are functions,
// which tests replace.
type reviver struct {
	state    func() core.State
	quitting func() bool
	apply    func() error
	sleep    func(time.Duration)

	mu      sync.Mutex
	ranAt   time.Time // when the core came to run; zero while it does not
	fails   int       // tries in a row since a core last ran for reviveCalm
	waiting bool      // a goroutine is at it
}

// note is told every status of the core.
func (r *reviver) note(s core.Status) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch s.State {
	case core.Running:
		r.ranAt = time.Now()
	case core.Stopped:
		r.ranAt = time.Time{}
	case core.Failed:
		if r.ranAt.IsZero() {
			return // it never ran: a start that failed, its caller's to report
		}
		if time.Since(r.ranAt) >= reviveCalm {
			r.fails = 0
		}
		r.ranAt = time.Time{}
		if !r.waiting {
			r.waiting = true
			go r.run()
		}
	}
}

func (r *reviver) run() {
	for {
		r.mu.Lock()
		if r.quitting() || r.state() != core.Failed {
			// Under the lock, so that a core that quits right now finds
			// waiting false and starts a goroutine of its own.
			r.waiting = false
			r.mu.Unlock()
			return
		}
		d := reviveDelays[min(r.fails, len(reviveDelays)-1)]
		r.fails++
		r.mu.Unlock()

		r.sleep(d)
		// Someone may have started it, stopped it or quit meanwhile.
		if r.quitting() || r.state() != core.Failed {
			continue
		}
		log.Printf("core: quit by itself, starting it again (waited %s)", d)
		if err := r.apply(); err != nil {
			log.Println("core: starting it again:", err)
		}
	}
}
