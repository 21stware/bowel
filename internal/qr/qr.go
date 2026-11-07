// Package qr reads QR codes, from pictures and from the screen, and makes
// them. It is pure Go (gozxing); the screen is read with macOS's
// screencapture.
package qr

import (
	"bytes"
	"errors"
	"image"
	_ "image/gif" // the formats a QR code's picture comes in
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"

	"github.com/makiuchi-d/gozxing"
	multiqr "github.com/makiuchi-d/gozxing/multi/qrcode"
	"github.com/makiuchi-d/gozxing/qrcode"
	"github.com/makiuchi-d/gozxing/qrcode/decoder"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

	"github.com/oboo/moho/internal/i18n"
)

// msg is an error said in the interface's language when it is said, and
// comparable with errors.Is.
type msg struct{ zh, en string }

func (m *msg) Error() string { return i18n.T(m.zh, m.en) }

// ErrNone means the picture holds no QR code that reads.
var ErrNone error = &msg{"没有找到二维码", "No QR code found"}

// Decode reads every QR code in a picture, in the order found, without
// repeats.
func Decode(img image.Image) ([]string, error) {
	src := gozxing.NewLuminanceSourceFromImage(img)
	hints := map[gozxing.DecodeHintType]any{gozxing.DecodeHintType_TRY_HARDER: true}
	var out []string
	add := func(s string) {
		if s != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	// Dark on light, then light on dark (a code in a dark theme).
	for _, s := range []gozxing.LuminanceSource{src, src.Invert()} {
		bmp, err := gozxing.NewBinaryBitmap(gozxing.NewHybridBinarizer(s))
		if err != nil {
			continue
		}
		if rs, err := multiqr.NewQRCodeMultiReader().DecodeMultiple(bmp, hints); err == nil {
			for _, r := range rs {
				add(r.GetText())
			}
		}
		if len(out) == 0 {
			// The multi reader gives up on some pictures the single one reads.
			if r, err := qrcode.NewQRCodeReader().Decode(bmp, hints); err == nil {
				add(r.GetText())
			}
		}
		if len(out) > 0 {
			return out, nil
		}
	}
	return nil, ErrNone
}

// DecodeBytes reads the QR codes in a picture's data: PNG, JPEG, GIF, WebP
// or BMP, and on macOS whatever sips converts (HEIC, TIFF, …).
func DecodeBytes(data []byte) ([]string, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		img, err = convert(data)
		if err != nil {
			return nil, errors.New(i18n.T("无法读取这张图片", "Cannot read this picture"))
		}
	}
	return Decode(img)
}

// DecodeFile reads the QR codes in a picture file.
func DecodeFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return DecodeBytes(data)
}

// convert turns a picture Go cannot decode into PNG with sips (macOS).
func convert(data []byte) (image.Image, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.ErrUnsupported
	}
	dir, err := os.MkdirTemp("", "moho-qr-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	in, out := filepath.Join(dir, "in"), filepath.Join(dir, "out.png")
	if err := os.WriteFile(in, data, 0o600); err != nil {
		return nil, err
	}
	if err := exec.Command("/usr/bin/sips", "-s", "format", "png", in, "--out", out).Run(); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(out)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	return img, err
}

// ErrScreen means the screen could not be captured: on macOS, Screen
// Recording has not been allowed.
var ErrScreen error = &msg{"无法读取屏幕", "Cannot read the screen"}

// maxDisplays is how many displays ScanScreen reads.
const maxDisplays = 8

// ScanScreen reads the QR codes on every display.
func ScanScreen() ([]string, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.ErrUnsupported
	}
	dir, err := os.MkdirTemp("", "moho-qr-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	// screencapture writes one file per display, in the order given, and
	// leaves the rest.
	args := []string{"-x", "-t", "png"}
	for i := range maxDisplays {
		args = append(args, filepath.Join(dir, string(rune('a'+i))+".png"))
	}
	runErr := exec.Command("/usr/sbin/screencapture", args...).Run()
	var out []string
	read := 0
	for _, f := range args[3:] {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		read++
		if texts, err := DecodeBytes(data); err == nil {
			for _, t := range texts {
				if !slices.Contains(out, t) {
					out = append(out, t)
				}
			}
		}
	}
	switch {
	case len(out) > 0:
		return out, nil
	case read == 0 && runErr != nil:
		return nil, ErrScreen
	}
	return nil, ErrNone
}

// ErrTooLong means the text does not fit in a QR code.
var ErrTooLong error = &msg{"内容太长，放不进二维码", "Too long to fit in a QR code"}

// Encode makes a QR code of text: one pixel per module, black on white,
// with the standard quiet zone. Show it scaled up, pixelated.
func Encode(text string) (*image.Gray, error) {
	if text == "" {
		return nil, errors.New(i18n.T("内容为空", "Nothing to encode"))
	}
	w := qrcode.NewQRCodeWriter()
	var m *gozxing.BitMatrix
	var err error
	// Medium correction where the text leaves room for it, low where not.
	for _, level := range []decoder.ErrorCorrectionLevel{decoder.ErrorCorrectionLevel_M, decoder.ErrorCorrectionLevel_L} {
		m, err = w.Encode(text, gozxing.BarcodeFormat_QR_CODE, 0, 0, map[gozxing.EncodeHintType]any{
			gozxing.EncodeHintType_CHARACTER_SET:    "UTF-8",
			gozxing.EncodeHintType_ERROR_CORRECTION: level,
			gozxing.EncodeHintType_MARGIN:           4,
		})
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, ErrTooLong
	}
	img := image.NewGray(image.Rect(0, 0, m.GetWidth(), m.GetHeight()))
	for y := range m.GetHeight() {
		for x := range m.GetWidth() {
			if !m.Get(x, y) {
				img.Pix[y*img.Stride+x] = 0xff
			}
		}
	}
	return img, nil
}
