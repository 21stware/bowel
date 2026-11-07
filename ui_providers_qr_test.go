package main

import (
	"testing"
	"time"

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
