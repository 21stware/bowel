package sysproxy

import (
	"fmt"
	"sync"

	"github.com/oboo/moho/internal/i18n"
)

// Written is what the last Enable gave the OS, for a report to show. Only
// Windows fills it: there the OS takes the bypass list in a form of its
// own, and may refuse it.
type Written struct {
	// Bypass is the bypass list as it was written, in the OS's own form:
	// empty when none was.
	Bypass string `json:"bypass"`
	// Dropped are the entries of bowel's list that were left out of it.
	Dropped []Dropped `json:"dropped,omitempty"`
	// Refused is the value the OS would not take, if it refused one.
	Refused *Refused `json:"refused,omitempty"`
}

// Dropped is an entry kept out of the bypass list. Why is "ipv6" (WinINet
// refuses the whole write for one), "malformed" (not a host, a wildcard or
// an IPv4 network) or "length" (the list was full).
type Dropped struct {
	Entry string `json:"entry"`
	Why   string `json:"why"`
}

// Refused is a value the OS refused, under the name its settings give it
// (ProxyOverride, ProxyServer).
type Refused struct {
	Value   string `json:"value"`
	Content string `json:"content"`
	Error   string `json:"error"`
	// Instead is what was done about it: "minimal" (the short bypass list
	// was written), "kept" (the system's bypass list was left as it was)
	// or "" (nothing: the takeover failed).
	Instead string `json:"instead,omitempty"`
}

// Empty says nothing was written.
func (w Written) Empty() bool { return w.Bypass == "" && len(w.Dropped) == 0 && w.Refused == nil }

// Note tells the user what the OS refused of a takeover that worked, or
// nothing.
func (w Written) Note() string {
	if w.Refused == nil {
		return ""
	}
	switch w.Refused.Instead {
	case "minimal":
		return i18n.T("Windows 没有接受「不走代理」的列表（ProxyOverride），已改用只含本机和局域网的列表", "Windows refused the bypass list (ProxyOverride); one with only this machine and local networks is in use")
	case "kept":
		return i18n.T("Windows 没有接受「不走代理」的列表（ProxyOverride），系统里原有的列表没有改动", "Windows refused the bypass list (ProxyOverride); the one the system had is unchanged")
	}
	return ""
}

// Line is w for the log: the refused value whole, as a report needs it.
func (w Written) Line() string {
	s := fmt.Sprintf("bypass written %q", w.Bypass)
	if r := w.Refused; r != nil {
		s += fmt.Sprintf("; refused %s %q (%s)", r.Value, r.Content, r.Error)
		if r.Instead != "" {
			s += ", " + r.Instead + " list instead"
		}
	}
	for _, d := range w.Dropped {
		if d.Why != "ipv6" {
			s += fmt.Sprintf("; dropped %q (%s)", d.Entry, d.Why)
		}
	}
	return s
}

var last struct {
	sync.Mutex
	w Written
}

// LastWrite is what the last Enable wrote, and what the OS refused of it.
func LastWrite() Written {
	last.Lock()
	defer last.Unlock()
	return last.w
}

func setLastWrite(w Written) {
	last.Lock()
	last.w = w
	last.Unlock()
}
