package monitor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// stats is what statsPath holds: traffic of the current local day and
// month.
type stats struct {
	Date      string `json:"date"` // 2006-01-02, local time
	TodayDown int64  `json:"todayDown"`
	TodayUp   int64  `json:"todayUp"`
	Month     string `json:"month"` // 2006-01
	MonthDown int64  `json:"monthDown"`
}

func loadStats(path string) stats {
	var s stats
	if path == "" {
		return s
	}
	if b, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(b, &s) != nil {
			s = stats{}
		}
	}
	return s
}

// roll starts a new day or month when now is past the stored one, and
// reports whether it changed anything.
func (s *stats) roll(now time.Time) bool {
	now = now.Local()
	day, month := now.Format(time.DateOnly), now.Format("2006-01")
	changed := false
	if s.Date != day {
		s.Date, s.TodayDown, s.TodayUp = day, 0, 0
		changed = true
	}
	if s.Month != month {
		s.Month, s.MonthDown = month, 0
		changed = true
	}
	return changed
}

func (m *Monitor) addTotalsLocked(down, up int64, now time.Time) {
	if m.stats.roll(now) {
		m.dirty = true
	}
	if down <= 0 && up <= 0 {
		return
	}
	m.stats.TodayDown += max(down, 0)
	m.stats.TodayUp += max(up, 0)
	m.stats.MonthDown += max(down, 0)
	m.dirty = true
}

// saveIfDue writes the stats when they changed and the last write is at
// least saveEvery old, or always when force is set and they changed.
func (m *Monitor) saveIfDue(now time.Time, force bool) {
	m.mu.Lock()
	if m.statsPath == "" || !m.dirty || (!force && now.Sub(m.lastSave) < saveEvery) {
		m.mu.Unlock()
		return
	}
	s := m.stats
	m.dirty = false
	m.lastSave = now
	m.mu.Unlock()

	if err := m.writeStats(s); err != nil {
		m.mu.Lock()
		m.dirty = true
		m.mu.Unlock()
	}
}

func (m *Monitor) writeStats(s stats) error {
	m.saveMu.Lock()
	defer m.saveMu.Unlock()
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.statsPath), 0o700); err != nil {
		return err
	}
	tmp := m.statsPath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.statsPath)
}
