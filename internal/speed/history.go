package speed

import (
	"time"

	"github.com/oboo/moho/internal/mihomo"
)

// StaleAfter is how long a failed test stands: three rounds of the slower
// of the core's two timers, given in seconds. The core tests a node again
// only while traffic passes, so a failure from before a quiet stretch would
// otherwise be the node's last word for as long as the quiet lasts.
func StaleAfter(healthInterval, testInterval int) time.Duration {
	return 3 * time.Duration(max(healthInterval, testInterval)) * time.Second
}

// Last is the last of tests, a node's tests of one URL in the core's
// order, and whether there is one. A failure older than stale is none: the
// node reads as not tested. A success stands however old.
func Last(tests []mihomo.History, stale time.Duration, now time.Time) (mihomo.History, bool) {
	if len(tests) == 0 {
		return mihomo.History{}, false
	}
	h := tests[len(tests)-1]
	if h.Delay == 0 && !h.Time.IsZero() && now.Sub(h.Time) > stale {
		return mihomo.History{}, false
	}
	return h, true
}
