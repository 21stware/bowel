package tun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oboo/moho/internal/i18n"
)

func asRoot(t *testing.T, goos string, uid int) {
	t.Helper()
	e, g := euid, rootGOOS
	t.Cleanup(func() { euid, rootGOOS = e, g })
	euid, rootGOOS = func() int { return uid }, goos
}

// TestRootCore: only root on Linux runs the bundled core for TUN.
func TestRootCore(t *testing.T) {
	for _, c := range []struct {
		goos string
		uid  int
		want bool
	}{{"linux", 0, true}, {"linux", 1000, false}, {"darwin", 0, false}, {"windows", 0, false}} {
		asRoot(t, c.goos, c.uid)
		if got := RootCore(); got != c.want {
			t.Errorf("%s uid %d: RootCore %v", c.goos, c.uid, got)
		}
	}
}

func TestRootReady(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "mihomo")
	if err := os.WriteFile(bin, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(dir, "data")
	if err := os.WriteFile(data, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !rootReady(bin) {
		t.Error("an executable isn't ready for root")
	}
	if rootReady(data) || rootReady(dir) || rootReady(filepath.Join(dir, "missing")) {
		t.Error("a file that can't be run, a directory or nothing is ready for root")
	}
}

// TestDeviceReady: the device and NET_ADMIN are what a container may lack,
// and the error names the flags that give them.
func TestDeviceReady(t *testing.T) {
	i18n.Set("en")
	t.Cleanup(func() { i18n.Set("zh") })
	dir := t.TempDir()
	dev := filepath.Join(dir, "tun")
	status := filepath.Join(dir, "status")
	write := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	err := deviceReady(dev, status)
	if err == nil || !strings.Contains(err.Error(), "--device /dev/net/tun") {
		t.Errorf("no device: %v", err)
	}
	write(dev, "")
	write(status, "Name:\tbowel\nCapEff:\t00000000a80425fb\n") // Docker's default set: no NET_ADMIN
	if err := deviceReady(dev, status); err == nil || !strings.Contains(err.Error(), "--cap-add NET_ADMIN") {
		t.Errorf("no NET_ADMIN: %v", err)
	}
	write(status, "Name:\tbowel\nCapEff:\t00000000a80435fb\n") // with it
	if err := deviceReady(dev, status); err != nil {
		t.Errorf("device and NET_ADMIN: %v", err)
	}
	if err := deviceReady(dev, filepath.Join(dir, "missing")); err != nil {
		t.Errorf("capabilities unknown: %v", err)
	}
	asRoot(t, "linux", 1000)
	if err := DeviceReady(); err != nil {
		t.Errorf("not root: %v", err)
	}
}

func TestEffectiveCap(t *testing.T) {
	if have, known := effectiveCap("CapEff:\t000001ffffffffff\n", capNetAdmin); !have || !known {
		t.Error("root's full set lacks NET_ADMIN")
	}
	if _, known := effectiveCap("Name: x\n", capNetAdmin); known {
		t.Error("no CapEff line is known")
	}
}
