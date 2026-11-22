package owner

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// listen opens a Server in a short folder: a unix socket's path has a
// limit, which macOS's temporary folders come close to.
func listen(t *testing.T) (*Server, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "o")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "owner.sock")
	s, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

// service is the VPN service's side: it connects n times and answers each
// question with answer, after wait.
type service struct {
	mu    sync.Mutex
	asked []string
	wait  time.Duration
	conns []net.Conn
}

func connect(t *testing.T, s *Server, path string, n int, answer func(q string) string) *service {
	t.Helper()
	v := &service{}
	want := s.open.Load() + int32(n)
	for i := 0; i < n; i++ {
		c, err := net.Dial("unix", path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		v.conns = append(v.conns, c)
		go func() {
			r := bufio.NewReader(c)
			for {
				q, err := r.ReadString('\n')
				if err != nil {
					return
				}
				q = strings.TrimSuffix(q, "\n")
				v.mu.Lock()
				v.asked = append(v.asked, q)
				wait := v.wait
				v.mu.Unlock()
				time.Sleep(wait)
				if _, err := io.WriteString(c, answer(q)+"\n"); err != nil {
					return
				}
			}
		}()
	}
	for deadline := time.Now().Add(2 * time.Second); s.open.Load() < want; {
		if time.Now().After(deadline) {
			t.Fatalf("the server took %d of %d connections", s.open.Load(), want)
		}
		time.Sleep(time.Millisecond)
	}
	return v
}

func (v *service) questions() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]string(nil), v.asked...)
}

var (
	src = netip.MustParseAddrPort("198.18.0.1:40312")
	dst = netip.MustParseAddrPort("198.18.0.9:443")
)

func chatgpt(string) string { return "10234 com.openai.chatgpt" }

// The service is asked for the connection's two ends and its answer is
// the uid and the package.
func TestLookupAsksTheService(t *testing.T) {
	s, path := listen(t)
	v := connect(t, s, path, 1, chatgpt)
	uid, pkg, err := s.Lookup("tcp", src, dst)
	if err != nil || uid != 10234 || pkg != "com.openai.chatgpt" {
		t.Fatalf("Lookup = %d %q %v", uid, pkg, err)
	}
	if _, _, err := s.Lookup("udp", src, netip.MustParseAddrPort("[::ffff:198.18.0.9]:53")); err != nil {
		t.Fatal(err)
	}
	want := []string{"tcp 198.18.0.1:40312 198.18.0.9:443", "udp 198.18.0.1:40312 198.18.0.9:53"}
	if got := v.questions(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("asked %q, want %q (an IPv4 address as IPv4)", got, want)
	}
}

// Android not knowing the connection, a uid with no package and an answer
// that makes no sense each say so.
func TestLookupAnswers(t *testing.T) {
	s, path := listen(t)
	answers := map[string]string{"udp 198.18.0.1:1 198.18.0.9:1": "-1 ", "udp 198.18.0.1:2 198.18.0.9:1": "1000 ", "udp 198.18.0.1:3 198.18.0.9:1": "what"}
	connect(t, s, path, 1, func(q string) string { return answers[q] })
	ask := func(port uint16) (uint32, string, error) {
		return s.Lookup("udp", netip.AddrPortFrom(src.Addr(), port), netip.AddrPortFrom(dst.Addr(), 1))
	}
	if _, _, err := ask(1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown connection: %v", err)
	}
	if uid, pkg, err := ask(2); err != nil || uid != 1000 || pkg != "" {
		t.Fatalf("uid without a package = %d %q %v", uid, pkg, err)
	}
	if _, _, err := ask(3); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("nonsense = %v", err)
	}
	if _, _, err := s.Lookup("all", src, dst); !errors.Is(err, ErrNetwork) {
		t.Fatalf("network all = %v", err)
	}
}

// With no service connected, as before Android 10, a lookup fails at once
// instead of holding the connection up.
func TestLookupWithoutService(t *testing.T) {
	s, _ := listen(t)
	began := time.Now()
	if _, _, err := s.Lookup("tcp", src, dst); !errors.Is(err, ErrNoService) {
		t.Fatalf("Lookup = %v", err)
	}
	if d := time.Since(began); d > s.timeout/2 {
		t.Fatalf("took %v", d)
	}
}

// A slow answer is given up, and its line with it, so that the late answer
// is not taken for the next question's.
func TestLookupGivesUpOnASlowService(t *testing.T) {
	s, path := listen(t)
	s.timeout = 50 * time.Millisecond
	slow := connect(t, s, path, 1, chatgpt)
	slow.mu.Lock()
	slow.wait = 200 * time.Millisecond
	slow.mu.Unlock()
	began := time.Now()
	if _, _, err := s.Lookup("tcp", src, dst); !errors.Is(err, ErrTimeout) {
		t.Fatalf("Lookup = %v", err)
	}
	if d := time.Since(began); d > 150*time.Millisecond {
		t.Fatalf("took %v", d)
	}
	if n := s.open.Load(); n != 0 {
		t.Fatalf("%d lines left open", n)
	}
	connect(t, s, path, 1, func(string) string { return "10001 org.example.next" })
	if _, pkg, err := s.Lookup("tcp", src, dst); err != nil || pkg != "org.example.next" {
		t.Fatalf("after the service connects again: %q %v", pkg, err)
	}
}

// A line the service closed is skipped for another.
func TestLookupSkipsAClosedLine(t *testing.T) {
	s, path := listen(t)
	v := connect(t, s, path, 2, chatgpt)
	v.conns[0].Close()
	v.conns[1].Close()
	connect(t, s, path, 1, chatgpt)
	for i := 0; i < 3; i++ {
		if _, pkg, err := s.Lookup("tcp", src, dst); err != nil || pkg != "com.openai.chatgpt" {
			t.Fatalf("Lookup = %q %v", pkg, err)
		}
	}
	if n := s.open.Load(); n != 1 {
		t.Fatalf("%d lines open, want the one that answers", n)
	}
}

// Lookups at once share the lines, one question on a line at a time.
func TestLookupsAtOnce(t *testing.T) {
	s, path := listen(t)
	connect(t, s, path, 2, func(q string) string {
		_, port, _ := strings.Cut(strings.Fields(q)[1], ":")
		return "1" + port + " app" + port
	})
	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		port := uint16(2000 + i)
		go func() {
			defer wg.Done()
			uid, pkg, err := s.Lookup("tcp", netip.AddrPortFrom(src.Addr(), port), dst)
			if err == nil && (pkg != "app"+strconv.Itoa(int(port)) || uid != uint32(10000+int(port))) {
				err = errors.New("crossed answers: " + pkg)
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

// Once closed, nothing is asked and the socket is gone.
func TestClose(t *testing.T) {
	s, path := listen(t)
	connect(t, s, path, 1, chatgpt)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Lookup("tcp", src, dst); !errors.Is(err, ErrNoService) {
		t.Fatalf("Lookup after Close = %v", err)
	}
	if c, err := net.Dial("unix", path); err == nil {
		c.Close()
		t.Fatal("still listening")
	}
}

// The ends come from the addresses the TUN saw when there are any, and from
// the metadata's fields otherwise.
func TestAddrPort(t *testing.T) {
	ip, port := netip.MustParseAddr("192.0.2.1"), uint16(1)
	if got := AddrPort(&net.TCPAddr{IP: net.ParseIP("198.18.0.9"), Port: 443}, ip, port); got.Addr().Unmap() != dst.Addr() || got.Port() != 443 {
		t.Fatalf("TCP = %v", got)
	}
	if got := AddrPort(&net.UDPAddr{IP: net.IPv4(198, 18, 0, 9), Port: 53}, ip, port); unmap(got).String() != "198.18.0.9:53" {
		t.Fatalf("UDP = %v", got)
	}
	var none *net.TCPAddr
	for _, a := range []net.Addr{nil, none, &net.UnixAddr{Name: "x"}} {
		if got := AddrPort(a, ip, port); got != netip.AddrPortFrom(ip, port) {
			t.Fatalf("%#v = %v", a, got)
		}
	}
}
