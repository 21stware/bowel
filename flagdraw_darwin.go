package main

// Whether the system's emoji font really draws a flag. Apple Color Emoji
// has a glyph for every flag but on some Macs draws one of them (🇹🇼) as a
// crossed-out box, so having the glyph proves nothing: the flag is drawn
// once, small, through Core Text (purego, no cgo), and a picture with no
// colour in it is no flag.

import (
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var (
	flagDrawOnce   sync.Once
	flagDrawOK     bool
	flagDrawnCache sync.Map // flag → bool

	cgColorSpaceCreateDeviceRGB func() uintptr
	cgBitmapContextCreate       func(data unsafe.Pointer, w, h, bpc, bpr uintptr, cs uintptr, info uint32) uintptr
	cgContextSetTextPosition    func(ctx uintptr, x, y float64)
	ctFontCreateWithName        func(name uintptr, size float64, matrix uintptr) uintptr
	ctLineCreateWithAttrString  func(s uintptr) uintptr
	ctLineDraw                  func(line, ctx uintptr)
	cfRelease                   func(ref uintptr)
	ctFontAttributeName         uintptr // kCTFontAttributeName
)

func flagDrawLoad() bool {
	open := func(p string) uintptr {
		h, err := purego.Dlopen(p, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return 0
		}
		return h
	}
	cg := open("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
	ct := open("/System/Library/Frameworks/CoreText.framework/CoreText")
	cf := open("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation")
	if open("/System/Library/Frameworks/Foundation.framework/Foundation") == 0 || cg == 0 || ct == 0 || cf == 0 {
		return false
	}
	sym, err := purego.Dlsym(ct, "kCTFontAttributeName")
	if err != nil {
		return false
	}
	ctFontAttributeName = *(*uintptr)(unsafe.Add(nil, sym)) // C memory
	purego.RegisterLibFunc(&cgColorSpaceCreateDeviceRGB, cg, "CGColorSpaceCreateDeviceRGB")
	purego.RegisterLibFunc(&cgBitmapContextCreate, cg, "CGBitmapContextCreate")
	purego.RegisterLibFunc(&cgContextSetTextPosition, cg, "CGContextSetTextPosition")
	purego.RegisterLibFunc(&ctFontCreateWithName, ct, "CTFontCreateWithName")
	purego.RegisterLibFunc(&ctLineCreateWithAttrString, ct, "CTLineCreateWithAttributedString")
	purego.RegisterLibFunc(&ctLineDraw, ct, "CTLineDraw")
	purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
	return true
}

// flagDrawn reports whether the emoji font draws flag as a flag. It says
// yes when it cannot tell.
func flagDrawn(flag string) bool {
	if v, ok := flagDrawnCache.Load(flag); ok {
		return v.(bool)
	}
	flagDrawOnce.Do(func() { flagDrawOK = flagDrawLoad() })
	ok := !flagDrawOK || flagColoured(flag)
	flagDrawnCache.Store(flag, ok)
	return ok
}

func flagColoured(flag string) bool {
	const w, h = 48, 40
	buf := make([]byte, w*h*4)
	cs := cgColorSpaceCreateDeviceRGB()
	defer cfRelease(cs)
	ctx := cgBitmapContextCreate(unsafe.Pointer(&buf[0]), w, h, 8, w*4, cs, 1) // kCGImageAlphaPremultipliedLast
	if ctx == 0 {
		return true
	}
	defer cfRelease(ctx)

	cls := func(s string) objc.ID { return objc.ID(objc.GetClass(s)) }
	pool := cls("NSAutoreleasePool").Send(objc.RegisterName("new"))
	defer pool.Send(objc.RegisterName("drain"))
	name := cls("NSString").Send(objc.RegisterName("stringWithUTF8String:"), "AppleColorEmoji")
	font := ctFontCreateWithName(uintptr(name), 32, 0)
	if font == 0 {
		return true
	}
	defer cfRelease(font)
	text := cls("NSString").Send(objc.RegisterName("stringWithUTF8String:"), flag)
	attrs := cls("NSDictionary").Send(objc.RegisterName("dictionaryWithObject:forKey:"), objc.ID(font), objc.ID(ctFontAttributeName))
	str := cls("NSAttributedString").Send(objc.RegisterName("alloc")).Send(objc.RegisterName("initWithString:attributes:"), text, attrs)
	defer str.Send(objc.RegisterName("release"))
	line := ctLineCreateWithAttrString(uintptr(str))
	if line == 0 {
		return true
	}
	defer cfRelease(line)
	cgContextSetTextPosition(ctx, 4, 8)
	ctLineDraw(line, ctx)
	runtime.KeepAlive(buf)

	// A flag has colour somewhere; the crossed box is black on nothing.
	var inked, coloured int
	for i := 0; i < len(buf); i += 4 {
		r, g, b, a := int(buf[i]), int(buf[i+1]), int(buf[i+2]), int(buf[i+3])
		if a < 64 {
			continue
		}
		inked++
		if max(r, g, b)-min(r, g, b) > 48 {
			coloured++
		}
	}
	return inked == 0 || coloured*20 > inked
}
