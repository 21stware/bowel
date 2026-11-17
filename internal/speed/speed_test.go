package speed

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// settle waits for q to have nothing queued or running.
func settle[K comparable](t *testing.T, q *Queue[K]) {
	t.Helper()
	until(t, func() bool {
		q.mu.Lock()
		defer q.mu.Unlock()
		return len(q.calls) == 0 && q.busy == 0
	})
}

func until(t *testing.T, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); !ok(); time.Sleep(time.Millisecond) {
		if time.Now().After(end) {
			t.Fatal("timed out")
		}
	}
}

// counted is a test function that notes what it was asked and waits at gate.
type counted struct {
	mu    sync.Mutex
	order []string
	gate  chan struct{}
}

func (c *counted) test(k string) bool {
	c.mu.Lock()
	c.order = append(c.order, k)
	c.mu.Unlock()
	if c.gate != nil {
		<-c.gate
	}
	return true
}

func (c *counted) ran() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.order)
}

func TestWantSkipsFreshAndRunning(t *testing.T) {
	c := &counted{gate: make(chan struct{})}
	q := New(2, c.test)
	q.Want(time.Hour, []string{"a"}, nil)
	until(t, func() bool { return len(c.ran()) == 1 })
	q.Want(time.Hour, []string{"a", "b"}, nil) // a is already asked for
	close(c.gate)
	settle(t, q)
	q.Want(time.Hour, []string{"a", "b"}, nil) // both were just tested
	now := func(string) time.Time { return time.Now() }
	q.Want(time.Hour, []string{"c"}, now) // someone else just tested c
	settle(t, q)
	if got := c.ran(); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("ran %v, want a and b once each", got)
	}
	long := func(string) time.Time { return time.Now().Add(-2 * time.Hour) }
	q.Want(time.Nanosecond, []string{"a"}, long) // its own test is older than a nanosecond
	settle(t, q)
	q.Want(time.Hour, []string{"d"}, long)
	settle(t, q)
	if got := c.ran(); !slices.Equal(got, []string{"a", "b", "a", "d"}) {
		t.Fatalf("ran %v, want a again and then d", got)
	}
}

// What is wanted is what the last Want named: a list scrolled past must
// not leave its rows behind in the queue. What Now asked for stays.
func TestWantDropsWhatIsNoLongerWanted(t *testing.T) {
	c := &counted{gate: make(chan struct{})}
	q := New(1, c.test)
	q.Want(time.Hour, []string{"a", "b", "c", "d"}, nil)
	until(t, func() bool { return len(c.ran()) == 1 }) // a runs, the others wait
	go q.Now(context.Background(), []string{"d"})
	until(t, func() bool {
		q.mu.Lock()
		defer q.mu.Unlock()
		return slices.Equal(q.queue, []string{"d", "b", "c"})
	})
	q.Want(time.Hour, []string{"c", "e"}, nil) // b and d are no longer on screen
	close(c.gate)
	settle(t, q)
	if got := c.ran(); !slices.Equal(got, []string{"a", "d", "c", "e"}) {
		t.Fatalf("ran %v, want a d c e", got)
	}
	q.Want(time.Hour, []string{"b"}, nil) // dropped is not tested: it is still due
	settle(t, q)
	if got := c.ran(); len(got) != 5 || got[4] != "b" {
		t.Fatalf("ran %v, want b at last", got)
	}
}

// A test that could not run (no core yet) is no test: its key is due as
// soon as it is wanted again, not an interval later.
func TestATestThatDidNotRunIsStillDue(t *testing.T) {
	var n atomic.Int32
	q := New(1, func(string) bool { return n.Add(1) > 1 })
	q.Want(time.Hour, []string{"a"}, nil)
	settle(t, q)
	q.Want(time.Hour, []string{"a"}, nil)
	settle(t, q)
	q.Want(time.Hour, []string{"a"}, nil) // the second one ran
	settle(t, q)
	if n.Load() != 2 {
		t.Fatalf("asked %d times, want 2", n.Load())
	}
}

func TestLimit(t *testing.T) {
	var now, most atomic.Int32
	var total atomic.Int32
	q := New(3, func(int) bool {
		n := now.Add(1)
		for {
			m := most.Load()
			if n <= m || most.CompareAndSwap(m, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		total.Add(1)
		now.Add(-1)
		return true
	})
	keys := make([]int, 20)
	for i := range keys {
		keys[i] = i
	}
	q.Now(context.Background(), keys)
	if total.Load() != 20 || most.Load() != 3 {
		t.Fatalf("%d tests, %d at once; want 20 and 3", total.Load(), most.Load())
	}
}

func TestNowJoinsAndGoesFirst(t *testing.T) {
	c := &counted{gate: make(chan struct{})}
	q := New(1, c.test)
	q.Want(time.Hour, []string{"a", "b", "c"}, nil)
	until(t, func() bool { return len(c.ran()) == 1 }) // a runs, b and c wait
	done := make(chan struct{})
	go func() {
		q.Now(context.Background(), []string{"c", "d", "a", "c"})
		close(done)
	}()
	until(t, func() bool {
		q.mu.Lock()
		defer q.mu.Unlock()
		return slices.Equal(q.queue, []string{"c", "d", "b"})
	})
	for i := range 3 { // a, c and d finish
		select {
		case <-done:
			t.Fatalf("returned after %d tests", i)
		default:
		}
		c.gate <- struct{}{}
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Now did not return when its tests were done")
	}
	c.gate <- struct{}{}
	settle(t, q)
	if got := c.ran(); !slices.Equal(got, []string{"a", "c", "d", "b"}) {
		t.Fatalf("ran %v, want a c d b", got)
	}
}

func TestNowTestsWhatIsFresh(t *testing.T) {
	c := &counted{}
	q := New(1, c.test)
	q.Now(context.Background(), []string{"a"})
	q.Now(context.Background(), []string{"a"})
	q.Now(context.Background(), nil)
	if got := c.ran(); !slices.Equal(got, []string{"a", "a"}) {
		t.Fatalf("ran %v, want a twice", got)
	}
}

func TestNowReturnsWhenContextEnds(t *testing.T) {
	c := &counted{gate: make(chan struct{})}
	q := New(1, c.test)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		q.Now(ctx, []string{"a", "b"})
		close(done)
	}()
	until(t, func() bool { return len(c.ran()) == 1 })
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Now did not return when its context ended")
	}
	close(c.gate)
	settle(t, q)
	if got := c.ran(); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("ran %v: what was asked goes on without the asker", got)
	}
}

func TestIdle(t *testing.T) {
	var idle atomic.Int32
	c := &counted{}
	q := New(4, c.test)
	q.Idle = func() { idle.Add(1) }
	q.Now(context.Background(), []string{"a", "b", "c", "d", "e"})
	until(t, func() bool { return idle.Load() == 1 })
	q.Want(time.Hour, []string{"a"}, nil) // fresh: nothing starts, nothing ends
	time.Sleep(20 * time.Millisecond)
	if n := idle.Load(); n != 1 {
		t.Fatalf("Idle was called %d times, want once", n)
	}
}

// A core that reloads forgets its results: what was tested before is due
// again, however lately that was.
func TestForgetMakesWhatWasTestedDueAgain(t *testing.T) {
	c := &counted{}
	q := New(1, c.test)
	q.Now(context.Background(), []string{"a"})
	q.Want(time.Hour, []string{"a"}, nil) // just tested
	settle(t, q)
	q.Forget()
	q.Want(time.Hour, []string{"a"}, nil)
	settle(t, q)
	if got := c.ran(); !slices.Equal(got, []string{"a", "a"}) {
		t.Fatalf("ran %v, want a again after Forget", got)
	}
}

// A test that was running when the core reloaded left its result with the
// old one: it does not count either.
func TestForgetLeavesOutATestStillRunning(t *testing.T) {
	c := &counted{gate: make(chan struct{})}
	q := New(1, c.test)
	q.Want(time.Hour, []string{"a"}, nil)
	until(t, func() bool { return len(c.ran()) == 1 })
	q.Forget()
	close(c.gate)
	settle(t, q)
	q.Want(time.Hour, []string{"a"}, nil)
	settle(t, q)
	if got := c.ran(); !slices.Equal(got, []string{"a", "a"}) {
		t.Fatalf("ran %v, want a again: its first test ended after Forget", got)
	}
}

// A key named more than once is tested once, and what an asker that has
// already left asked for is tested all the same.
func TestNowWithDuplicatesAndNoOneWaiting(t *testing.T) {
	c := &counted{}
	q := New(2, c.test)
	q.Now(context.Background(), []string{"a", "a", "b", "a"})
	got := c.ran()
	slices.Sort(got)
	if !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("ran %v, want a and b once each", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q.Now(ctx, []string{"c"})
	settle(t, q)
	if got := c.ran(); len(got) != 3 || got[2] != "c" {
		t.Fatalf("ran %v, want c after them", got)
	}
}
