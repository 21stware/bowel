package speed

import (
	"testing"
	"time"

	"github.com/oboo/moho/internal/mihomo"
)

func node(all []mihomo.History, byURL map[string][]mihomo.History) mihomo.Proxy {
	p := mihomo.Proxy{History: all, Extra: map[string]mihomo.URLTests{}}
	for u, h := range byURL {
		p.Extra[u] = mihomo.URLTests{History: h}
	}
	return p
}

// A refusal is a failed test of the URL that has its delay among all the
// node's tests, at the same instant. A test nothing answered has none, and
// a test that passed is no failure.
func TestRefusalsTellARefusalFromNoAnswer(t *testing.T) {
	t0 := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
	fail := func(at time.Time) mihomo.History { return mihomo.History{Time: at} }
	took := func(at time.Time, ms int) mihomo.History { return mihomo.History{Time: at, Delay: ms} }
	var r Refusals
	r.Note(map[string]mihomo.Proxy{
		"hk": node([]mihomo.History{took(t0, 90), fail(t0.Add(time.Second)), took(t0.Add(2*time.Second), 70)},
			map[string][]mihomo.History{"refuses": {fail(t0)}, "dead": {fail(t0.Add(time.Second))}, "fine": {took(t0.Add(2*time.Second), 70)}}),
	})
	if !r.Is("hk", "refuses", fail(t0)) {
		t.Error("an answer with a status the site doesn't expect is not a refusal")
	}
	if r.Is("hk", "dead", fail(t0.Add(time.Second))) {
		t.Error("a test nothing answered is a refusal")
	}
	if r.Is("hk", "fine", took(t0.Add(2*time.Second), 70)) {
		t.Error("a test that passed is a refusal")
	}
	if r.Is("jp", "refuses", fail(t0)) {
		t.Error("another node's refusal")
	}
	var none *Refusals
	if none.Is("hk", "refuses", fail(t0)) {
		t.Error("nil knows of a refusal")
	}
}

// The core keeps a node's last ten tests of anything, so the test that
// shows a failure was answered is soon gone from that list: what was read
// while it was there still stands, until the URL is tested again.
func TestRefusalsRememberPastTheCoresList(t *testing.T) {
	t0 := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
	refusal := mihomo.History{Time: t0}
	var r Refusals
	r.Note(map[string]mihomo.Proxy{"hk": node([]mihomo.History{{Time: t0, Delay: 90}}, map[string][]mihomo.History{"u": {refusal}})})

	// Ten other tests later, the pair is gone, and the refusal is still
	// the node's last test of the URL.
	var later []mihomo.History
	for i := 1; i <= 10; i++ {
		later = append(later, mihomo.History{Time: t0.Add(time.Duration(i) * time.Minute), Delay: 50})
	}
	r.Note(map[string]mihomo.Proxy{"hk": node(later, map[string][]mihomo.History{"u": {refusal}})})
	if !r.Is("hk", "u", refusal) {
		t.Fatal("the refusal was forgotten once the core's list moved on")
	}

	// A newer test of the URL that nothing answered is not a refusal.
	dead := mihomo.History{Time: t0.Add(time.Hour)}
	r.Note(map[string]mihomo.Proxy{"hk": node(append(later, dead), map[string][]mihomo.History{"u": {refusal, dead}})})
	if r.Is("hk", "u", dead) || r.Is("hk", "u", refusal) {
		t.Fatal("an older refusal stands for a newer test")
	}

	// A node that is gone is forgotten.
	r.Note(map[string]mihomo.Proxy{"hk": node([]mihomo.History{{Time: t0, Delay: 90}}, map[string][]mihomo.History{"u": {refusal}})})
	r.Note(map[string]mihomo.Proxy{})
	if r.Is("hk", "u", refusal) {
		t.Fatal("a node that is gone is still refused")
	}
}
