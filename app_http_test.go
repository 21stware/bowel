package main

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// deadAddr is a loopback address nothing listens on: what an earlier proxy
// app leaves in the system's settings once it is closed or removed.
func deadAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

// through is the app's download client with its proxy fixed to addr.
func through(scheme, addr string) *http.Client {
	return &http.Client{Transport: directWhenProxyIsDown(&http.Transport{Proxy: http.ProxyURL(&url.URL{Scheme: scheme, Host: addr})})}
}

func TestDownloadGoesDirectWhenTheProxyIsDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "nodes") }))
	defer srv.Close()
	for _, scheme := range []string{"http", "socks5"} {
		resp, err := through(scheme, deadAddr(t)).Get(srv.URL + "/sub")
		if err != nil {
			t.Errorf("%s proxy: %v", scheme, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if string(body) != "nodes" {
			t.Errorf("%s proxy: got %q", scheme, body)
		}
	}
}

func TestDownloadNamesTheProxyWhenDirectFailsToo(t *testing.T) {
	proxy, target := deadAddr(t), deadAddr(t)
	_, err := through("http", proxy).Get("http://" + target + "/sub")
	if err == nil {
		t.Fatal("no error")
	}
	// The proxy is what the user can fix; the direct failure is why it mattered.
	if !strings.Contains(err.Error(), proxy) || !strings.Contains(err.Error(), "dial tcp "+target) {
		t.Errorf("error %q should name the proxy %s and the direct failure to %s", err, proxy, target)
	}
}

// A proxy that is there decides: what it answers stands, and nothing goes
// around it.
func TestDownloadStaysWithAProxyThatAnswers(t *testing.T) {
	var direct atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { direct.Add(1) }))
	defer srv.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer proxy.Close()
	resp, err := through("http", proxy.Listener.Addr().String()).Get(srv.URL + "/sub")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway || direct.Load() != 0 {
		t.Errorf("status %d, %d direct requests: the proxy's answer should stand", resp.StatusCode, direct.Load())
	}
}
