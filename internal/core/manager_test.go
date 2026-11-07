package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/oboo/moho/internal/routes"
)

func coreBin(t *testing.T) string {
	if p := os.Getenv("MOHO_CORE"); p != "" {
		return p
	}
	p, _ := filepath.Abs(filepath.Join("..", "..", "resources", runtime.GOOS+"-"+runtime.GOARCH, "bin", "mihomo"))
	if _, err := os.Stat(p); err != nil {
		t.Skip("no mihomo binary; run scripts/fetch-core.sh")
	}
	return p
}

// geo copies the bundled geo data into dir, as the app does.
func geo(t *testing.T, dir string) {
	src, _ := filepath.Abs(filepath.Join("..", "..", "resources", "geo"))
	for _, f := range []string{"GeoSite.dat", "GeoIP.dat"} {
		b, err := os.ReadFile(filepath.Join(src, f))
		if err != nil {
			t.Skip("no geo data; run scripts/fetch-core.sh")
		}
		_ = os.WriteFile(filepath.Join(dir, f), b, 0o600)
	}
}

const yamlNodes = "proxies:\n  - {name: \"东京 01\", type: direct}\n  - {name: \"香港 02\", type: direct}\n"
const linkNodes = "trojan://pw@203.0.113.11:443?sni=example.com#%E9%A6%99%E6%B8%AF%2002\n"

func TestManagerLifecycle(t *testing.T) {
	bin := coreBin(t)
	dir, err := os.MkdirTemp("", "moho")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	geo(t, dir)
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte(yamlNodes), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "b.txt"), []byte(linkNodes), 0o600)
	m, err := New(bin, dir)
	if err != nil {
		t.Fatal(err)
	}
	probe, _ := FreePort()
	in := Input{
		Providers: []ProviderRef{{ID: "aaa", Path: filepath.Join(dir, "a.txt")}, {ID: "bbb", Path: filepath.Join(dir, "b.txt")}},
		Routes:    routes.Template(), Mode: "rule", MixedPort: 17899, LogLevel: "info",
		TestURL: "https://www.gstatic.com/generate_204", ProbePort: probe, Controller: m.ControllerFields(),
	}
	cfg, err := BuildConfig(in)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Apply(cfg); err != nil {
		t.Fatalf("start: %v\n%s\n%s", err, m.logs.last(10), cfg)
	}
	defer m.Stop()

	c, _ := m.Client()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	px, err := c.Proxies(ctx)
	if err != nil {
		t.Fatal(err)
	}
	g := px["默认"]
	if g.Type != "Selector" || g.Now != "默认·自动" {
		t.Fatalf("默认 = %+v", g)
	}
	// Both providers' 香港 02 are there, told apart by the marker.
	var hk []string
	for _, n := range g.All {
		if strings.HasPrefix(n, "香港 02"+NodeSep) {
			hk = append(hk, n)
		}
	}
	if len(hk) != 2 {
		t.Fatalf("香港 02 nodes = %q in %q", hk, g.All)
	}
	if err := c.Select(ctx, "AI 服务", "东京 01"+NodeSep+"aaa"); err != nil {
		t.Fatal(err)
	}
	if _, ok := px[ProbeGroup]; !ok {
		t.Fatal("no probe group")
	}
	// The 设置 window's DNS buttons; the core answers even without DNS on.
	if err := c.FlushDNS(ctx); err != nil {
		t.Errorf("flush DNS: %v", err)
	}
	if err := c.FlushFakeIP(ctx); err != nil {
		t.Errorf("flush fake-ip: %v", err)
	}

	// Reload with a provider disabled, without restarting.
	pid := m.Status().PID
	in.Providers = in.Providers[:1]
	in.Mode = "global"
	cfg, _ = BuildConfig(in)
	if err := m.Apply(cfg); err != nil {
		t.Fatal(err)
	}
	conf, _ := c.Config(ctx)
	if conf.Mode != "global" || m.Status().PID != pid {
		t.Fatalf("after reload: %+v pid %d → %d", conf, pid, m.Status().PID)
	}
	m.Stop()
	if s := m.Status(); s.State != Stopped {
		t.Fatalf("after stop: %+v", s)
	}
}

func TestBuildConfigWithoutProviders(t *testing.T) {
	out, err := BuildConfig(Input{Routes: routes.Template(), Mode: "rule", MixedPort: 7899, ProbePort: 7900, TestURL: "x"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"MATCH,默认", "GEOSITE,cn,DIRECT", "DOMAIN-SUFFIX,openai.com,AI 服务", "proxy: MOHO-PROBE"} {
		if !strings.Contains(s, want) {
			t.Errorf("config lacks %q", want)
		}
	}
	if strings.Contains(s, "proxy-providers") || strings.Contains(s, "tun:") {
		t.Errorf("unexpected section:\n%s", s)
	}
}
