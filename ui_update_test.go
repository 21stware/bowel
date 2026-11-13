package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func updateFixture(status string) *UI {
	u := fixtureUI()
	u.snap.UpdateInfo.Can = true
	u.snap.Update = &UpdateView{Status: status, Version: "9.9.9", Notes: "- 第一条改动\n\n- 第二条改动 `code`",
		Date: time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)}
	return u
}

// TestUpdateNotesLink: 新版本 x.y.z opens what it changes, which the
// pointer is told first.
func TestUpdateNotesLink(t *testing.T) {
	u := updateFixture(updateAvailable)
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(time.Second)
	r, ok := tt.Find("新版本 9.9.9")
	if !ok {
		t.Fatalf("no notice: %q", tt.Texts())
	}
	if _, ok := tt.AccessByRole(ui.RoleButton, "新版本 9.9.9"); !ok {
		t.Error("the text is no button")
	}
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Advance(2 * time.Second)
	if !tt.HasText("查看 9.9.9 的更新内容") {
		t.Errorf("no hint on hover: %q", tt.Texts())
	}
	shot(t, tt, "update-available-hover")
	if err := tt.Click("新版本 9.9.9"); err != nil {
		t.Fatal(err)
	}
	if !u.clog.next {
		t.Error("the notes were not asked for")
	}
}

// TestUpdateNoNotesNoLink: a release without notes has nothing to open.
func TestUpdateNoNotesNoLink(t *testing.T) {
	u := updateFixture(updateAvailable)
	u.snap.Update.Notes = ""
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(time.Second)
	_ = tt.Click("新版本 9.9.9")
	if u.clog.next {
		t.Error("opened notes that are not there")
	}
	if _, ok := tt.AccessByRole(ui.RoleButton, "新版本 9.9.9"); ok {
		t.Error("the text is a button")
	}
}

// TestUpdateReadyLine: once installed, the title bar and the panel say so
// and offer the restart, which is not made for the user.
func TestUpdateReadyLine(t *testing.T) {
	u := updateFixture(updateReady)
	tt := ui.NewTester(u.view, 1060, 760)
	tt.Advance(time.Second)
	for _, s := range []string{"9.9.9 已就绪 · 重启以完成更新", "立即重启"} {
		if !tt.HasText(s) {
			t.Errorf("title bar lacks %q: %q", s, tt.Texts())
		}
	}
	r, _ := tt.Find("立即重启")
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Advance(2 * time.Second)
	if !hasTextPart(tt, "网络会中断几秒") {
		t.Errorf("no warning on hover: %q", tt.Texts())
	}
	shot(t, tt, "update-ready-titlebar")

	pt := ui.NewTester(u.panelView, panelWidth, 640)
	pt.Advance(time.Second)
	for _, s := range []string{"9.9.9 已就绪 · 重启以完成更新", "立即重启"} {
		if !pt.HasText(s) {
			t.Errorf("panel lacks %q: %q", s, pt.Texts())
		}
	}
	shot(t, pt, "update-ready-panel")
}

// TestUpdateRowReady: 关于 → 软件更新 says what a restart does, and links
// to the new version's notes.
func TestUpdateRowReady(t *testing.T) {
	for _, status := range []string{updateAvailable, updateReady} {
		u := updateFixture(status)
		u.set.section = "about"
		tt := ui.NewTesterWith(u.settingsView, 760, 900, ui.TesterOptions{})
		tt.Advance(time.Second)
		if !tt.HasText("9.9.9 更新内容") {
			t.Errorf("%s: no notes button: %q", status, tt.Texts())
		}
		want := "期间网络不受影响"
		if status == updateReady {
			want = "下次打开 bowel 时生效"
			if !tt.HasText("立即重启") {
				t.Errorf("no restart: %q", tt.Texts())
			}
		}
		if !hasTextPart(tt, want) {
			t.Errorf("%s: lacks %q: %q", status, want, tt.Texts())
		}
		shot(t, tt, "update-row-"+status)
		if err := tt.Click("9.9.9 更新内容"); err != nil {
			t.Fatal(err)
		}
		if !u.clog.next {
			t.Errorf("%s: the notes were not asked for", status)
		}
	}
}

// TestUpdateNotesWindow: the 更新内容 window shows the new version's notes
// alone, and the versions before after 查看全部版本.
func TestUpdateNotesWindow(t *testing.T) {
	u := updateFixture(updateReady)
	u.clog.next = true
	tt := ui.NewTester(u.changelogView, 560, 620)
	tt.Advance(time.Second)
	for _, s := range []string{"9.9.9", "已装好，待重启", "第一条改动", "第二条改动 `code`", "2026-11-20"} {
		if !tt.HasText(s) {
			t.Errorf("lacks %q: %q", s, tt.Texts())
		}
	}
	newest := parseChangelog(changelogText)[0].Version
	if tt.HasText(newest) {
		t.Errorf("shows the versions before too: %q", tt.Texts())
	}
	shot(t, tt, "update-notes-window")
	if err := tt.Click("查看全部版本"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(100 * time.Millisecond)
	if u.clog.next || !tt.HasText("9.9.9") || !tt.HasText(newest) {
		t.Errorf("all versions: %q", tt.Texts())
	}
}
