package core

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oboo/moho/internal/devnet"
)

// bundledMihomo is the core this checkout ships, for the tests that need
// the real thing to say what it makes of a config.
func bundledMihomo(t *testing.T) (bin, geo string) {
	t.Helper()
	root := filepath.Join("..", "..", "resources")
	bin = filepath.Join(root, runtime.GOOS+"-"+runtime.GOARCH, "bin", BinName)
	if _, err := os.Stat(bin); err != nil || testing.Short() {
		t.Skip("no bundled core")
	}
	return bin, filepath.Join(root, "geo")
}

func coreDir(t *testing.T, geo string) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range []string{"GeoIP.dat", "GeoSite.dat"} {
		b, err := os.ReadFile(filepath.Join(geo, f))
		if err != nil {
			t.Skip("no bundled geo data")
		}
		if err := os.WriteFile(filepath.Join(dir, f), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The core accepts everything an overlay can put in a config.
func TestCoreAcceptsOverlay(t *testing.T) {
	bin, geo := bundledMihomo(t)
	dir := coreDir(t, geo)
	dev := devnet.Compile(devnet.Options{Extra: []string{"corp.example", "10.8.0.0/16"}}, devEnv())
	for _, mode := range []string{"rule", "global", "direct"} {
		for _, tun := range []bool{false, true} {
			in := devInput(mode, dev)
			in.Tun, in.Sniff = tun, true
			in.Options.DirectNameservers = []string{"system", "10.0.0.53"}
			b, err := BuildConfig(in)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(path, b, 0o600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(bin, "-t", "-d", dir, "-f", path).CombinedOutput(); err != nil {
				t.Errorf("%s tun=%v: %v\n%s\n%s", mode, tun, err, out, b)
			}
		}
	}
}

// answerDNS answers every A question with 127.0.0.1, on a port of its own.
func answerDNS(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			q := buf[:n]
			end := 12
			for end < n && q[end] != 0 {
				end += int(q[end]) + 1
			}
			end += 5
			if end > n {
				continue
			}
			isA := q[end-4] == 0 && q[end-3] == 1
			r := append([]byte{q[0], q[1], 0x81, 0x80, 0, 1, 0, 0, 0, 0, 0, 0}, q[12:end]...)
			if isA {
				r[7] = 1
				r = append(r, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 30, 0, 4, 127, 0, 0, 1)
			}
			_, _ = pc.WriteTo(r, from)
		}
	}()
	return pc.LocalAddr().String()
}

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// Through the real core, with no TUN and in global mode: a peer's short
// name by the bound outbound, a split-DNS name by its own server, and no
// resolver of the core's other than the one that takes the split.
func TestOverlayRuns(t *testing.T) {
	bin, geo := bundledMihomo(t)
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok "+r.Host) }))
	defer web.Close()
	_, webPort, _ := net.SplitHostPort(web.Listener.Addr().String())

	env := devnet.Env{
		Tailscale: devnet.Tailscale{Installed: true, State: "Running", Iface: "lo0", Suffix: "tail.example",
			Peers: []devnet.Peer{{Name: "mybox", FQDN: "mybox.tail.example", IPs: []netip.Addr{netip.MustParseAddr("127.0.0.1")}}}},
		Resolvers: []devnet.Resolver{{Domain: "corp-test", Servers: []string{answerDNS(t)}}},
	}
	port, err := FreePort()
	if err != nil {
		t.Fatal(err)
	}
	probe, err := FreePort()
	if err != nil {
		t.Fatal(err)
	}
	in := devInput("global", devnet.Compile(devnet.Options{}, env))
	in.MixedPort, in.ProbePort, in.FindProcess = port, probe, "off"
	b, err := BuildConfig(in)
	if err != nil {
		t.Fatal(err)
	}
	dir := coreDir(t, geo)
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var log lockedBuf
	cmd := exec.CommandContext(ctx, bin, "-d", dir, "-f", path)
	cmd.Stdout, cmd.Stderr = &log, &log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Wait()
	defer cancel()
	up := false
	for i := 0; i < 100 && !up; i++ {
		if c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond); err == nil {
			c.Close()
			up = true
		} else {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if !up {
		t.Fatalf("the core did not start:\n%s", log.String())
	}
	proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	client := &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}}
	for host, via := range map[string]string{
		"mybox":              "using " + devnet.TailscaleOutbound,
		"mybox.tail.example": "using " + devnet.TailscaleOutbound,
		"api.corp-test":      "using DIRECT",
	} {
		res, err := client.Get("http://" + host + ":" + webPort + "/")
		if err != nil {
			t.Errorf("%s: %v\n%s", host, err, log.String())
			continue
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 || !strings.HasPrefix(string(body), "ok ") {
			t.Errorf("%s: %d %s\n%s", host, res.StatusCode, body, log.String())
		}
		went := slices.ContainsFunc(strings.Split(log.String(), "\n"), func(l string) bool {
			return strings.Contains(l, host+":"+webPort) && strings.Contains(l, via)
		})
		if !went {
			t.Errorf("%s did not go %q:\n%s", host, via, log.String())
		}
	}
}
