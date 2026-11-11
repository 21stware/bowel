package core

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oboo/moho/internal/routes"
)

// signedAs is the code-signing identifier of the program at bin.
func signedAs(t *testing.T, bin string) string {
	t.Helper()
	out, err := exec.Command("/usr/bin/codesign", "-d", "-v", bin).CombinedOutput()
	if err != nil {
		t.Fatalf("codesign -d %s: %v\n%s", bin, err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if id, ok := strings.CutPrefix(line, "Identifier="); ok {
			return id
		}
	}
	return ""
}

// TestCodesignNamesTheCore: signed the way a build signs the code among
// the resources, with no identifier given, a program called BinName comes
// out as dev.moho.app.core. (An ad hoc signature stands in for the
// Developer ID here; the name is made the same way.)
func TestCodesignNamesTheCore(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), BinName)
	if err := os.WriteFile(bin, b, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("/usr/bin/codesign", "--force", "--sign", "-", "--preserve-metadata=entitlements", bin).CombinedOutput(); err != nil {
		t.Skipf("codesign: %v\n%s", err, out)
	}
	if id := signedAs(t, bin); id != "dev.moho.app.core" {
		t.Errorf("signed as %q", id)
	}
}

// TestDetachedCoreNamesProcess runs the real core the way the app does and
// opens a connection through it: the core says which program did, which
// is what a rule for an application matches on. It is the check that macOS
// still shows the socket list to a core started and signed like this.
func TestDetachedCoreNamesProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	bin := coreBin(t)
	if id := signedAs(t, bin); !strings.Contains(id, ".") {
		t.Skipf("the core at %s is signed as %q; macOS shows the socket list only to an identifier with a dot", bin, id)
	}
	dir, err := os.MkdirTemp("", "moho")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	geo(t, dir)
	m, err := New(bin, dir)
	if err != nil {
		t.Fatal(err)
	}
	probe, _ := FreePort()
	port := mustFree(t)
	cfg, err := BuildConfig(Input{
		Routes: routes.Template(), Mode: "direct", MixedPort: port, LogLevel: "info",
		TestURL: "https://www.gstatic.com/generate_204", ProbePort: probe, Controller: m.ControllerFields(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Apply(cfg); err != nil {
		t.Fatalf("start: %v\n%s", err, m.logs.last(10))
	}
	defer m.Stop()

	// Somewhere to connect to, and a connection held open through the core.
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() { _, _ = io.Copy(io.Discard, c); c.Close() }()
		}
	}()
	c, err := net.DialTimeout("tcp4", fmt.Sprintf("127.0.0.1:%d", port), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", l.Addr(), l.Addr())
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 256)
	if n, err := c.Read(buf); err != nil || !strings.Contains(string(buf[:n]), " 200 ") {
		t.Fatalf("CONNECT: %q, %v", buf[:n], err)
	}

	self, _ := os.Executable()
	self, _ = filepath.EvalSymlinks(self)
	cl, _ := m.Client()
	local := fmt.Sprint(c.LocalAddr().(*net.TCPAddr).Port)
	deadline := time.Now().Add(5 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		snap, err := cl.Connections(ctx)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		for _, cn := range snap.Connections {
			if cn.Metadata.SourcePort != local {
				continue
			}
			got, _ := filepath.EvalSymlinks(cn.Metadata.ProcessPath)
			if got != self || cn.Metadata.Process != filepath.Base(self) {
				t.Fatalf("the core names %q at %q, not %q", cn.Metadata.Process, cn.Metadata.ProcessPath, self)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the core lists no connection from port %s: %+v", local, snap.Connections)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
