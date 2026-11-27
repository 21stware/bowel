package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

// throttledExit is exitTo with the way back held to bytesPerSec: a node
// that answers at once but downloads at that rate. Closing the returned
// switch makes it refuse every connection from then on, as a node that
// went down.
func throttledExit(t *testing.T, upstream string, bytesPerSec int) (port int, down *atomic.Bool) {
	t.Helper()
	down = new(atomic.Bool)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || down.Load() {
			http.Error(w, "no", http.StatusBadGateway)
			return
		}
		up, err := net.Dial("tcp", upstream)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer up.Close()
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection established\r\n\r\n")
		go func() { _, _ = io.Copy(up, conn) }()
		const chunk = 16 << 10
		buf := make([]byte, chunk)
		start, sent := time.Now(), 0
		for {
			n, err := up.Read(buf)
			if n > 0 {
				if _, werr := conn.Write(buf[:n]); werr != nil {
					return
				}
				sent += n
				if ahead := time.Duration(float64(sent)/float64(bytesPerSec)*float64(time.Second)) - time.Since(start); ahead > 0 {
					time.Sleep(ahead)
				}
			}
			if err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	_, p, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port, _ = strconv.Atoi(p)
	return port, down
}

// TestDownloadPickEndToEnd runs a scheduled download test on the real core
// (issue 267): of three nodes that answer as fast but download at 1, 3 and
// 8 MB/s, and one that answers nothing, the group is put on the 8 MB/s
// one, the pick is kept on disk, and when that node goes down the group is
// back on its automatic choice.
func TestDownloadPickEndToEnd(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/204" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		buf := make([]byte, 64<<10)
		for {
			if _, err := w.Write(buf); err != nil {
				return
			}
		}
	}))
	t.Cleanup(site.Close)
	_, sitePort, _ := net.SplitHostPort(site.Listener.Addr().String())
	base := "http://dl.bowel.test:" + sitePort
	a, _ := routingApp(t, func(s *settings.Settings) {
		s.Hosts = []string{"dl.bowel.test=127.0.0.1"}
		s.DevOff = true
		s.DownloadURL = base + "/down"
		s.RouteTargets = map[string][]routes.Target{"media": {{Name: "Local", URL: base + "/204"}}}
	})
	ctx := context.Background()

	nowhere, err := core.FreePort()
	if err != nil {
		t.Fatal(err)
	}
	upstream := site.Listener.Addr().String()
	slowPort, _ := throttledExit(t, upstream, 1<<20)
	midPort, _ := throttledExit(t, upstream, 3<<20)
	fastPort, fastDown := throttledExit(t, upstream, 8<<20)
	sub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "proxies:\n"+
			"  - {name: Slow, type: http, server: 127.0.0.1, port: "+strconv.Itoa(slowPort)+"}\n"+
			"  - {name: Mid, type: http, server: 127.0.0.1, port: "+strconv.Itoa(midPort)+"}\n"+
			"  - {name: Fast, type: http, server: 127.0.0.1, port: "+strconv.Itoa(fastPort)+"}\n"+
			"  - {name: Dead, type: http, server: 127.0.0.1, port: "+strconv.Itoa(nowhere)+"}\n")
	}))
	t.Cleanup(sub.Close)
	pv, err := a.addProvider(ctx, sub.URL+"/dl")
	if err != nil {
		t.Fatal(err)
	}
	fast := "Fast" + core.NodeSep + pv.ID
	// The range is this subscription's nodes: the fixture's own go direct,
	// at the speed of the loopback. Rounds run with takeover off too, so the
	// refreshes would start one of their own beside the test's: as tried
	// just now, they leave it to the test.
	a.downloadTriedNow("media")
	if err := a.updateSettings(func(s *settings.Settings) {
		s.Routing.ChangePolicy("media", func(q *routes.Policy) {
			q.Mode, q.Providers, q.DownloadEvery = routes.ModeAuto, []string{pv.ID}, 12
		})
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.testRoute(ctx, "media"); err != nil {
		t.Fatal(err)
	}
	a.refresh()

	plan := a.planRound("media")
	for _, n := range plan.Nodes {
		if providerOf(n) != pv.ID || display(n) == "Dead" {
			t.Errorf("the round would test %s", n)
		}
	}
	if len(plan.Nodes) != 3 {
		t.Fatalf("plan %v", plan.Nodes)
	}

	began := time.Now()
	a.scheduledDownload("media")
	t.Logf("the round took %s", time.Since(began).Round(100*time.Millisecond))
	a.mu.Lock()
	pick, results := a.picks["media"], a.bandwidth
	a.mu.Unlock()
	for _, n := range plan.Nodes {
		r := results[n]
		t.Logf("%s: %.1f Mbps failed=%v", display(n), float64(r.bps)/1e6, r.failed)
	}
	if pick.Node != fast {
		t.Fatalf("picked %q, want %q", pick.Node, fast)
	}
	r := routeOf(t, a, "media")
	if now := coreProxies(t, a)[routes.Group(r)].Now; now != fast {
		t.Errorf("the group is on %q, want the pick", now)
	}
	if b, err := os.ReadFile(a.downloadPicksFile()); err != nil || !strings.Contains(string(b), "Fast") {
		t.Errorf("pick not kept on disk: %s %v", b, err)
	}
	for _, rv := range a.snapshot().Routes {
		if rv.ID == "media" && (rv.Download == nil || rv.Download.Node != fast || rv.Node != fast) {
			t.Errorf("the view: node %q, download %+v", rv.Node, rv.Download)
		}
	}

	// Not due again: nothing runs.
	if downloadDue(r, pick, true, time.Time{}, time.Now()) {
		t.Error("due again right after a round")
	}

	// 现在测一轮: a round by hand chooses as a scheduled one does, and the
	// next is due from it.
	hand := a.planBandwidth("media", plan.Nodes)
	if !hand.Round || hand.Cap != scheduledCap || len(hand.Nodes) != 3 {
		t.Fatalf("by hand: %+v", hand)
	}
	if err := a.testBandwidth(ctx, "media", hand); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	again := a.picks["media"]
	a.mu.Unlock()
	if again.Node != fast || !again.At.After(pick.At) {
		t.Fatalf("the round by hand: %+v, before %+v", again, pick)
	}
	pick = again

	// The pick goes down: back on the automatic group, which leaves it out.
	fastDown.Store(true)
	if err := a.testRoute(ctx, "media"); err != nil {
		t.Fatal(err)
	}
	a.refresh() // and one more: a select the core dropped is sent again
	a.refresh()
	if now := coreProxies(t, a)[routes.Group(r)].Now; now != routes.Auto(r) {
		t.Errorf("with the pick down the group is on %q", now)
	}
	a.mu.Lock()
	after := a.picks["media"]
	a.mu.Unlock()
	if after.Node != "" || !after.At.Equal(pick.At) {
		t.Errorf("the pick after it went down: %+v", after)
	}
}
