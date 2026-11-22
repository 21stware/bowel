package main

// The rates on two lines (Settings.TraySpeedTwoLines, #188). A menu bar
// item's title is one line of the menu bar's font, too tall for two, so
// the two lines are drawn into the icon, beside the logo, in a smaller
// size of the system's sans-serif font (tray_rates_darwin.go): every rate
// is as wide, and the item never changes width.

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

const (
	// trayRateSize is the font's size in the icon's pixels, which are half
	// a point: the icon is 36 px for the menu bar's 18 pt.
	trayRateSize = 17
	// trayRateWeight is the system font's weight, as NSFont has it:
	// semibold. At this size the regular weight is a hairline beside the
	// logo's stroke and the arrows.
	trayRateWeight = 0.3 // NSFontWeightSemibold
	// trayRateGap is the room between the logo and the rates, in pixels.
	trayRateGap = 6
	// trayArrowBox is the arrow's width, and trayArrowGap the room after it.
	trayArrowBox = 8
	trayArrowGap = 3
)

// rateUnits are the units fmtRateShort ends a rate with.
const rateUnits = "BKMGT"

// A rateRun is text to draw, the left end of its baseline at x, y: the
// icon's pixels, from its top left.
type rateRun struct {
	s    string
	x, y float64
}

// A rateFont draws the rates' characters. widths gives each string's
// width as drawn, and the height of the digits; draw puts the runs in dst.
type rateFont interface {
	widths(ss []string) (w []float64, capH float64)
	draw(dst *image.Alpha, runs []rateRun)
}

// trayRateFont keeps the drawing of the rates to one at a time; goFace is
// the fallback's face, made once.
var trayRateFont = struct {
	sync.Mutex
	once   sync.Once
	goFace font.Face
}{}

// rateFace is the font the rates are drawn in: the system's
// (systemRateFont), or Go Medium where that cannot be had.
func rateFace() rateFont {
	if f := systemRateFont(); f != nil {
		return f
	}
	return goRateFace()
}

// goRateFace is Go Medium, or nil if it cannot be parsed.
func goRateFace() rateFont {
	trayRateFont.once.Do(func() {
		f, err := opentype.Parse(gomedium.TTF)
		if err != nil {
			return
		}
		trayRateFont.goFace, _ = opentype.NewFace(f, &opentype.FaceOptions{Size: trayRateSize, DPI: 72, Hinting: font.HintingFull})
	})
	if trayRateFont.goFace == nil {
		return nil
	}
	return goRateFont{trayRateFont.goFace}
}

// goRateFont draws in a face of golang.org/x/image.
type goRateFont struct{ face font.Face }

func (g goRateFont) widths(ss []string) ([]float64, float64) {
	w := make([]float64, len(ss))
	for i, s := range ss {
		w[i] = float64(font.MeasureString(g.face, s)) / 64
	}
	return w, float64(g.face.Metrics().CapHeight) / 64
}

func (g goRateFont) draw(dst *image.Alpha, runs []rateRun) {
	d := font.Drawer{Dst: dst, Src: image.Opaque, Face: g.face}
	for _, r := range runs {
		d.Dot = fixed.Point26_6{X: fixed.Int26_6(math.Round(r.x * 64)), Y: fixed.Int26_6(math.Round(r.y * 64))}
		d.DrawString(r.s)
	}
}

// rateParts parses a line of trayLines: its arrow, then a rate padded with
// figure spaces, its number and its unit.
func rateParts(l string) (up bool, num, unit string) {
	up = strings.HasPrefix(l, "↑")
	l = strings.TrimLeft(strings.TrimLeft(strings.TrimLeft(l, "↑"), "↓"), "  ")
	if l == "" {
		return up, "", ""
	}
	return up, l[:len(l)-1], l[len(l)-1:]
}

// trayRatesIcon is trayIcon with lines (trayLines) drawn on its right, one
// over the other, each centered on its half of the icon's height.
func trayRatesIcon(on, white bool, lines [2]string) []byte {
	trayRateFont.Lock()
	defer trayRateFont.Unlock()
	f := rateFace()
	if f == nil {
		return trayIcon(on, white)
	}
	return drawRatesIcon(f, on, white, lines)
}

// drawRatesIcon draws trayRatesIcon in f. Each line is an arrow in a box of
// its own (drawArrow), the number right-aligned in a box three digits wide,
// and the unit in a box as wide as the widest unit: the image is as wide
// for every pair of rates (#175, #186), in a font whose letters and dot
// are not all as wide as a digit, and the two lines' units stand one over
// the other.
func drawRatesIcon(f rateFont, on, white bool, lines [2]string) []byte {
	const n = 36
	// Every digit and unit is measured, for the boxes; then each line's
	// number and unit, from index glyphs on.
	const glyphs = 10 + len(rateUnits)
	ss := make([]string, 0, glyphs+4)
	for d := '0'; d <= '9'; d++ {
		ss = append(ss, string(d))
	}
	for _, u := range rateUnits {
		ss = append(ss, string(u))
	}
	var up [2]bool
	for i, l := range lines {
		var num, unit string
		up[i], num, unit = rateParts(l)
		ss = append(ss, num, unit)
	}
	w, capH := f.widths(ss)
	var digit, unit float64
	for _, v := range w[:10] {
		digit = max(digit, v)
	}
	for _, v := range w[10:glyphs] {
		unit = max(unit, v)
	}
	x0 := float64(n + trayRateGap)
	numEnd := math.Ceil(x0 + trayArrowBox + trayArrowGap + 3*digit)
	width := int(math.Ceil(numEnd + unit))

	text := image.NewAlpha(image.Rect(0, 0, width, n))
	var runs []rateRun
	for i := range lines {
		// The digits' middle on the middle of the line's half.
		y := math.Round(float64(i*n/2) + (n/2+capH)/2)
		drawArrow(text, float32(x0)+trayArrowBox/2, float32(y), float32(capH), up[i])
		k := glyphs + 2*i
		runs = append(runs, rateRun{ss[k], numEnd - w[k], y}, rateRun{ss[k+1], numEnd, y})
	}
	f.draw(text, runs)

	cov := make([]float64, width*n)
	const scale = 34.0 / (170 + logoWidth) // as in trayIcon
	const o = 18 - 100*scale
	strokeLogo(logoPoints, cov, width, n, scale, o, o, logoWidth)
	ink := 1.0
	if !on {
		ink = .4
	}
	var rgb uint8
	if white {
		rgb = 255
	}
	img := image.NewNRGBA(image.Rect(0, 0, width, n))
	for y := 0; y < n; y++ {
		for x := 0; x < width; x++ {
			c := math.Max(math.Min(1, cov[y*width+x])*ink, float64(text.AlphaAt(x, y).A)/255)
			if c > 0 {
				img.SetNRGBA(x, y, color.NRGBA{R: rgb, G: rgb, B: rgb, A: uint8(c * 255)})
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// drawArrow draws an arrow centered on cx, standing on baseline y and as
// tall as the digits (capH): a stem and a 45° head, one outline. It is
// drawn rather than taken from the font, whose arrows are thinner than
// its digits and sit off their middle; the stem is 2 px, a point, and
// stands on whole pixels, so it stays sharp.
func drawArrow(dst *image.Alpha, cx, y, capH float32, up bool) {
	const s, a = 2, 3.8 // the stroke, and how far the head reaches out and down
	b := dst.Bounds()
	z := vector.NewRasterizer(b.Dx(), b.Dy())
	// pt places a point given as for an up arrow, d down from its tip, and
	// flips it for a down one.
	pt := func(x, d float32) (float32, float32) {
		if up {
			return x, y - capH + d
		}
		return x, y - d
	}
	r2 := float32(math.Sqrt2)
	pts := [][2]float32{{cx, 0}, {cx + a, a}, {cx + a - s/r2, a + s/r2},
		{cx + s/2, s*r2 + s/2}, {cx + s/2, capH}, {cx - s/2, capH},
		{cx - s/2, s*r2 + s/2}, {cx - a + s/r2, a + s/r2}, {cx - a, a}}
	z.MoveTo(pt(pts[0][0], pts[0][1]))
	for _, p := range pts[1:] {
		z.LineTo(pt(p[0], p[1]))
	}
	z.ClosePath()
	z.Draw(dst, b, image.Opaque, image.Point{})
}
