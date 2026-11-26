package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/settings"
)

// escape presses Escape and lets the frame after it settle.
func escape(tt *ui.Tester) {
	tt.Key(0, ui.KeyEscape)
	tt.Advance(time.Second)
}

// TestEscapePutsTheWindowAway: in the main window Escape steps back first,
// and only with nothing left to step back from hides the window; a held
// Escape's repeats never do.
func TestEscapePutsTheWindowAway(t *testing.T) {
	u := fixtureUI()
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Move(1040, 740)
	tt.Advance(time.Second)
	u.home.x, u.home.sub = "routes", "ai"
	tt.Frame()
	tt.Advance(time.Second)

	escape(tt)
	if u.home.x != "routes" || u.home.sub != "" || u.closes != 0 {
		t.Fatalf("first Escape: x %q sub %q closes %d, want the route list", u.home.x, u.home.sub, u.closes)
	}
	escape(tt)
	if u.home.x != "" || u.closes != 0 {
		t.Fatalf("second Escape: x %q closes %d, want the whole board", u.home.x, u.closes)
	}
	tt.KeyRepeat(0, ui.KeyEscape)
	tt.Advance(time.Second)
	if u.closes != 0 {
		t.Fatalf("a held Escape's repeat hid the window")
	}
	escape(tt)
	if u.closes != 1 {
		t.Fatalf("Escape on the whole board: closes %d, want 1", u.closes)
	}
}

// TestEscapeInTheLog: an open connection closes first; a text field keeps
// Escape while it has the focus (a search field clears on it), and the
// window has it again once the focus leaves.
func TestEscapeInTheLog(t *testing.T) {
	u := fixtureUI()
	u.page = "activity"
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(time.Second)
	u.act.open = "x"
	tt.Frame()
	escape(tt)
	if u.act.open != "" || u.closes != 0 {
		t.Fatalf("Escape with a connection open: open %q closes %d", u.act.open, u.closes)
	}

	if err := tt.Click("搜索连接"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	escape(tt)
	if u.closes != 0 {
		t.Fatalf("Escape in an empty search field hid the window")
	}
	tt.Type("qq")
	tt.Advance(time.Second)
	escape(tt)
	if u.act.q != "" || u.closes != 0 {
		t.Fatalf("Escape in the search field: q %q closes %d, want it cleared", u.act.q, u.closes)
	}
	escape(tt)
	if u.closes != 0 {
		t.Fatalf("Escape in the cleared search field hid the window")
	}
	tt.ClickAt(1040, 740) // off the field
	tt.Advance(time.Second)
	escape(tt)
	if u.closes != 1 {
		t.Fatalf("Escape after the search: closes %d, want 1", u.closes)
	}
}

// TestEscapeLeavesSelectsAndSetup: open options take Escape, and the
// first setup never hides on it.
func TestEscapeLeavesSelectsAndSetup(t *testing.T) {
	u := providersUIFixture()
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(time.Second)
	if err := tt.Click("每 12 小时"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("每 24 小时") {
		t.Fatalf("auto-update options did not open: %q", tt.Texts())
	}
	escape(tt)
	if tt.HasText("每 24 小时") || u.closes != 0 {
		t.Fatalf("Escape with options open: closes %d, options still open %v", u.closes, tt.HasText("每 24 小时"))
	}

	u = setupFixture()
	tt = ui.NewTester(u.view, 1060, 760)
	tt.Advance(time.Second)
	escape(tt)
	if u.closes != 0 {
		t.Fatalf("Escape in the first setup hid the window")
	}
}

// TestEscapeClosesSettings: Escape clears a search, then closes, but not
// from a text field, whose edit it would lose.
func TestEscapeClosesSettings(t *testing.T) {
	u := fixtureUI()
	u.snap.Settings = settings.Defaults.Clone()
	tt := ui.NewTesterWith(u.settingsView, 760, 900, ui.TesterOptions{})
	tt.Advance(time.Second)

	tt.Key(ui.Cmd, ui.KeyF)
	tt.Advance(time.Second)
	escape(tt)
	if u.closes != 0 {
		t.Fatalf("Escape in the empty search field closed settings")
	}

	u.set.query = "端口"
	tt.Frame()
	tt.ClickAt(5, 890) // off the field: the focus leaves it
	tt.Advance(time.Second)
	if u.set.query != "端口" {
		t.Fatalf("the click off the field changed the search to %q", u.set.query)
	}
	escape(tt)
	if u.set.query != "" || u.closes != 0 {
		t.Fatalf("Escape with a search: query %q closes %d, want it cleared", u.set.query, u.closes)
	}
	escape(tt)
	if u.closes != 1 {
		t.Fatalf("Escape in settings: closes %d, want 1", u.closes)
	}
}

// TestEscapeClosesWhatsNew: What's new closes on Escape as on ⌘W.
func TestEscapeClosesWhatsNew(t *testing.T) {
	u := fixtureUI()
	tt := ui.NewTesterWith(u.changelogView, 560, 700, ui.TesterOptions{})
	tt.Advance(time.Second)
	escape(tt)
	if u.closes != 1 {
		t.Fatalf("Escape in What's new: closes %d, want 1", u.closes)
	}
}
