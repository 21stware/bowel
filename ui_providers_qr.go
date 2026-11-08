package main

// Provider's QR codes: 扫码 in the add bar reads one (the camera, the
// screen, a picture or the clipboard) into the add bar, where it shows
// what it will be added as before 添加; a provider's 二维码 shows its
// subscription link or node links as a code to scan, copy or save.

import (
	"context"
	"errors"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/provider"
	"github.com/oboo/moho/internal/qr"
)

// provCopyWord is a provider's copy button.
func provCopyWord(kind string) string {
	if kind == "link" {
		return i18n.T("复制节点链接", "Copy links")
	}
	return i18n.T("复制订阅链接", "Copy link")
}

// provCopyWhat is what the copy button copied, for its toast.
func provCopyWhat(kind string) string {
	if kind == "link" {
		return i18n.T("节点链接", "the node links")
	}
	return i18n.T("订阅链接", "the subscription link")
}

// provShareText is what shares a provider.
func (u *UI) provShareText(id string) (string, error) {
	switch {
	case u.prov.share != nil:
		return u.prov.share(id)
	case u.app != nil:
		return u.app.providerShare(id)
	}
	return "", errors.New(i18n.T("无法分享", "Cannot share"))
}

// ---- Reading a code ----

// provScanBtn is 扫码: the ways to read a code.
func (u *UI) provScanBtn(c *ui.Context, p *pal) {
	u.provBtn(c.Key("scan"), p, i18n.T("扫码", "Scan QR"), false).Height(36).PopoverMenuButton(func(m *ui.Menu) {
		if m.Item(i18n.T("用摄像头扫描", "Scan with the camera")).
			Subtitle(i18n.T("对准手机或纸上的二维码", "Hold up a code on a phone or paper")).Chosen() {
			u.provCameraOpen()
		}
		if m.Item(i18n.T("扫描屏幕上的二维码", "Scan the screen")).
			Subtitle(i18n.T("读取所有显示器上露出的二维码，读到的会框出来", "Reads codes in view on every display, and frames the one it read")).Chosen() && u.app != nil {
			u.provQRScreen()
		}
		m.Separator()
		if m.Item(i18n.T("从图片读取…", "From a picture…")).Chosen() && u.app != nil {
			u.provQR(u.app.qrImageFile)
		}
		if m.Item(i18n.T("从剪贴板里的图片读取", "From a picture on the clipboard")).Chosen() && u.app != nil {
			u.provQR(func(context.Context) (string, error) {
				s, ok, err := u.app.qrClipboard()
				if !ok && err == nil {
					err = errors.New(i18n.T("剪贴板里没有图片", "There is no picture on the clipboard"))
				}
				return s, err
			})
		}
	})
}

// provQR reads a code off the main thread and puts what it says in the
// add bar; "" (a canceled dialog) changes nothing.
func (u *UI) provQR(read func(context.Context) (string, error)) {
	u.do(func() error {
		s, err := read(bg())
		if err != nil || s == "" || u.win == nil {
			return err
		}
		u.win.Update(func() { u.provQRRead(s) })
		return nil
	})
}

// provQRScreen reads the codes on the screen and frames where they were,
// so that the user sees which code went into the add bar.
func (u *UI) provQRScreen() {
	u.do(func() error {
		s, spots, err := u.app.qrScreen(bg())
		if u.win == nil {
			return err
		}
		u.win.Update(func() {
			for _, sp := range spots {
				u.qrHintShow(sp, err == nil)
			}
			if err == nil && s != "" {
				u.provQRRead(s)
			}
		})
		return err
	})
}

func (u *UI) provQRRead(s string) {
	u.prov.input, u.prov.focus = s, true
	u.toasts = append(u.toasts, i18n.T("已读取二维码，确认后按添加", "QR code read: check it, then Add"))
}

// provCameraOpen opens the camera's dialog and the camera. Frames show
// as they come; every fifth of a second one is searched for a code, and
// the first that reads closes the dialog.
func (u *UI) provCameraOpen() {
	pr := &u.prov
	if u.app == nil || u.win == nil {
		return
	}
	pr.camGen++
	gen := pr.camGen
	pr.camOpen, pr.camErr, pr.camFrame, pr.camW = true, "", nil, 0
	go func() {
		cam, err := u.app.openCamera(bg())
		u.win.Update(func() {
			switch {
			case gen != pr.camGen || !pr.camOpen:
				if cam != nil {
					go cam.Close()
				}
			case err != nil:
				pr.camErr = err.Error()
			default:
				pr.cam = cam
			}
		})
		if err != nil {
			return
		}
		var searched time.Time
		for frame := range cam.Frames() {
			u.win.Update(func() {
				if gen != pr.camGen {
					return
				}
				if pr.camFrame == nil {
					pr.camFrame = ui.NewBitmap(frame)
				} else {
					pr.camFrame.Update(frame)
				}
				b := frame.Bounds()
				pr.camW = float32(b.Dx()) / float32(b.Dy())
			})
			if time.Since(searched) < 200*time.Millisecond {
				continue
			}
			searched = time.Now()
			texts, err := qr.Decode(frame)
			if err != nil {
				continue
			}
			s, err := provider.FromQR(texts)
			u.win.Update(func() {
				switch {
				case gen != pr.camGen:
				case err != nil:
					pr.camErr = err.Error() // a code, but not one of nodes: keep looking
				default:
					pr.camOpen = false
					u.provQRRead(s)
				}
			})
			if err == nil {
				return
			}
		}
	}()
}

// provCameraTick closes the camera once its dialog has closed.
func (u *UI) provCameraTick() {
	pr := &u.prov
	if pr.camOpen || pr.cam == nil {
		return
	}
	cam := pr.cam
	pr.cam, pr.camFrame = nil, nil
	pr.camGen++
	go cam.Close() // stopRunning waits for the camera
}

func (u *UI) provCameraDialog(c *ui.Context, p *pal) {
	pr := &u.prov
	ui.Modal(c.Key("camera"), &pr.camOpen, func() {
		ui.Column(c).Gap(12).Width(520).Children(func() {
			ui.Text(c, i18n.T("用摄像头扫描二维码", "Scan a QR code with the camera")).FontSize(18).FontWeight(700).TextColor(p.ink)
			ui.Box(c).Width(520).Height(292).Radius(p.r(10)).Background(ui.Hex("#000000")).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
				switch {
				case pr.camFrame != nil:
					// Mirrored, as a mirror shows you: the code moves the way
					// your hand does. It is read as the camera sees it.
					ui.Image(c, pr.camFrame).Absolute().Fill().Fit(ui.Cover).Scale(-1, 1)
					ui.Box(c).Size(180, 180).Radius(p.r(14)).Border(2, ui.RGBA(255, 255, 255, .85))
				case pr.camErr == "":
					ui.Text(c, i18n.T("正在打开摄像头…", "Opening the camera…")).FontSize(12.5).TextColor(ui.RGBA(255, 255, 255, .7))
				}
			})
			note, col := i18n.T("把二维码放进框里，读到后会填入添加栏", "Hold the code in the frame: once read, it goes into the add bar"), p.mute
			if pr.camErr != "" {
				note, col = pr.camErr, p.warn
			}
			ui.Text(c, note).FontSize(12).TextColor(col)
			ui.Row(c).Justify(ui.End).Children(func() {
				u.provBtn(c.Key("cam-cancel"), p, i18n.T("取消", "Cancel"), false).OnClick(func() { pr.camOpen = false })
			})
		})
	})
}

// ---- Where a code on the screen was ----

// qrHint is a display's frames around the codes read off it, shown for a
// moment over everything, the mouse going through.
type qrHint struct {
	spots qrSpots
	ok    bool // the codes were nodes: they went into the add bar
	shown bool
}

const (
	qrHintIn   = 280 * time.Millisecond
	qrHintStay = 1800 * time.Millisecond
	qrHintOut  = 320 * time.Millisecond
)

// qrHintShow frames the codes on one display: a panel over all apps, the
// size of the display, that fades in, stays and fades out. Main thread.
func (u *UI) qrHintShow(sp qrSpots, ok bool) {
	if u.app == nil {
		return
	}
	h := &qrHint{spots: sp, ok: ok}
	b := sp.Display
	var w *mygo.Window
	w = mygo.NewWindow(mygo.WindowOptions{
		Title: i18n.T("二维码位置", "Where the code is"), X: b.X, Y: b.Y, Width: b.Width, Height: b.Height,
		Frameless: true, Transparent: true, Panel: true, Hidden: true,
		DisableShadow: true, DisableResize: true, DisableMove: true, SkipTaskbar: true,
		Content: ui.View(func(c *ui.Context) { u.qrHintView(c, h) }),
	})
	w.SetBounds(b) // X and Y of zero would center it
	w.SetIgnoreMouseEvents(true)
	w.ShowInactive()
	// The first frame draws it faded out; from there it fades in.
	time.AfterFunc(30*time.Millisecond, func() { w.Update(func() { h.shown = true }) })
	time.AfterFunc(qrHintIn+qrHintStay, func() { w.Update(func() { h.shown = false }) })
	time.AfterFunc(qrHintIn+qrHintStay+qrHintOut, w.Close)
}

// qrHintView dims the display but for the code, and frames it with the
// accent; one word above it says what became of it.
func (u *UI) qrHintView(c *ui.Context, h *qrHint) {
	c.DisableAnimations()
	useLook(u.shownSettings())
	p := palette(c)
	c.Root().Background(ui.Transparent)
	a := c.Root().Animate("in", map[bool]float32{true: 1, false: 0}[h.shown], qrHintIn)
	edge, ink := p.accent, p.onAccent
	word := i18n.T("已读取这个二维码", "Read this code")
	if !h.ok {
		edge, ink, word = p.warn, ui.Hex("#ffffff"), i18n.T("读到了，但不是节点二维码", "Read, but not a code of nodes")
	}
	rs := h.spots.Rects
	ui.Box(c).Absolute().Fill().Opacity(a).Children(func() {
		for i, r := range rs {
			x, y, w, hh := float32(r.X), float32(r.Y), float32(r.Width), float32(r.Height)
			const pad = 10
			if len(rs) == 1 {
				// A shadow wide enough to cover the display dims all but the code.
				ui.Box(c).Key(i).Absolute().Left(x-pad).Top(y-pad).Size(w+2*pad, hh+2*pad).Radius(p.r(14)).
					Shadow(0, 0, 0, 6000, ui.RGBA(0, 0, 0, .32))
			}
			s := 1.18 - .18*a // closes in on the code as it shows
			ui.Box(c).Key(100+i).Absolute().Left(x-pad).Top(y-pad).Size(w+2*pad, hh+2*pad).Radius(p.r(14)).
				Border(3, edge).Shadow(0, 0, 22, 2, edge.Alpha(.55)).TransformOrigin(.5, .5).Scale(s, s)
			ty := y - pad - 38
			if ty < 8 {
				ty = y + hh + pad + 10 // no room above: below
			}
			ui.Row(c).Key(200+i).Absolute().Left(x-pad).Top(ty).Height(30).Padding(0, 13).Radius(p.r(15)).
				AlignItems(ui.Center).Gap(6).Background(edge).Shadow(0, 4, 14, 0, ui.RGBA(0, 0, 0, .25)).Children(func() {
				ui.Text(c, word).FontSize(13).FontWeight(600).TextColor(ink).MaxLines(1)
			})
		}
	})
}

// ---- Sharing a provider ----

// provShareOpen makes a provider's code and opens the share dialog.
func (u *UI) provShareOpen(id string) {
	pr := &u.prov
	pr.shareID, pr.shareOpen, pr.shareErr = id, true, ""
	pr.shareText, pr.shareCode, pr.shareBmp = "", nil, nil
	text, err := u.provShareText(id)
	if err != nil {
		pr.shareErr = err.Error()
		return
	}
	pr.shareText = text
	code, err := qr.Encode(text)
	if err != nil {
		pr.shareErr = err.Error()
		return
	}
	pr.shareCode, pr.shareBmp = code, ui.NewBitmap(code).Pixelated()
}

func (u *UI) provShareDialog(c *ui.Context, p *pal) {
	pr := &u.prov
	if !pr.shareOpen {
		pr.shareBmp, pr.shareCode, pr.shareText = nil, nil, "" // the link is a secret: keep it no longer
	}
	kind := ""
	for _, pv := range u.snap.Providers {
		if pv.ID == pr.shareID {
			kind = pv.Kind
		}
	}
	ui.Modal(c.Key("share"), &pr.shareOpen, func() {
		ui.Column(c).Gap(12).Width(340).AlignItems(ui.Center).Children(func() {
			ui.Text(c, u.providerName(pr.shareID)).FontSize(18).FontWeight(700).TextColor(p.ink).MaxLines(1)
			ui.Text(c, provKindWord(kind)).FontSize(11.5).TextColor(p.mute)
			if pr.shareBmp != nil {
				// Black on white in either theme, as scanners expect.
				ui.Box(c).Size(264, 264).Radius(p.r(10)).Background(ui.Hex("#ffffff")).Children(func() {
					ui.Image(c, pr.shareBmp).Fill().Label(i18n.T("二维码", "QR code"))
				})
				ui.Text(c, i18n.T("用 bowel、Clash Meta、Shadowrocket 等扫码添加", "Scan it with bowel, Clash Meta, Shadowrocket, …")).FontSize(12).TextColor(p.mute)
			}
			if pr.shareErr != "" {
				ui.Text(c, pr.shareErr).FontSize(12.5).TextColor(p.warn).Padding(20, 0)
			}
			if pr.shareText != "" {
				ui.Text(c, pr.shareText).Font("SF Mono, Menlo").FontSize(11).TextColor(p.ink2).MaxLines(3).Selectable().
					AlignSelf(ui.Stretch)
			}
			ui.Row(c).Gap(8).Margin(4, 0, 0, 0).AlignSelf(ui.Stretch).Children(func() {
				if pr.shareText != "" {
					u.provBtn(c.Key("share-copy"), p, provCopyWord(kind), false).OnClick(func() {
						c.WriteClipboard(pr.shareText)
						u.toasts = append(u.toasts, i18n.T("已复制", "Copied ")+provCopyWhat(kind))
					})
				}
				if code := pr.shareCode; code != nil {
					name := u.providerName(pr.shareID)
					u.provBtn(c.Key("share-save"), p, i18n.T("存储图片…", "Save image…"), false).OnClick(func() {
						if u.app != nil {
							u.do(func() error { return u.app.saveQR(bg(), name, code) })
						}
					})
				}
				ui.Box(c).Grow(1)
				u.provBtn(c.Key("share-done"), p, i18n.T("完成", "Done"), false).OnClick(func() { pr.shareOpen = false })
			})
		})
	})
}
