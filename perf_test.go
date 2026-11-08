package main

// What the interface costs while it sits there: how many times a second
// the view is built when nothing changes, and what a build and a paint
// take.
//
//	CGO_ENABLED=0 go test -run IdleBuilds .
//	CGO_ENABLED=0 go test -run xxx -bench . -benchmem .

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// perfUI is the stress UI on a page, its builds counted in builds.
func perfUI(page, borders string, builds *int) (*UI, *ui.Tester) {
	u := stressUI()
	u.page = page
	u.snap.Settings.Borders = borders
	if page == "activity" {
		u.act.apply(stressConns(3000), stressLogs(5000))
	}
	tt := ui.NewTester(func(c *ui.Context) { *builds++; u.view(c) }, 1180, 760)
	tt.Advance(3 * time.Second) // past the opening animations
	return u, tt
}

// TestIdleBuilds: an idle second builds no page again. What moves on its
// own, as the map's comets, repaints its drawing; building the view would
// lay out the whole window at the display's rate.
func TestIdleBuilds(t *testing.T) {
	for _, tc := range []struct{ page, borders string }{
		{"home", "straight"}, {"home", "twisted"}, {"activity", "straight"},
	} {
		var n int
		_, tt := perfUI(tc.page, tc.borders, &n)
		n = 0
		tt.Advance(time.Second)
		if n > 0 {
			t.Errorf("%s (%s borders): %d builds in an idle second, want 0", tc.page, tc.borders, n)
		}
	}
}

func benchFrame(b *testing.B, page, borders string) {
	var n int
	_, tt := perfUI(page, borders, &n)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tt.Frame()
	}
}

func BenchmarkFrameHome(b *testing.B)        { benchFrame(b, "home", "straight") }
func BenchmarkFrameHomeTwisted(b *testing.B) { benchFrame(b, "home", "twisted") }
func BenchmarkFrameActivity(b *testing.B)    { benchFrame(b, "activity", "straight") }

// BenchmarkTick is the once-a-second update: a new Tick, and the frame
// it brings.
func BenchmarkTick(b *testing.B) {
	var n int
	u, tt := perfUI("home", "straight", &n)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t := stressTick(i)
		u.tick = t
		u.home.pushTick(t)
		tt.Frame()
	}
}

// BenchmarkIdleSecond is a second of the home page with nothing changing:
// the frames its drawings that move ask for.
func BenchmarkIdleSecond(b *testing.B) {
	var n int
	_, tt := perfUI("home", "straight", &n)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tt.Advance(time.Second)
	}
}

// BenchmarkIdleSecondSkins is BenchmarkIdleSecond in each skin: what its
// texture, painted over the window, adds.
func BenchmarkIdleSecondSkins(b *testing.B) {
	for _, k := range skins {
		b.Run(k.id, func(b *testing.B) {
			var n int
			u, tt := perfUI("home", "straight", &n)
			u.snap.Settings.Skin = k.id
			tt.Advance(time.Second)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tt.Advance(time.Second)
			}
		})
	}
}
