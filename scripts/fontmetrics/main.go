// Command fontmetrics sets the vertical metrics of a TrueType font in place:
// the ascent and descent in hhea, which macOS and Linux lay lines out with;
// OS/2's usWinAscent and usWinDescent, which Windows does (DirectWrite, when
// the font does not ask for its typographic metrics); and OS/2's typographic
// ascent and descent, which other renderers read. Line gaps become 0.
//
// Raise Sans (resources/windows/fonts, resources/linux/fonts) takes Noto
// Sans SC's lines, 1.448 em, taller than those of OPPO Sans, which the
// interface was laid out with: 1.172 em from hhea, 1.319 em on Windows. Text
// cannot set a line shorter than its font's, so its copies here take OPPO
// Sans 4.0's metrics, all of them:
//
//	go run ./scripts/fontmetrics <RaiseSans.ttf> 928 244 1048 271 850 150
//
// Its Han, from -0.12 to 0.88 em, stays inside the lines.
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
)

const usage = "usage: fontmetrics <font.ttf> <ascent> <descent> <win ascent> <win descent> <typo ascent> <typo descent>"

func main() {
	if len(os.Args) != 8 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	var v metrics
	for i, p := range []*int{&v.ascent, &v.descent, &v.winAscent, &v.winDescent, &v.typoAscent, &v.typoDescent} {
		n, err := strconv.Atoi(os.Args[2+i])
		if err != nil || n < 0 || n > 0x7fff {
			fail(fmt.Errorf("metrics are positive font units, as 928 244 1048 271 850 150"))
		}
		*p = n
	}
	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		fail(err)
	}
	if err := set(b, v); err != nil {
		fail(err)
	}
	if err := os.WriteFile(os.Args[1], b, 0o644); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "fontmetrics:", err)
	os.Exit(1)
}

// metrics are font units, descents as positive numbers.
type metrics struct {
	ascent, descent, winAscent, winDescent, typoAscent, typoDescent int
}

type table struct{ dir, off, len int }

// set changes the font b in place and puts its checksums right.
func set(b []byte, v metrics) error {
	be := binary.BigEndian
	if len(b) < 12 {
		return fmt.Errorf("not a font")
	}
	if t := be.Uint32(b); t != 0x00010000 && t != 0x74727565 {
		return fmt.Errorf("not a TrueType font (a collection or CFF?)")
	}
	tables := map[string]table{}
	for i, n := 0, int(be.Uint16(b[4:])); i < n; i++ {
		d := 12 + 16*i
		if d+16 > len(b) {
			return fmt.Errorf("table directory runs past the file")
		}
		t := table{d, int(be.Uint32(b[d+8:])), int(be.Uint32(b[d+12:]))}
		if t.off+t.len > len(b) {
			return fmt.Errorf("table %s runs past the file", b[d:d+4])
		}
		tables[string(b[d:d+4])] = t
	}
	hhea, os2, head := tables["hhea"], tables["OS/2"], tables["head"]
	if hhea.len < 10 || os2.len < 78 || head.len < 12 {
		return fmt.Errorf("no hhea, OS/2 or head")
	}
	i16 := func(off, n int) { be.PutUint16(b[off:], uint16(int16(n))) }
	i16(hhea.off+4, v.ascent)
	i16(hhea.off+6, -v.descent)
	i16(hhea.off+8, 0)
	i16(os2.off+68, v.typoAscent)
	i16(os2.off+70, -v.typoDescent)
	i16(os2.off+72, 0)
	be.PutUint16(b[os2.off+74:], uint16(v.winAscent))
	be.PutUint16(b[os2.off+76:], uint16(v.winDescent))

	// Checksums: each table's, then the file's, which head's
	// checkSumAdjustment makes 0xB1B0AFBA, counted as 0.
	be.PutUint32(b[head.off+8:], 0)
	for _, t := range []table{hhea, os2, head} {
		be.PutUint32(b[t.dir+4:], sum(b[t.off:t.off+t.len]))
	}
	be.PutUint32(b[head.off+8:], 0xB1B0AFBA-sum(b))
	return nil
}

// sum is the OpenType checksum: the data as big-endian uint32s, padded
// with zeros.
func sum(b []byte) uint32 {
	var s uint32
	for i := 0; i < len(b); i += 4 {
		var w [4]byte
		copy(w[:], b[i:])
		s += binary.BigEndian.Uint32(w[:])
	}
	return s
}
