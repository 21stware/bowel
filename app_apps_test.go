package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"runtime"
	"testing"
)

// TestAppChoices: the editor's applications are those running and those
// with connections, each once, with connections first; bowel itself is not
// one. On macOS each is its bundle, helpers and all; the system's own
// agents and bundles inside a framework are not offered, and of two of one
// name the installed one is shown. On Linux the system's helpers and the
// shells are left out; on Windows, for now, all but those with connections.
func TestAppChoices(t *testing.T) {
	var running, conns []string
	var want []appChoice
	own := ownApp
	t.Cleanup(func() { ownApp = own })
	switch runtime.GOOS {
	case "darwin":
		ownApp = "bowel.app"
		cc := "/Users/me/Library/Application Support/Claude/claude-code/2.1.0/claude.app/Contents/MacOS/claude"
		running = []string{
			cc,
			"/Applications/Claude.app/Contents/MacOS/Claude",
			"/Applications/Claude.app/Contents/Frameworks/Claude Helper.app/Contents/MacOS/Claude Helper",
			"/Applications/Zed.app/Contents/MacOS/zed",
			"/Applications/bowel.app/Contents/MacOS/bowel",
			"/System/Library/CoreServices/Dock.app/Contents/MacOS/Dock",
			"/opt/homebrew/Cellar/python@3.13/3.13.1/Frameworks/Python.framework/Versions/3.13/Resources/Python.app/Contents/MacOS/Python",
			"/usr/libexec/trustd",
			"/opt/homebrew/bin/node",
		}
		conns = []string{
			cc,
			"/Applications/Telegram.app/Contents/MacOS/Telegram",
			"/Applications/Claude.app/Contents/Frameworks/Claude Helper.app/Contents/MacOS/Claude Helper",
			"/Applications/Claude.app/Contents/Frameworks/Claude Helper.app/Contents/MacOS/Claude Helper",
			"/Applications/bowel.app/Contents/Resources/bin/core",
			"/usr/bin/curl",
		}
		want = []appChoice{
			{"Claude.app", "/Applications/Claude.app", 3},
			{"curl", "", 1},
			{"Telegram.app", "/Applications/Telegram.app", 1},
			{"Zed.app", "/Applications/Zed.app", 0},
		}
	case "windows":
		ownApp = "bowel.exe"
		running = []string{`C:\Program Files\Telegram Desktop\Telegram.exe`, `C:\Program Files\Google\Chrome\Application\chrome.exe`}
		conns = []string{`C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Users\me\AppData\Local\Programs\Microsoft VS Code\Code.exe`, `C:\Program Files\bowel\bowel.exe`, ""}
		want = []appChoice{{"chrome.exe", "", 1}, {"Code.exe", "", 1}}
	default:
		ownApp = "bowel"
		running = []string{
			"/usr/bin/telegram-desktop", "/usr/lib/firefox/firefox", "/usr/lib/firefox/firefox", "/usr/bin/bash", "/usr/bin/zsh",
			"/usr/lib/systemd/systemd", "/usr/libexec/xdg-desktop-portal", "/usr/bin/bowel",
		}
		conns = []string{"/usr/lib/firefox/firefox", "/usr/bin/curl", "/usr/bin/bowel"}
		want = []appChoice{{"curl", "", 1}, {"firefox", "", 1}, {"telegram-desktop", "", 0}}
	}
	if got := appChoices(running, conns); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("choices\n got %+v\nwant %+v", got, want)
	}
	if got := appChoices(nil, nil); len(got) != 0 {
		t.Errorf("nothing running: %+v", got)
	}
}

// TestIcnsPNG: of an icon file's PNGs the smallest at least as wide as
// asked is taken, else the largest; entries that are not PNGs, a wrong
// length and a file that is not an icon file give no picture.
func TestIcnsPNG(t *testing.T) {
	pngOf := func(w int) []byte {
		var b bytes.Buffer
		if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, w, w))); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	icns := func(entries ...[]byte) []byte {
		var body []byte
		for i, e := range entries {
			body = append(body, fmt.Sprintf("ic%02d", i)...)
			body = binary.BigEndian.AppendUint32(body, uint32(8+len(e)))
			body = append(body, e...)
		}
		head := binary.BigEndian.AppendUint32([]byte("icns"), uint32(8+len(body)))
		return append(head, body...)
	}
	raw := bytes.Repeat([]byte{7}, 40) // an old kind of entry: raw pixels
	file := icns(pngOf(16), raw, pngOf(256), pngOf(64), pngOf(32))
	for _, c := range []struct{ px, want int }{{36, 64}, {64, 64}, {65, 256}, {10, 16}, {512, 256}} {
		if got := pngWidth(icnsPNG(file, c.px)); got != c.want {
			t.Errorf("at least %d px: got a PNG %d wide, want %d", c.px, got, c.want)
		}
	}
	for name, b := range map[string][]byte{
		"no PNG":      icns(raw),
		"not an icon": append([]byte("abcd"), file[4:]...),
		"short":       []byte("icns"),
		"cut off":     file[:40],
		"bad length":  append(append([]byte{}, file[:12]...), 0, 0, 0, 3),
	} {
		if got := icnsPNG(b, 36); got != nil {
			t.Errorf("%s: %d bytes of picture", name, len(got))
		}
	}
}

// TestBundleIcon: on macOS an app's icon is read from its bundle, as named
// in its Info.plist. Finder is there on every Mac.
func TestBundleIcon(t *testing.T) {
	if runtime.GOOS != "darwin" {
		if bundleIcon("/Applications/X.app", 36) != nil {
			t.Error("an icon outside macOS")
		}
		t.Skip("bundles are macOS's")
	}
	if w := pngWidth(bundleIcon("/System/Library/CoreServices/Finder.app", 36)); w < 36 {
		t.Errorf("Finder's icon: a PNG %d wide", w)
	}
	if bundleIcon("/nowhere/X.app", 36) != nil {
		t.Error("an icon for a bundle that isn't there")
	}
}

// TestIconImage: an icon is kept scaled down to the size it is shown at;
// one smaller is kept as it is, and one larger than maxIcon, or not a PNG,
// is not decoded.
func TestIconImage(t *testing.T) {
	pngOf := func(w, h int) []byte {
		var b bytes.Buffer
		if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, w, h))); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	for _, c := range []struct{ w, h, wantW, wantH int }{{64, 64, 36, 36}, {1024, 1024, 36, 36}, {16, 16, 16, 16}, {72, 36, 36, 18}} {
		img := iconImage(pngOf(c.w, c.h), 36)
		if img == nil {
			t.Errorf("%dx%d: no picture", c.w, c.h)
			continue
		}
		if b := img.Bounds(); b.Dx() != c.wantW || b.Dy() != c.wantH {
			t.Errorf("%dx%d: kept at %dx%d, want %dx%d", c.w, c.h, b.Dx(), b.Dy(), c.wantW, c.wantH)
		}
	}
	for name, b := range map[string][]byte{"too wide": pngOf(maxIcon+1, 1), "too tall": pngOf(1, maxIcon+1), "not a PNG": []byte("icns"), "nothing": nil} {
		if iconImage(b, 36) != nil {
			t.Errorf("%s: decoded", name)
		}
	}
}
