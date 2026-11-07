package qr

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"slices"
	"strings"
	"testing"
)

// scaled draws a code larger, at (x, y) of a canvas, as a screen shows it.
func scaled(dst draw.Image, src *image.Gray, x, y, k int, invert bool) {
	b := src.Bounds()
	for j := range b.Dy() * k {
		for i := range b.Dx() * k {
			v := src.GrayAt(i/k, j/k).Y
			if invert {
				v = 255 - v
			}
			dst.Set(x+i, y+j, color.Gray{v})
		}
	}
}

func TestRoundTrip(t *testing.T) {
	for _, text := range []string{
		"https://sub.example.com/api/v1/client/subscribe?token=abc123",
		"trojan://pw@hk.example.com:443#香港 01\nvless://uuid@jp.example.com:443?security=tls#東京",
	} {
		code, err := Encode(text)
		if err != nil {
			t.Fatal(err)
		}
		canvas := image.NewGray(image.Rect(0, 0, 900, 700))
		draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.Gray{0xf0}), image.Point{}, draw.Src)
		scaled(canvas, code, 120, 90, 5, false)
		got, err := Decode(canvas)
		if err != nil || len(got) != 1 || got[0] != text {
			t.Errorf("Decode = %q, %v; want %q", got, err, text)
		}
		// Through PNG bytes, as a file or the clipboard gives it.
		var buf bytes.Buffer
		_ = png.Encode(&buf, canvas)
		if got, err := DecodeBytes(buf.Bytes()); err != nil || got[0] != text {
			t.Errorf("DecodeBytes = %q, %v", got, err)
		}
	}
}

func TestDecodeInvertedAndMany(t *testing.T) {
	a, _ := Encode("ss://YWVzOnB3@a.example:8388#A")
	b, _ := Encode("ss://YWVzOnB3@b.example:8388#B")
	canvas := image.NewGray(image.Rect(0, 0, 1200, 600))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	scaled(canvas, a, 60, 60, 6, false)
	scaled(canvas, b, 700, 120, 6, false)
	got, err := Decode(canvas)
	if err != nil || len(got) != 2 {
		t.Fatalf("two codes: %q, %v", got, err)
	}
	dark := image.NewGray(image.Rect(0, 0, 600, 600))
	scaled(dark, a, 80, 80, 6, true) // light on dark
	if got, err := Decode(dark); err != nil || !slices.Contains(got, "ss://YWVzOnB3@a.example:8388#A") {
		t.Errorf("inverted: %q, %v", got, err)
	}
	if _, err := Decode(image.NewGray(image.Rect(0, 0, 300, 300))); err != ErrNone {
		t.Errorf("blank: %v", err)
	}
}

func TestEncodeTooLong(t *testing.T) {
	if _, err := Encode(strings.Repeat("vmess://x", 600)); err != ErrTooLong {
		t.Errorf("err = %v", err)
	}
}
