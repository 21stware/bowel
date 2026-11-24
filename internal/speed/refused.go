package speed

import (
	"sync"
	"time"

	"github.com/oboo/moho/internal/mihomo"
)

// Refusals tells, among the failed tests of a URL, the ones the site
// answered with a refusal from the ones nothing answered.
//
// The core says it in passing. A test whose answer has a status its URL
// does not expect (routes.Expected) is a failure among the node's tests of
// that URL, and stands with its delay among all the node's tests, at the
// same instant; a test nothing answered is a failure in both. The second
// list holds the node's last ten tests of anything, so the pair is there
// for a while only: Note reads it while it is, and Is remembers.
type Refusals struct {
	mu sync.Mutex
	at map[Target]time.Time // the failed test that was a refusal, by node and URL
}

// Note reads the tests of nodes, every node the core has: what is known of
// a node that is gone, or of a test that is no longer a node's last of its
// URL, is dropped.
func (r *Refusals) Note(nodes map[string]mihomo.Proxy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	next := map[Target]time.Time{}
	for name, p := range nodes {
		for url, tests := range p.Extra {
			if len(tests.History) == 0 {
				continue
			}
			h := tests.History[len(tests.History)-1]
			if h.Delay != 0 || h.Time.IsZero() {
				continue
			}
			k := Target{Node: name, URL: url}
			if r.at[k].Equal(h.Time) || answered(p.History, h.Time) {
				next[k] = h.Time
			}
		}
	}
	r.at = next
}

// Is says whether h, node's last test of url, was the site's refusal. A nil
// r knows of none.
func (r *Refusals) Is(node, url string, h mihomo.History) bool {
	if r == nil || h.Delay != 0 || h.Time.IsZero() {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.at[Target{Node: node, URL: url}].Equal(h.Time)
}

// answered says whether the test at instant at, among all a node's tests,
// got an answer.
func answered(all []mihomo.History, at time.Time) bool {
	for _, h := range all {
		if h.Delay > 0 && h.Time.Equal(at) {
			return true
		}
	}
	return false
}
