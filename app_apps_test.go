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
// with connections, each once, with connections first. On macOS each is its
// bundle, helpers and all, and the system's own agents are not offered.
func TestAppChoices(t *testing.T) {
	var running, conns []string
	var want []appChoice
	switch runtime.GOOS {
	case "darwin":
		running = []string{
			"/Applications/Claude.app/Contents/MacOS/Claude",
			"/Applications/Claude.app/Contents/Frameworks/Claude Helper.app/Contents/MacOS/Claude Helper",
			"/Applications/Zed.app/Contents/MacOS/zed",
			"/System/Library/CoreServices/Dock.app/Contents/MacOS/Dock",
			"/usr/libexec/trustd",
			"/opt/homebrew/bin/node",
		}
		conns = []string{
			"/Applications/Telegram.app/Contents/MacOS/Telegram",
			"/Applications/Claude.app/Contents/Frameworks/Claude Helper.app/Contents/MacOS/Claude Helper",
			"/Applications/Claude.app/Contents/Frameworks/Claude Helper.app/Contents/MacOS/Claude Helper",
			"/usr/bin/curl",
		}
		want = []appChoice{
			{"Claude.app", "/Applications/Claude.app", 2},
			{"curl", "", 1},
			{"Telegram.app", "/Applications/Telegram.app", 1},
			{"Zed.app", "/Applications/Zed.app", 0},
		}
	case "windows":
		running = []string{"Telegram.exe", "chrome.exe", "chrome.exe", "Code.exe"}
		conns = []string{`C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Users\me\AppData\Local\Programs\Microsoft VS Code\Code.exe`, ""}
		want = []appChoice{{"chrome.exe", "", 1}, {"Code.exe", "", 1}, {"Telegram.exe", "", 0}}
	default:
		running = []string{"/usr/bin/telegram-desktop", "/usr/lib/firefox/firefox", "/usr/lib/firefox/firefox", "/usr/bin/bash"}
		conns = []string{"/usr/lib/firefox/firefox", "/usr/bin/curl"}
		want = []appChoice{{"curl", "", 1}, {"firefox", "", 1}, {"bash", "", 0}, {"telegram-desktop", "", 0}}
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
