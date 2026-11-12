package main

// The first run: a window opened with no node sources shows this instead of
// the tabs, in two steps. 添加节点 takes what the Provider page's add bar
// takes; once a source is there, 接管流量 offers the two ways of taking over.
// Taking over, or skipping a step, leaves for the board. It is decided once,
// as the window opens (openWindow): deleting the last source later stays on
// the Provider page.
//
// The right of the window is the app's icon at the size of the window: the
// theme color, and the logo's line across it. It draws itself in, carries a
// band of light while a source is added, and then wears the flags of where
// the nodes are.

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/settings"
)

type setupUI struct {
	on bool // the window shows it

	err    string // why adding failed
	errFor string // the text it failed on: edited, the error goes

	drawing bool      // the line has begun to draw itself
	card    ui.Handle // the paste card, whose border follows the focus
}

var setupLeave = ui.ElementTransition{Animation: &motion, Exit: &ui.Motion{Opacity: 0}}

// A step arrives from a little below, as the next page of the same thing.
var setupStep = ui.ElementTransition{Animation: &motion,
	Enter: &ui.Motion{Opacity: 0, Y: 14}, Exit: &ui.Motion{Opacity: 0, Y: -8}}

// setupTick ends the first run when traffic is taken over: there is nothing
// left to set up.
func (u *UI) setupTick() {
	if u.setup.on && len(u.snap.Providers) > 0 && u.snap.Takeover.On {
		u.setup.on = false
		u.page = "home"
	}
}

func (u *UI) setupLeave() {
	u.setup.on = false
	u.page = "home"
}

func (u *UI) setupPage(c *ui.Context, p *pal) {
	u.provCameraTick()
	ready := len(u.snap.Providers) > 0
	if !ready {
		u.provPaste(c)
	}
	fill, line := setupColors(p)
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		ui.Box(c.Key("steps")).Grow(1).Basis(0).MinWidth(0).Children(func() {
			step := ui.Column(c.Key(ready)).Absolute().Fill().Padding(64, 56, 28, 64).Transition(setupStep)
			step.Children(func() {
				ui.Column(c).Grow(1).Basis(0).MinHeight(0).Justify(ui.Center).Children(func() {
					if ready {
						u.setupReady(c, p)
					} else {
						u.setupAdd(c, p)
					}
				})
				u.setupFoot(c, p, ready)
			})
		})
		u.setupArt(c, p, fill, line, ready)
	})
	u.setupBar(c, p, line)
	u.provCameraDialog(c, p)
}

// setupColors are the icon's: the theme color as chosen, not the darker one
// that reads as a line on paper, and whichever of near-black and white shows
// on it. With no theme color, ink and paper.
func setupColors(p *pal) (fill, line ui.Color) {
	if p.accentOf == "ink" || !settings.ValidAccent(p.accentOf) {
		return p.ink, p.paper
	}
	fill, line = ui.Hex(p.accentOf), ui.Hex("#171615")
	if white := ui.Hex("#ffffff"); contrast(white, fill) > contrast(line, fill) {
		line = white
	}
	return fill, line
}

// setupBar is the window's top edge, to drag it by, with 设置 over the art.
func (u *UI) setupBar(c *ui.Context, p *pal, line ui.Color) {
	bar := c.TitleBar()
	q := *p
	q.ink2, q.hair2 = line, line.Alpha(.12)
	ui.Row(c.Key("bar")).Absolute().Top(0).Left(0).Right(0).Height(max(bar.Height, 50)).
		Padding(0, bar.Right+16, 0, bar.Left+14).Justify(ui.End).AlignItems(ui.Center).DragWindow().
		Children(func() { u.settingsButton(c, &q) })
}

// setupTitle is a step's words: who is speaking, the headline, and a line.
func setupTitle(c *ui.Context, p *pal, kick, title, lede string) {
	kicker(c, p, kick)
	disp(ui.Text(c, title), p).FontSize(52).FontWeight(700).LetterSpacing(-1.1).TextColor(p.ink).
		Margin(12, 0, 0, 0).Balance().ContentTransition(morphText)
	ui.Text(c, lede).FontSize(15).TextColor(p.ink2).Margin(12, 0, 0, 0)
}

// ---- Step one: a source ----

func (u *UI) setupAdd(c *ui.Context, p *pal) {
	setupTitle(c, p, i18n.T("欢迎使用 bowel", "Welcome to bowel"),
		i18n.T("连接两端世界", "Connect two worlds"),
		i18n.T("添加一个节点来源，就可以开始使用。", "Add a source of nodes to get started."))
	u.setupCard(c, p)
	u.setupNote(c, p)
}

// setupCard is the add bar at the size of the page: the text, and under it
// the other ways in and 添加.
func (u *UI) setupCard(c *ui.Context, p *pal) {
	s := &u.setup
	card := ui.Column(c.Key("card")).Bind(&s.card).Margin(28, 0, 0, 0).Radius(p.r(14)).
		Background(p.card).Border(1, p.hair).Transition(ui.ElementTransition{Colors: true, Duration: 160 * time.Millisecond})
	if s.card.FocusWithin(c) {
		card.BorderColor(p.ink)
	}
	card.Children(func() {
		u.provField(c, p, func(in string) { u.provSubmit(in, u.setupFail) }).
			Lines(4, 7).Padding(15, 18, 6, 18).FontSize(14)
		ui.Row(c).Gap(8).Padding(4, 10, 10, 12).AlignItems(ui.Center).Children(func() {
			u.provScanBtn(c, p)
			u.provFileBtn(c, p, u.setupFail)
			ui.Spacer(c)
			u.provAddBtn(c, p, func() { u.provSubmit(u.prov.input, u.setupFail) })
		})
	})
}

// setupNote says what the text will be added as, or why it could not be. Its
// room is kept, so that the card stays put as the words come and go.
func (u *UI) setupNote(c *ui.Context, p *pal) {
	s := &u.setup
	ui.Column(c).Margin(10, 2, 0, 2).MinHeight(36).Children(func() {
		if s.err != "" && s.errFor == strings.TrimSpace(u.prov.input) {
			ui.Text(c, s.err).FontSize(12).TextColor(p.signal).MaxLines(2).Ellipsis("…").Tooltip(s.err)
			return
		}
		u.provDetectLine(c, p)
	})
}

// setupFail keeps what went wrong under the card: from any goroutine.
func (u *UI) setupFail(msg string) {
	if u.win == nil {
		return
	}
	u.win.Update(func() {
		u.setup.err, u.setup.errFor = msg, strings.TrimSpace(u.prov.input)
	})
}

// ---- Step two: taking over ----

func (u *UI) setupReady(c *ui.Context, p *pal) {
	pv := u.snap.Providers[0]
	n := pv.Nodes
	shown := 0
	for _, nd := range u.snap.Nodes {
		if nd.Provider == pv.ID {
			shown++
		}
	}
	n = max(n, shown)
	title := i18n.T("来源已添加", "Source added")
	if n > 0 {
		title = nodesCount(n) + i18n.T("已就绪", " ready")
	}
	setupTitle(c, p, i18n.T("来自 ", "From ")+pv.Name, title,
		i18n.T("选择 bowel 接管流量的方式，之后可以随时更改。", "Choose how bowel takes over traffic. You can change this anytime."))
	if pv.Error != "" {
		ui.Text(c, pv.Error).FontSize(12).TextColor(p.signal).MaxLines(2).Margin(10, 0, 0, 0)
	}
	// A complete config is asked about here too: someone who added one for
	// its routing would otherwise leave this page on bowel's.
	ui.Column(c.Key("routing")).Margin(10, 0, 0, 0).Children(func() { u.provRouting(c, p, pv) })
	ui.Column(c).Gap(10).Margin(28, 0, 0, 0).Children(func() { u.entryChoices(c, p) })
}

// ---- The foot: where you are, and the way out ----

func (u *UI) setupFoot(c *ui.Context, p *pal, ready bool) {
	word, skip := i18n.T("添加节点", "Add nodes"), i18n.T("稍后再添加", "Add one later")
	if ready {
		word, skip = i18n.T("接管流量", "Take over traffic"), i18n.T("暂不接管，先进入主界面", "Not now, open the main window")
	}
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		for i := range 2 {
			col := p.hair
			if i == 0 || ready {
				col = p.accentLine
			}
			ui.Box(c.Key(i)).Size(26, 3).Radius(p.r(2)).Background(col)
		}
		kicker(c, p, word).Margin(0, 0, 0, 4).NoWrap()
		ui.Spacer(c)
		b := ui.ButtonBase(c.Key("skip")).Padding(5, 6).Margin(-5, -6).Label(skip)
		col := p.mute
		if b.Hovered() {
			col = p.ink
		}
		b.OnClick(u.setupLeave).Children(func() {
			ui.Text(c, skip).FontSize(12.5).FontWeight(500).TextColor(col).NoWrap()
		})
	})
}

// ---- The art ----

// setupFlags are the pictures of the first few places the nodes are in.
func (u *UI) setupFlags() []*ui.SVG {
	const most = 7
	var out []*ui.SVG
	seen := map[string]bool{}
	for _, n := range u.snap.Nodes {
		cc := countryCode(nodeFlag(n))
		if cc == "" || seen[cc] {
			continue
		}
		seen[cc] = true
		if pic := flagSVG(cc); pic != nil {
			out = append(out, pic)
			if len(out) == most {
				break
			}
		}
	}
	return out
}

func (u *UI) setupArt(c *ui.Context, p *pal, fill, line ui.Color, ready bool) {
	s := &u.setup
	box := ui.Box(c.Key("art")).WidthPercent(44).Shrink(0).Background(fill).AccessHidden(true)
	if !s.drawing {
		box.Jump("draw", 0)
		s.drawing = true
	}
	drawIn := motion
	drawIn.Duration = 1500 * time.Millisecond
	k := box.AnimateTo("draw", 1, drawIn)
	beads := motion
	beads.Duration = 900 * time.Millisecond
	worn := box.AnimateTo("beads", map[bool]float32{false: 0, true: 1}[ready], beads)
	flow := float32(-1)
	if u.pending("add") {
		flow = box.Loop("flow", 1500*time.Millisecond, ui.Linear)
	}
	flags := u.setupFlags()
	box.Draw(func(pt *ui.Painter, r ui.Rect) { setupDraw(pt, r, line, k, flow, worn, flags) })
}

// setupDraw paints the logo's line in r, k of it drawn. While flow is not
// negative the line goes quiet and a band runs along it, at flow of its
// length. flags sit in the line, spread along it, worn of the way grown.
func setupDraw(pt *ui.Painter, r ui.Rect, line ui.Color, k, flow, worn float32, flags []*ui.SVG) {
	s := min(r.W/194, r.H/188) * .72
	ox, oy := r.X+(r.W-200*s)/2, r.Y+(r.H-200*s)/2
	trace := func() *ui.Path {
		var path ui.Path
		path.MoveTo(ox+float32(logoPoints[0])*s, oy+float32(logoPoints[1])*s)
		for i := 2; i+1 < len(logoPoints); i += 2 {
			path.LineTo(ox+float32(logoPoints[i])*s, oy+float32(logoPoints[i+1])*s)
		}
		return &path
	}
	width := logoWidth * s
	if flow >= 0 {
		pt.StrokePath(trace().Trim(0, k), width, line.Alpha(.28))
		pt.StrokePath(trace().Trim(max(0, flow*1.3-.3), min(k, flow*1.3)), width, line)
	} else {
		pt.StrokePath(trace().Trim(0, k), width, line)
	}
	// One after another, down the line.
	points := len(logoPoints) / 2
	for i, pic := range flags {
		g := max(0, min(1, worn*float32(len(flags))-float32(i)))
		if g <= 0 {
			break
		}
		at := (i + 1) * (points - 1) / (len(flags) + 1)
		x, y := ox+float32(logoPoints[2*at])*s, oy+float32(logoPoints[2*at+1])*s
		d := width * .82 * g
		pt.Image(pic, ui.Rect{X: x - d/2, Y: y - d/2, W: d, H: d}, ui.Contain)
	}
}
