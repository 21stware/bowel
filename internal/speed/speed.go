// Package speed decides when a speed test runs: every asker goes through
// one queue, which tests a thing once however many ask, a few at a time,
// and leaves alone what was tested not long ago. What a test is, and where
// its result is kept, is the caller's.
package speed

import (
	"context"
	"slices"
	"sync"
	"time"
)

// Target is a URL requested through a node.
type Target struct{ Node, URL string }

// Queue runs the tests of the things K names.
type Queue[K comparable] struct {
	// Idle, if set before the first test, is called when the last one
	// asked for has finished.
	Idle func()

	limit int
	test  func(K) bool

	mu    sync.Mutex
	calls map[K]*call     // asked for: waiting or running
	queue []K             // the ones waiting, the next one first
	busy  int             // goroutines running tests
	at    map[K]time.Time // when each was last tested here
}

type call struct {
	running bool
	now     bool // Now asked for it: someone waits
	done    chan struct{}
}

// New is a queue that runs test for at most limit things at once. test
// returns when the test is over: it carries its own timeout. It returns
// false when it could not test at all, which leaves the thing due.
func New[K comparable](limit int, test func(K) bool) *Queue[K] {
	return &Queue[K]{limit: limit, test: test, calls: map[K]*call{}, at: map[K]time.Time{}}
}

// Want asks for a test of each of keys whose last one is older than age,
// and returns at once. tested, if not nil, says when someone else last
// tested a key: a test is a test, whoever ran it. keys are all that is
// wanted: what an earlier Want asked for, has not started and is not among
// them is dropped.
func (q *Queue[K]) Want(age time.Duration, keys []K, tested func(K) time.Time) {
	now := time.Now()
	last := make([]time.Time, len(keys))
	if tested != nil {
		for i, k := range keys {
			last[i] = tested(k)
		}
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue = slices.DeleteFunc(q.queue, func(k K) bool {
		if q.calls[k].now || slices.Contains(keys, k) {
			return false
		}
		delete(q.calls, k)
		return true
	})
	for i, k := range keys {
		if q.calls[k] != nil || now.Sub(q.at[k]) < age || now.Sub(last[i]) < age {
			continue
		}
		q.calls[k] = &call{done: make(chan struct{})}
		q.queue = append(q.queue, k)
	}
	q.start()
}

// Now tests keys whatever their age, ahead of what Want asked for, and
// returns when they are done or ctx ends; the tests go on either way. A
// key already being tested is waited for, not tested twice.
func (q *Queue[K]) Now(ctx context.Context, keys []K) {
	q.mu.Lock()
	var first []K
	var wait []*call
	for _, k := range keys {
		c := q.calls[k]
		if c == nil {
			c = &call{done: make(chan struct{})}
			q.calls[k] = c
		} else if c.running || slices.Contains(first, k) {
			wait = append(wait, c)
			continue
		}
		c.now = true
		first = append(first, k)
		wait = append(wait, c)
	}
	q.queue = append(first, slices.DeleteFunc(q.queue, func(k K) bool { return slices.Contains(first, k) })...)
	q.start()
	q.mu.Unlock()
	for _, c := range wait {
		select {
		case <-c.done:
		case <-ctx.Done():
			return
		}
	}
}

// start has as many goroutines testing as the queue has work for and the
// limit allows. q.mu is held.
func (q *Queue[K]) start() {
	for n := min(q.limit-q.busy, len(q.queue)); n > 0; n-- {
		q.busy++
		go q.work()
	}
}

func (q *Queue[K]) work() {
	for {
		q.mu.Lock()
		if len(q.queue) == 0 {
			q.busy--
			idle := q.Idle
			if q.busy > 0 {
				idle = nil
			}
			q.mu.Unlock()
			if idle != nil {
				idle()
			}
			return
		}
		k := q.queue[0]
		q.queue = q.queue[1:]
		c := q.calls[k]
		c.running = true
		q.mu.Unlock()
		ran := q.test(k)
		q.mu.Lock()
		if ran {
			q.at[k] = time.Now()
		}
		delete(q.calls, k)
		q.mu.Unlock()
		close(c.done)
	}
}
