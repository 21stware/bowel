package main

import (
	"errors"
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/uwploop"
)

// uwpRow is the System proxy option that lets Windows' packaged apps
// reach it (issue #320): how many may, and the buttons that allow them
// all or undo what bowel allowed. The count is read when the page is
// first drawn and after each change, not at each frame.
func (u *UI) uwpRow(c *ui.Context, p *pal) {
	if !uwploop.Supported {
		return
	}
	if u.set.uwp == nil && !u.set.uwpReading && u.app != nil {
		u.uwpReload()
	}
	st := u.set.uwp
	optRow(c, p, uwpTitle(), uwpDesc(st), func() {
		if st == nil || !st.All() {
			u.uwpAction(c, p, "uwp-allow", i18n.T("全部允许", "Allow all"),
				i18n.T("UWP 应用现在可以使用代理了", "UWP apps can now use the proxy"), func() error { return u.app.uwpAllow() })
		}
		if st != nil && st.Added > 0 {
			u.uwpAction(c, p, "uwp-undo", i18n.T("撤销", "Undo"),
				i18n.T("已撤销 bowel 允许的 UWP 应用", "Undid the UWP apps bowel allowed"), func() error { return u.app.uwpUndo() })
		}
	})
}

func uwpTitle() string { return i18n.T("允许 UWP 应用使用代理", "Let UWP apps use the proxy") }

// uwpDesc says what the option is for, and how far it is done.
func uwpDesc(st *uwploop.Status) string {
	why := i18n.T("微软商店、邮件、Xbox 等 UWP 应用默认连不上本机的代理；允许时 Windows 可能要管理员授权", "The Store, Mail, Xbox and other UWP apps can't reach a proxy on this computer by default; allowing them may take an administrator")
	switch {
	case st == nil || st.Total == 0:
		return why
	case st.All():
		return fmt.Sprintf(i18n.T("这台电脑上的 %d 个 UWP 应用都可以使用代理", "All %d UWP apps on this computer can use the proxy"), st.Total)
	}
	return why + fmt.Sprintf(i18n.T("（已允许 %d / %d 个）", " (%d of %d allowed)"), st.Exempt, st.Total)
}

// uwpReload reads the count again, in the background.
func (u *UI) uwpReload() {
	u.set.uwpReading = true
	go func() {
		st, err := u.app.uwpStatus()
		u.settingsUpdate(func() {
			u.set.uwpReading = false
			if err != nil {
				u.set.toasts = append(u.set.toasts, err.Error())
				st = uwploop.Status{}
			}
			u.set.uwp = &st
		})
	}()
}

// uwpAction is a button as optAction's, for a change the user may cancel
// in Windows' prompt: that says nothing, and after either the count is
// read again.
func (u *UI) uwpAction(c *ui.Context, p *pal, key, label, done string, run func() error) {
	if u.set.busy[key] {
		label += "…"
	}
	btn(c.Key(key), p, label).OnClick(func() {
		if u.app == nil || u.set.busy[key] {
			return
		}
		if u.set.busy == nil {
			u.set.busy = map[string]bool{}
		}
		u.set.busy[key] = true
		go func() {
			err := run()
			u.settingsUpdate(func() {
				delete(u.set.busy, key)
				switch {
				case errors.Is(err, uwploop.ErrCanceled):
				case err != nil:
					u.set.toasts = append(u.set.toasts, err.Error())
				default:
					u.set.toasts = append(u.set.toasts, done)
				}
				u.uwpReload()
			})
		}()
	})
}
