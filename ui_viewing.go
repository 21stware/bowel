package main

import (
	"slices"

	"github.com/oboo/moho/internal/speed"
)

// watch starts the list of what a frame of window win ("window" or "panel")
// draws that shows a test's result, and returns what ends it. The app is
// told when the list is not the one it has, and keeps tested what the
// window shows.
func (u *UI) watch(win string) func() {
	// A list builds some rows after the view returned, while it is laid
	// out: they are in the last frame's list by now.
	u.tell(win)
	if u.drawn == nil {
		u.drawn = map[string]*viewing{}
	}
	u.seeing = &viewing{}
	u.drawn[win] = u.seeing
	return func() { u.tell(win) }
}

// tell tells the app what win's last frame drew, if it does not know.
func (u *UI) tell(win string) {
	cur, last := u.drawn[win], u.seen[win]
	if cur == nil || slices.Equal(last.tests, cur.tests) && slices.Equal(last.pings, cur.pings) {
		return
	}
	if u.seen == nil {
		u.seen = map[string]viewing{}
	}
	u.seen[win] = *cur
	if u.app != nil {
		u.app.setViewing(win, *cur)
	}
}

// see notes that the frame shows node's delay on url.
func (u *UI) see(node, url string) {
	if u.seeing != nil && node != "" && url != "" {
		u.seeing.tests = append(u.seeing.tests, speed.Target{Node: node, URL: url})
	}
}

// seePing notes that the frame shows node's ping.
func (u *UI) seePing(node string) {
	if u.seeing != nil && node != "" {
		u.seeing.pings = append(u.seeing.pings, node)
	}
}
