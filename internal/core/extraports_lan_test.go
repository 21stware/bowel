package core

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/oboo/moho/internal/routes"
)

// ownAddr is an IPv4 address of this machine that is not loopback: a
// connection from this machine to it comes from it, as one from the LAN
// comes from the LAN.
func ownAddr(t *testing.T) string {
	t.Helper()
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && !n.IP.IsLoopback() && !n.IP.IsLinkLocalUnicast() {
				return n.IP.String()
			}
		}
	}
	t.Skip("no address but loopback")
	return ""
}

// TestExtraPortsOnTheLanRealCore: with allowLan on, a port marked lan is
// reached at the machine's own address, behind the mixed port's
// authentication and only from allowLan's networks; this machine itself
// needs no password; one not marked is not there at all; and with allowLan
// off neither is. UDP goes through it by SOCKS5, to the port's group.
//
// Linux only: on Windows and macOS a port on every address can bring up
// the firewall's question in front of whoever runs the tests.
func TestExtraPortsOnTheLanRealCore(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("listening on every address asks the firewall here")
	}
	bin := coreBin(t)
	ip := ownAddr(t)
	dir := t.TempDir()
	geo(t, dir)
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte(yamlNodes), 0o600)
	m, err := New(bin, dir)
	if err != nil {
		t.Fatal(err)
	}
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))
	defer web.Close()
	echo, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, err := echo.ReadFrom(buf)
			if err != nil {
				return
			}
			echo.WriteTo(buf[:n], from)
		}
	}()

	mixed, lan, local, probe := mustFree(t), mustFree(t), mustFree(t), mustFree(t)
	in := Input{
		Providers: []ProviderRef{{ID: "aaa", Path: filepath.Join(dir, "a.txt")}},
		Routes:    routes.Template(), Mode: "direct", MixedPort: mixed, LogLevel: "info", AllowLan: true,
		TestURL: "http://x/", ProbePort: probe, Controller: m.ControllerFields(),
		Options: Options{FindProcess: "off", Authentication: []string{"u:p"}, LanAllowedIPs: []string{"127.0.0.0/8", ip + "/32"},
			ExtraPorts: []ExtraPort{{Port: lan, Route: "ai", Lan: true}, {Port: local, Route: "media"}}},
	}
	apply := func() {
		t.Helper()
		cfg, err := BuildConfig(in)
		if err != nil {
			t.Fatal(err)
		}
		if err := m.Apply(cfg); err != nil {
			t.Fatalf("apply: %v\n%s\n%s", err, m.logs.last(10), cfg)
		}
	}
	apply()
	defer m.Stop()

	// get is the status of a request through host:port, as user (no
	// password if ""), or the error that stopped it.
	get := func(host string, port int, user string) (int, error) {
		proxy := &url.URL{Scheme: "http", Host: net.JoinHostPort(host, strconv.Itoa(port))}
		if user != "" {
			proxy.User = url.UserPassword(user, "p")
		}
		client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}}
		res, err := client.Get(web.URL)
		if err != nil {
			return 0, err
		}
		res.Body.Close()
		return res.StatusCode, nil
	}
	waitFor := func(what string, ok func() bool) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); !ok(); time.Sleep(50 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("%s: status %+v\n%s", what, m.Status(), m.logs.last(30))
			}
		}
	}
	waitFor("the core routes", func() bool { code, _ := get("127.0.0.1", mixed, ""); return code == 200 })
	waitFor("the port is open", func() bool { code, _ := get(ip, lan, "u"); return code == 200 })

	if code, err := get(ip, lan, ""); code != http.StatusProxyAuthRequired {
		t.Errorf("from the LAN without a password: %d %v", code, err)
	}
	if code, err := get("127.0.0.1", lan, ""); code != 200 {
		t.Errorf("from this machine: %d %v", code, err)
	}
	if code, err := get("127.0.0.1", local, ""); code != 200 {
		t.Errorf("the local port from this machine: %d %v", code, err)
	}
	if _, err := net.DialTimeout("tcp", net.JoinHostPort(ip, strconv.Itoa(local)), time.Second); err == nil {
		t.Error("the port not marked lan is open on the LAN")
	}
	if got, err := socksUDP(net.JoinHostPort("127.0.0.1", strconv.Itoa(lan)), echo.LocalAddr().String(), []byte("ping")); err != nil || string(got) != "ping" {
		t.Errorf("UDP through the port: %q %v\n%s", got, err, m.logs.last(20))
	}

	// Outside allowLan's networks: refused, password or not.
	in.LanAllowedIPs = []string{"127.0.0.0/8"}
	apply()
	waitFor("a network not allowed is refused", func() bool { code, _ := get(ip, lan, "u"); return code != 200 })
	if code, err := get("127.0.0.1", lan, ""); code != 200 {
		t.Errorf("from this machine, with its network allowed: %d %v", code, err)
	}

	// allowLan off: this machine's alone.
	in.AllowLan, in.LanAllowedIPs = false, nil
	apply()
	waitFor("allowLan off closes it to the LAN", func() bool {
		c, err := net.DialTimeout("tcp", net.JoinHostPort(ip, strconv.Itoa(lan)), time.Second)
		if err == nil {
			c.Close()
		}
		return err != nil
	})
	if code, err := get("127.0.0.1", lan, ""); code != 200 {
		t.Errorf("allowLan off, from this machine: %d %v", code, err)
	}
}

// socksUDP sends one datagram to dst through the SOCKS5 server at proxy
// (no authentication) and returns the answer.
func socksUDP(proxy, dst string, data []byte) ([]byte, error) {
	c, err := net.DialTimeout("tcp", proxy, 2*time.Second)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.Write([]byte{5, 1, 0}); err != nil {
		return nil, err
	}
	hello := make([]byte, 2)
	if _, err := io.ReadFull(c, hello); err != nil || hello[1] != 0 {
		return nil, fmt.Errorf("greeting %v %v", hello, err)
	}
	if _, err := c.Write([]byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil { // UDP ASSOCIATE from anywhere
		return nil, err
	}
	head := make([]byte, 4)
	if _, err := io.ReadFull(c, head); err != nil || head[1] != 0 {
		return nil, fmt.Errorf("associate %v %v", head, err)
	}
	var addr []byte
	switch head[3] {
	case 1:
		addr = make([]byte, 4)
	case 4:
		addr = make([]byte, 16)
	default:
		return nil, fmt.Errorf("relay address type %d", head[3])
	}
	port := make([]byte, 2)
	if _, err := io.ReadFull(c, addr); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(c, port); err != nil {
		return nil, err
	}
	relay := &net.UDPAddr{IP: net.IP(addr), Port: int(port[0])<<8 | int(port[1])}
	if relay.IP.IsUnspecified() {
		relay.IP = net.IPv4(127, 0, 0, 1)
	}
	u, err := net.DialUDP("udp", nil, relay)
	if err != nil {
		return nil, err
	}
	defer u.Close()
	to, err := net.ResolveUDPAddr("udp", dst)
	if err != nil {
		return nil, err
	}
	pkt := append([]byte{0, 0, 0, 1}, to.IP.To4()...)
	pkt = append(pkt, byte(to.Port>>8), byte(to.Port))
	u.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := u.Write(append(pkt, data...)); err != nil {
		return nil, err
	}
	buf := make([]byte, 1500)
	n, err := u.Read(buf)
	if err != nil {
		return nil, err
	}
	if n < len(pkt) || !bytes.Equal(buf[:4], pkt[:4]) {
		return nil, errors.New("not a SOCKS5 datagram")
	}
	return buf[len(pkt):n], nil
}
