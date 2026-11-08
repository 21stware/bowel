package main

import (
	"image"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/qr"
)

func TestProviderShareQR(t *testing.T) {
	const sub = "https://sub.example-a.com/api/v1/client/subscribe?token=abc"
	u := providersUIFixture()
	u.prov.share = func(id string) (string, error) { return sub, nil }
	u.prov.open = "a"
	tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
	tt.Advance(time.Second)
	for _, s := range []string{"复制订阅链接", "二维码"} {
		if !tt.HasText(s) {
			t.Fatalf("open row lacks %q: %q", s, tt.Texts())
		}
	}
	if err := tt.Click("复制订阅链接"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if got := tt.Clipboard(); got != sub {
		t.Errorf("clipboard = %q", got)
	}

	if err := tt.Click("二维码"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	shot(t, tt, "providers-share")
	if !u.prov.shareOpen || u.prov.shareCode == nil || !tt.HasText(sub) || !tt.HasText("存储图片…") {
		t.Fatalf("share dialog: open=%v %q", u.prov.shareOpen, tt.Texts())
	}
	// The code it shows reads back as the link.
	if got, err := qr.Decode(u.prov.shareCode); err != nil || got[0] != sub {
		t.Errorf("code reads %q, %v", got, err)
	}
	if err := tt.Click("完成"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if u.prov.shareOpen || u.prov.shareText != "" {
		t.Errorf("share dialog did not close and forget the link")
	}

	// A local file has neither.
	u.prov.open = "d"
	for i, pv := range u.snap.Providers {
		if pv.ID == "d" {
			u.snap.Providers[i].Kind = "file"
		}
	}
	tt.Frame()
	tt.Advance(time.Second)
	if tt.HasText("二维码") {
		t.Errorf("a file offers a QR code")
	}
}

func TestProviderScanMenu(t *testing.T) {
	u := providersUIFixture()
	tt := ui.NewTesterWith(u.view, 1060, 760, ui.TesterOptions{})
	tt.Advance(time.Second)
	if err := tt.Click("扫码"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	shot(t, tt, "providers-scan-menu")
	for _, s := range []string{"用摄像头扫描", "扫描屏幕上的二维码", "从图片读取…", "从剪贴板里的图片读取"} {
		if !tt.HasText(s) {
			t.Errorf("scan menu lacks %q: %q", s, tt.Texts())
		}
	}
	// A code read fills the add bar, to be checked before 添加.
	tt.Key(0, ui.KeyEscape)
	u.provQRRead("trojan://pw@hk.example.com:443#香港")
	tt.Advance(time.Second)
	if u.prov.input == "" || !tt.HasText("节点链接") {
		t.Errorf("add bar after a read: %q %q", u.prov.input, tt.Texts())
	}
}

// TestScreenSpots puts codes read off the screen's captures on their
// displays, in DIPs: a Retina capture is twice its display, and a capture
// whose shape is not that of the display at its index goes to the one it
// matches.
func TestScreenSpots(t *testing.T) {
	displays := []mygo.Display{
		{Bounds: mygo.Rectangle{X: 0, Y: 0, Width: 1512, Height: 982}, ScaleFactor: 2},
		{Bounds: mygo.Rectangle{X: -1080, Y: -400, Width: 1080, Height: 1920}, ScaleFactor: 1}, // portrait
		{Bounds: mygo.Rectangle{X: 1512, Y: 0, Width: 2560, Height: 1440}, ScaleFactor: 1},
	}
	codes := []qr.ScreenCode{
		{Code: qr.Code{Text: "a", Rect: image.Rect(200, 100, 600, 500)}, Display: 0, Size: image.Pt(3024, 1964)},
		// The second capture is the 2560×1440 display, not the portrait one.
		{Code: qr.Code{Text: "b", Rect: image.Rect(10, 20, 110, 120)}, Display: 1, Size: image.Pt(2560, 1440)},
		{Code: qr.Code{Text: "c", Rect: image.Rect(1000, 20, 1100, 120)}, Display: 1, Size: image.Pt(2560, 1440)},
		{Code: qr.Code{Text: "d", Rect: image.Rect(0, 0, 50, 50)}, Display: 5, Size: image.Pt(800, 600)}, // no such display
	}
	got := screenSpots(codes, displays)
	want := []qrSpots{
		{Display: displays[0].Bounds, Rects: []mygo.Rectangle{{X: 100, Y: 50, Width: 200, Height: 200}}},
		{Display: displays[2].Bounds, Rects: []mygo.Rectangle{{X: 10, Y: 20, Width: 100, Height: 100}, {X: 1000, Y: 20, Width: 100, Height: 100}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("screenSpots =\n%+v\nwant\n%+v", got, want)
	}
}

// TestQRHintView frames a code with what became of it.
func TestQRHintView(t *testing.T) {
	u := providersUIFixture()
	for _, c := range []struct {
		ok   bool
		word string
	}{{true, "已读取这个二维码"}, {false, "读到了，但不是节点二维码"}} {
		h := &qrHint{spots: qrSpots{Display: mygo.Rectangle{Width: 1200, Height: 800}, Rects: []mygo.Rectangle{{X: 500, Y: 20, Width: 200, Height: 200}}}, ok: c.ok, shown: true}
		tt := ui.NewTesterWith(func(cx *ui.Context) { u.qrHintView(cx, h) }, 1200, 800, ui.TesterOptions{})
		tt.Advance(time.Second)
		if !tt.HasText(c.word) {
			t.Errorf("ok=%v: no %q", c.ok, c.word)
		}
	}
}
