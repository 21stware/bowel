package main

import "testing"

func TestPeekWidth(t *testing.T) {
	ws := []float32{40, 60, 60, 60}
	for _, tc := range []struct {
		avail float32
		want  float32
		cut   bool
	}{
		{300, 0, false},  // all 226 fit
		{226, 0, false},  // exactly fits
		{200, 196, true}, // ends halfway through the last chip
		{150, 134, true}, // halfway through the third
		{133, 72, true},  // the third's middle is past: halfway through the second
		{19, 0, false},   // not even half the first: just shrink
	} {
		w, cut := peekWidth(ws, 2, tc.avail)
		if w != tc.want || cut != tc.cut {
			t.Errorf("peekWidth(%v) = %v, %v; want %v, %v", tc.avail, w, cut, tc.want, tc.cut)
		}
	}
}
