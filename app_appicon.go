package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/image/draw"

	"github.com/oboo/moho/internal/nowindow"
)

// bundleIcon is the icon of a macOS bundle as a PNG at least px wide (the
// smallest such, else the largest), nil when it has none bowel can read:
// elsewhere, or an icon file without a PNG in it.
func bundleIcon(bundle string, px int) []byte {
	if runtime.GOOS != "darwin" || bundle == "" {
		return nil
	}
	name := "AppIcon"
	// Info.plist is often binary: plutil reads both kinds.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := nowindow.CommandContext(ctx, "/usr/bin/plutil", "-extract", "CFBundleIconFile", "raw", "-o", "-",
		filepath.Join(bundle, "Contents", "Info.plist")).Output()
	if s := strings.TrimSpace(string(out)); err == nil && s != "" {
		name = s
	}
	if !strings.HasSuffix(strings.ToLower(name), ".icns") {
		name += ".icns"
	}
	data, err := os.ReadFile(filepath.Join(bundle, "Contents", "Resources", filepath.Base(name)))
	if err != nil {
		return nil
	}
	return icnsPNG(data, px)
}

// icnsPNG picks a PNG out of an Apple icon file: a header ("icns", the
// length) and entries of a type, a length and the data. Icons since macOS
// 10.7 keep their sizes as PNGs; older kinds of entry are passed over.
func icnsPNG(data []byte, px int) []byte {
	if len(data) < 8 || string(data[:4]) != "icns" {
		return nil
	}
	end := min(len(data), int(binary.BigEndian.Uint32(data[4:])))
	var best []byte
	bestW := 0
	for i := 8; i+8 <= end; {
		n := int(binary.BigEndian.Uint32(data[i+4:]))
		if n < 8 || i+n > end {
			break
		}
		png := data[i+8 : i+n]
		if w := pngWidth(png); w > 0 {
			// The smallest at least px wide; until there is one, the largest.
			if best == nil || (w >= px && (bestW < px || w < bestW)) || (bestW < px && w > bestW) {
				best, bestW = png, w
			}
		}
		i += n
	}
	return best
}

// pngWidth is a PNG's width from its header, 0 when b is not a PNG.
func pngWidth(b []byte) int {
	if len(b) < 24 || !bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")) || string(b[12:16]) != "IHDR" {
		return 0
	}
	return int(binary.BigEndian.Uint32(b[16:]))
}

// maxIcon is the largest side of an icon bowel decodes. A PNG's header
// can claim any size, and the decoder allocates what it claims.
const maxIcon = 1024

// iconImage is an icon's PNG decoded and scaled down to px wide, as it is
// kept while the window lasts; nil when it isn't a PNG or is larger than
// maxIcon a side.
func iconImage(b []byte, px int) image.Image {
	cfg, err := png.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxIcon || cfg.Height > maxIcon {
		return nil
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return nil
	}
	if cfg.Width <= px {
		return img
	}
	small := image.NewRGBA(image.Rect(0, 0, px, max(1, cfg.Height*px/cfg.Width)))
	draw.CatmullRom.Scale(small, small.Bounds(), img, img.Bounds(), draw.Src, nil)
	return small
}
