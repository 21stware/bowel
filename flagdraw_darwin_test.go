package main

import (
	"os"
	"os/exec"
	"runtime"
	"testing"

	"github.com/ebitengine/purego/objc"
)

func TestFlagDrawn(t *testing.T) {
	for _, f := range []string{"🇭🇰", "🇯🇵", "🇸🇬", "🇺🇸", "🇨🇭"} {
		if !flagDrawn(f) {
			t.Errorf("flagDrawn(%s) = false, want true", f)
		}
	}
	t.Logf("flagDrawn(🇹🇼) = %v on this Mac", flagDrawn("🇹🇼"))
}

// TestFlagDrawnThenNSFont: drawing a flag in a process that has not loaded
// AppKit yet must leave NSFont working (mygo asks it for the system font
// next). It runs in a fresh process, since by now AppKit may be loaded.
func TestFlagDrawnThenNSFont(t *testing.T) {
	if os.Getenv("MOHO_FLAG_NSFONT") == "1" {
		flagDrawn("🇯🇵")
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
		defer pool.Send(objc.RegisterName("drain"))
		nsFont := objc.ID(objc.GetClass("NSFont"))
		for _, size := range []float64{12, 13, 32} {
			if nsFont.Send(objc.RegisterName("systemFontOfSize:weight:"), size, 0.0) == 0 {
				t.Fatalf("no system font at %v", size)
			}
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestFlagDrawnThenNSFont$", "-test.count=1")
	cmd.Env = append(os.Environ(), "MOHO_FLAG_NSFONT=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("NSFont after flagDrawn: %v\n%s", err, out)
	}
}
