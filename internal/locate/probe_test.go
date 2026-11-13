package locate_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/locate"
	"github.com/oboo/moho/internal/routes"
)

func TestProbeDirect(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	bin, _ := filepath.Abs(filepath.Join("..", "..", "resources", runtime.GOOS+"-"+runtime.GOARCH, "bin", core.BinName))
	if p := os.Getenv("MOHO_CORE"); p != "" {
		bin = p
	}
	if _, err := os.Stat(bin); err != nil {
		t.Skip("no mihomo binary; run scripts/fetch-core.sh")
	}
	if c, err := net.DialTimeout("tcp", "www.cloudflare.com:443", 5*time.Second); err != nil {
		t.Skipf("no network: %v", err)
	} else {
		c.Close()
	}

	dir, err := os.MkdirTemp("", "loc") // short: unix socket paths must be < 104 bytes
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	src, _ := filepath.Abs(filepath.Join("..", "..", "resources", "geo"))
	for _, f := range []string{"GeoSite.dat", "GeoIP.dat"} {
		b, err := os.ReadFile(filepath.Join(src, f))
		if err != nil {
			t.Skip("no geo data; run scripts/fetch-core.sh")
		}
		_ = os.WriteFile(filepath.Join(dir, f), b, 0o600)
	}

	m, err := core.New(bin, dir)
	if err != nil {
		t.Fatal(err)
	}
	mixed, _ := core.FreePort()
	probe, _ := core.FreePort()
	cfg, err := core.BuildConfig(core.Input{
		Routes: routes.Template(), Mode: "rule", MixedPort: mixed, LogLevel: "warning",
		TestURL: "https://www.gstatic.com/generate_204", ProbePort: probe, Controller: m.ControllerFields(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Apply(cfg); err != nil {
		t.Fatalf("start core: %v", err)
	}
	defer m.Stop()
	c, err := m.Client()
	if err != nil {
		t.Fatal(err)
	}

	cache := filepath.Join(dir, "locate.json")
	l := locate.New(cache)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	r, err := l.Probe(ctx, c, probe, locate.Direct)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if r.Source != locate.SourceProbe || r.IP == "" || r.Place == nil {
		t.Fatalf("probe DIRECT: %+v", r)
	}
	t.Logf("DIRECT exits at %+v (ip %s)", *r.Place, r.IP)

	// Persisted, and Guess now answers from the probe.
	if g := locate.New(cache).Guess(locate.Direct, "DIRECT"); g.Source != locate.SourceProbe || g.Place == nil || g.Place.Country != r.Place.Country {
		t.Fatalf("guess after probe: %+v", g)
	}
}
