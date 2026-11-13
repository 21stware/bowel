package main

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/oboo/moho/internal/core"
)

// fakeCore is a core as the reviver sees it.
type fakeCore struct {
	mu       sync.Mutex
	state    core.State
	quitting bool
	applies  int
	slept    []time.Duration
	applyErr error
	r        *reviver
	// afterApply runs once a start has come to run, with the try's number.
	afterApply func(n int)
	done       chan struct{} // closed by the test's last apply
}

func newFakeCore() (*fakeCore, *reviver) {
	f := &fakeCore{state: core.Stopped, done: make(chan struct{})}
	f.r = &reviver{
		state:    func() core.State { f.mu.Lock(); defer f.mu.Unlock(); return f.state },
		quitting: func() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.quitting },
		// The wait is recorded, not waited; a moment is, so that a test
		// that lets starts fail does not spin through thousands of them.
		sleep: func(d time.Duration) {
			f.mu.Lock()
			f.slept = append(f.slept, d)
			f.mu.Unlock()
			time.Sleep(time.Millisecond)
		},
	}
	f.r.apply = func() error {
		f.mu.Lock()
		f.applies++
		n, err, after := f.applies, f.applyErr, f.afterApply
		f.mu.Unlock()
		if err != nil {
			return err
		}
		f.set(core.Running)
		if after != nil {
			after(n)
		}
		return nil
	}
	return f, f.r
}

// set changes the core's state and tells the reviver, as the manager does.
func (f *fakeCore) set(s core.State) {
	f.mu.Lock()
	f.state = s
	f.mu.Unlock()
	f.r.note(core.Status{State: s})
}

func (f *fakeCore) wait(t *testing.T) {
	t.Helper()
	select {
	case <-f.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the reviver did not get there")
	}
	// The goroutine ends once it has seen the core run.
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		f.r.mu.Lock()
		w := f.r.waiting
		f.r.mu.Unlock()
		if !w {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the reviver is still at it")
		}
	}
}

// A core that ran and quit is started again, once.
func TestReviveStartsACoreThatQuit(t *testing.T) {
	f, _ := newFakeCore()
	f.afterApply = func(int) { close(f.done) }
	f.set(core.Running)
	f.set(core.Failed)
	f.wait(t)
	if f.applies != 1 || len(f.slept) != 1 || f.slept[0] != reviveDelays[0] {
		t.Fatalf("applies %d, slept %v", f.applies, f.slept)
	}
}

// A start that fails was asked for by someone, who is told: no retry.
func TestReviveLeavesAFailedStart(t *testing.T) {
	f, r := newFakeCore()
	f.set(core.Starting)
	f.set(core.Failed)
	r.mu.Lock()
	w := r.waiting
	r.mu.Unlock()
	if w || f.applies != 0 {
		t.Fatalf("waiting %v, applies %d", w, f.applies)
	}
}

// A core that keeps quitting is tried at growing intervals, and so is one
// that does not start; neither is given up on.
func TestReviveBacksOff(t *testing.T) {
	f, _ := newFakeCore()
	f.applyErr = errors.New("no")
	f.afterApply = func(n int) {
		if n < 6 {
			go f.set(core.Failed) // quits again at once
		} else {
			close(f.done)
		}
	}
	f.set(core.Running)
	f.mu.Lock()
	f.state = core.Failed // as a failed start leaves it
	f.mu.Unlock()
	f.r.note(core.Status{State: core.Failed})
	// Two starts fail, then they work and the core quits each time.
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		f.mu.Lock()
		n := f.applies
		if n >= 2 {
			f.applyErr = nil
		}
		f.mu.Unlock()
		if n >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no retries of a start that fails")
		}
	}
	f.wait(t)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.slept) < 6 {
		t.Fatalf("slept %v", f.slept)
	}
	for i := 1; i < len(f.slept); i++ {
		if f.slept[i] < f.slept[i-1] {
			t.Fatalf("the waits shrink: %v", f.slept)
		}
	}
	if last := f.slept[len(f.slept)-1]; last < 15*time.Second {
		t.Fatalf("the waits did not grow: %v", f.slept)
	}
}

// Quitting, or a core someone stopped, ends it.
func TestReviveStopsWhenQuitting(t *testing.T) {
	f, r := newFakeCore()
	f.set(core.Running)
	f.mu.Lock()
	f.quitting = true
	f.mu.Unlock()
	f.set(core.Failed)
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		r.mu.Lock()
		w := r.waiting
		r.mu.Unlock()
		if !w {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("still waiting")
		}
	}
	if f.applies != 0 {
		t.Fatalf("applies %d", f.applies)
	}
}
