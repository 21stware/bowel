//go:build darwin

package main

// The rates' characters on macOS: the system font, SF Pro, as the menu
// bar's own text is, with its digits at one width, drawn through Core Text
// (purego, no cgo) as the flags are (flagdraw_darwin.go).

import (
	"image"
	"image/color"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego/objc"
)

// ctRateFont is the system font at size px and weight (NSFontWeight).
type ctRateFont struct{ size, weight float64 }

// systemRateFont is the system font, or nil where Core Text cannot be had.
func systemRateFont() rateFont {
	flagDrawOnce.Do(func() { flagDrawOK = flagDrawLoad() })
	if !flagDrawOK {
		return nil
	}
	return ctRateFont{trayRateSize, trayRateWeight}
}

// lines runs fn with the font and a Core Text line of each string, made in
// an autorelease pool. It says false if there is no such font.
func (f ctRateFont) lines(ss []string, fn func(font uintptr, lines []uintptr)) bool {
	// An autorelease pool belongs to the thread that pushed it: the
	// goroutine stays on that thread until the pool is drained.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	cls := func(s string) objc.ID { return objc.ID(objc.GetClass(s)) }
	sel := objc.RegisterName
	pool := cls("NSAutoreleasePool").Send(sel("new"))
	defer pool.Send(sel("drain"))
	font := cls("NSFont").Send(sel("monospacedDigitSystemFontOfSize:weight:"), f.size, f.weight)
	if font == 0 {
		return false
	}
	attrs := cls("NSDictionary").Send(sel("dictionaryWithObject:forKey:"), font, objc.ID(ctFontAttributeName))
	ls := make([]uintptr, len(ss))
	for i, s := range ss {
		text := cls("NSString").Send(sel("stringWithUTF8String:"), s)
		str := cls("NSAttributedString").Send(sel("alloc")).Send(sel("initWithString:attributes:"), text, attrs)
		ls[i] = ctLineCreateWithAttrString(uintptr(str))
		str.Send(sel("release"))
	}
	defer func() {
		for _, l := range ls {
			if l != 0 {
				cfRelease(l)
			}
		}
	}()
	fn(uintptr(font), ls)
	return true
}

func (f ctRateFont) widths(ss []string) (w []float64, capH float64) {
	w = make([]float64, len(ss))
	f.lines(ss, func(font uintptr, ls []uintptr) {
		capH = ctFontGetCapHeight(font)
		for i, l := range ls {
			if l != 0 {
				w[i] = ctLineGetTypographicBounds(l, nil, nil, nil)
			}
		}
	})
	return w, capH
}

func (f ctRateFont) draw(dst *image.Alpha, runs []rateRun) {
	b := dst.Bounds()
	w, h := b.Dx(), b.Dy()
	buf := make([]byte, w*h)
	ctx := cgBitmapContextCreate(unsafe.Pointer(&buf[0]), uintptr(w), uintptr(h), 8, uintptr(w), 0, 7) // kCGImageAlphaOnly
	if ctx == 0 {
		return
	}
	defer cfRelease(ctx)
	ss := make([]string, len(runs))
	for i, r := range runs {
		ss[i] = r.s
	}
	f.lines(ss, func(_ uintptr, ls []uintptr) {
		for i, l := range ls {
			if l != 0 {
				// Core Graphics counts y up from the bottom.
				cgContextSetTextPosition(ctx, runs[i].x, float64(h)-runs[i].y)
				ctLineDraw(l, ctx)
			}
		}
	})
	runtime.KeepAlive(buf)
	// Its rows run from the top, as the image's do; what dst has already
	// (the arrows) stays.
	for y := range h {
		for x := range w {
			if a := buf[y*w+x]; a > dst.AlphaAt(b.Min.X+x, b.Min.Y+y).A {
				dst.SetAlpha(b.Min.X+x, b.Min.Y+y, color.Alpha{A: a})
			}
		}
	}
}
