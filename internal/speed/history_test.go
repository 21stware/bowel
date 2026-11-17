package speed

import (
	"testing"
	"time"

	"github.com/oboo/moho/internal/mihomo"
)

func TestLastExpiresOldFailures(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration, ms int) mihomo.History { return mihomo.History{Time: now.Add(-d), Delay: ms} }
	stale := 30 * time.Minute
	for _, c := range []struct {
		name  string
		tests []mihomo.History
		delay int
		ok    bool
	}{
		{"never tested", nil, 0, false},
		{"a success", []mihomo.History{ago(time.Minute, 120)}, 120, true},
		{"an old success still shows", []mihomo.History{ago(5*time.Hour, 120)}, 120, true},
		{"a failure", []mihomo.History{ago(time.Hour, 120), ago(29*time.Minute, 0)}, 0, true},
		{"an old failure is no result", []mihomo.History{ago(time.Hour, 120), ago(31*time.Minute, 0)}, 0, false},
		{"a failure without a time is kept", []mihomo.History{{Delay: 0}}, 0, true},
	} {
		h, ok := Last(c.tests, stale, now)
		if ok != c.ok || h.Delay != c.delay {
			t.Errorf("%s: %d, %v; want %d, %v", c.name, h.Delay, ok, c.delay, c.ok)
		}
	}
}

func TestStaleAfter(t *testing.T) {
	if d := StaleAfter(600, 300); d != 30*time.Minute {
		t.Errorf("defaults: %v, want 30m", d)
	}
	if d := StaleAfter(60, 900); d != 45*time.Minute {
		t.Errorf("a longer test interval: %v, want 45m", d)
	}
}
