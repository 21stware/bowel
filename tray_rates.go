package main

// The rates on two lines (Settings.TraySpeedTwoLines, #188). A menu bar
// item's title is one line of the menu bar's font, too tall for two, so
// the two lines are drawn into the icon, beside the logo, in a smaller
// monospaced font: every rate is as wide, and the item never changes width.

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"sync"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

const (
	// trayRateSize is the font's size in the icon's pixels, which are half
	// a point: the icon is 36 px for the menu bar's 18 pt.
	trayRateSize = 16
	// trayRateGap is the room between the logo and the rates, in pixels.
	trayRateGap = 6
	// trayArrowBox is the arrow's width, and trayArrowGap the room after it.
	trayArrowBox = 8
	trayArrowGap = 3
)

// trayRateFont is the face the rates are drawn in, and the lock that keeps
// it to one drawing at a time.
var trayRateFont = struct {
	once sync.Once
	sync.Mutex
	face font.Face
}{}

func rateFace() font.Face {
	trayRateFont.once.Do(func() {
		f, err := opentype.Parse(gomono.TTF)
		if err != nil {
			return
		}
		trayRateFont.face, _ = opentype.NewFace(f, &opentype.FaceOptions{Size: trayRateSize, DPI: 72, Hinting: font.HintingFull})
	})
	return trayRateFont.face
}

// trayRatesIcon is trayIcon with lines (trayLines) drawn on its right, one
// over the other, each centered on its half of the icon's height. The arrow
// has a box of its own (drawArrow), every other character takes one cell
// of the monospaced font, and a figure space is an empty one, so the image
// is as wide for every pair of rates (#175, #186).
func trayRatesIcon(on, white bool, lines [2]string) []byte {
	const n = 36
	trayRateFont.Lock()
	defer trayRateFont.Unlock()
	face := rateFace()
	if face == nil {
		return trayIcon(on, white)
	}
	cells := 0
	for _, l := range lines {
		cells = max(cells, utf8.RuneCountInString(l))
	}
	adv, _ := face.GlyphAdvance('0')
	// The arrow has a box of its own, and room after it; the rate's
	// characters take a cell each.
	w := n + trayRateGap + trayArrowBox + trayArrowGap + (adv * fixed.Int26_6(cells-1)).Ceil()

	text := image.NewAlpha(image.Rect(0, 0, w, n))
	d := font.Drawer{Dst: text, Src: image.Opaque, Face: face}
	capH := face.Metrics().CapHeight
	for i, l := range lines {
		// The digits' middle on the middle of the line's half.
		y := fixed.I((fixed.I(i*n/2) + (fixed.I(n/2)+capH)/2).Round())
		x := fixed.I(n + trayRateGap)
		for _, r := range l {
			if r == '↑' || r == '↓' {
				drawArrow(text, f26(x)+trayArrowBox/2, f26(y), f26(capH), r == '↑')
				x += fixed.I(trayArrowBox + trayArrowGap)
				continue
			}
			if r != ' ' {
				d.Dot = fixed.Point26_6{X: x, Y: y}
				d.DrawString(string(r))
			}
			x += adv
		}
	}

	cov := make([]float64, w*n)
	const scale = 34.0 / (170 + logoWidth) // as in trayIcon
	const o = 18 - 100*scale
	strokeLogo(logoPoints, cov, w, n, scale, o, o, logoWidth)
	ink := 1.0
	if !on {
		ink = .4
	}
	var rgb uint8
	if white {
		rgb = 255
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, n))
	for y := 0; y < n; y++ {
		for x := 0; x < w; x++ {
			c := math.Max(math.Min(1, cov[y*w+x])*ink, float64(text.AlphaAt(x, y).A)/255)
			if c > 0 {
				img.SetNRGBA(x, y, color.NRGBA{R: rgb, G: rgb, B: rgb, A: uint8(c * 255)})
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func f26(v fixed.Int26_6) float32 { return float32(v) / 64 }

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
