// Package locate finds where a node really exits: from a probe through the
// node (Cloudflare's /cdn-cgi/trace) when there is one, else from the node's
// name.
package locate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oboo/moho/internal/geo"
	"github.com/oboo/moho/internal/mihomo"
)

// ProbeGroup is the selector the probe listener is bound to (core.ProbeGroup;
// repeated here so locate does not depend on core).
const ProbeGroup = "MOHO-PROBE"

// Direct is the node that measures the user's own location.
const Direct = "DIRECT"

// Sources of a Result.
const (
	SourceProbe   = "probe"
	SourceName    = "name"
	SourceUnknown = "unknown"
)

const (
	traceURL     = "https://www.cloudflare.com/cdn-cgi/trace"
	probeTimeout = 8 * time.Second
	// maxAge is how long a probe is trusted by Guess.
	maxAge = 24 * time.Hour
)

// Result is where a node exits, and how Moho knows.
type Result struct {
	Place  *geo.Place `json:"place,omitempty"`
	Source string     `json:"source"` // "probe", "name", "unknown"
	IP     string     `json:"ip,omitempty"`
	At     time.Time  `json:"at"`
}

// Locator caches probes by node and keeps them in a JSON file.
type Locator struct {
	path  string
	now   func() time.Time
	slot  chan struct{} // one probe at a time
	mu    sync.Mutex
	cache map[string]Result
	// named is geo.FromName by display name: a snapshot asks for every
	// node's, every 2 s, and names don't move.
	named map[string]namedPlace
	// Login, when set, gives the probe listener's user and password.
	Login func() *url.Userinfo
}

type namedPlace struct {
	place geo.Place
	ok    bool
}

// New returns a locator whose probes are kept in cachePath (none if empty),
// loading the ones already there.
func New(cachePath string) *Locator {
	l := &Locator{path: cachePath, now: time.Now, slot: make(chan struct{}, 1), cache: map[string]Result{}}
	if cachePath != "" {
		if b, err := os.ReadFile(cachePath); err == nil {
			_ = json.Unmarshal(b, &l.cache)
		}
		if l.cache == nil {
			l.cache = map[string]Result{}
		}
	}
	return l
}

// Guess answers without the network: a cached probe (at most 24 h old), else
// geo.FromName(display), else unknown.
func (l *Locator) Guess(node, display string) Result {
	now := l.now()
	l.mu.Lock()
	r, ok := l.cache[node]
	l.mu.Unlock()
	if ok && r.Place != nil && now.Sub(r.At) <= maxAge {
		return r
	}
	l.mu.Lock()
	n, seen := l.named[display]
	l.mu.Unlock()
	if !seen {
		n.place, n.ok = geo.FromName(display)
		l.mu.Lock()
		if l.named == nil || len(l.named) >= 4096 { // names that come and go
			l.named = map[string]namedPlace{}
		}
		l.named[display] = n
		l.mu.Unlock()
	}
	if n.ok {
		p := n.place
		return Result{Place: &p, Source: SourceName, At: now}
	}
	return Result{Source: SourceUnknown, At: now}
}

// Probe measures where node exits; node "DIRECT" measures the user's own
// location. It points the selector group MOHO-PROBE at node, then requests
// Cloudflare's trace through the probe listener 127.0.0.1:probePort and maps
// its loc and colo with geo.FromTrace. Probes run one at a time; results are
// cached by node and written to the cache file.
func (l *Locator) Probe(ctx context.Context, c *mihomo.Client, probePort int, node string) (Result, error) {
	select {
	case l.slot <- struct{}{}:
	case <-ctx.Done():
		return Result{Source: SourceUnknown, At: l.now()}, ctx.Err()
	}
	defer func() { <-l.slot }()

	if err := c.Select(ctx, ProbeGroup, node); err != nil {
		return Result{Source: SourceUnknown, At: l.now()}, fmt.Errorf("select %s: %w", node, err)
	}
	var login *url.Userinfo
	if l.Login != nil {
		login = l.Login()
	}
	body, err := fetchTrace(ctx, probePort, login)
	if err != nil {
		return Result{Source: SourceUnknown, At: l.now()}, err
	}
	return l.record(node, body)
}

// Direct measures where this device is by requesting the trace itself, for
// when no core is running to send it through. It is only right when nothing
// tunnels the device's own traffic: under a VPN it would find the VPN's exit.
// The result is cached as Probe("DIRECT") would be.
func (l *Locator) Direct(ctx context.Context) (Result, error) {
	select {
	case l.slot <- struct{}{}:
	case <-ctx.Done():
		return Result{Source: SourceUnknown, At: l.now()}, ctx.Err()
	}
	defer func() { <-l.slot }()
	body, err := fetchTrace(ctx, 0, nil)
	if err != nil {
		return Result{Source: SourceUnknown, At: l.now()}, err
	}
	return l.record("DIRECT", body)
}

// record reads a trace answer and caches what it says about node.
func (l *Locator) record(node, body string) (Result, error) {
	ip, loc, colo := parseTrace(body)
	if ip == "" && loc == "" {
		return Result{Source: SourceUnknown, At: l.now()}, errors.New("trace: no ip or loc in the answer")
	}
	r := Result{Source: SourceProbe, IP: ip, At: l.now()}
	if p, ok := geo.FromTrace(loc, colo); ok {
		r.Place = &p
	}
	l.mu.Lock()
	l.cache[node] = r
	err := l.saveLocked()
	l.mu.Unlock()
	return r, err
}

// fetchTrace requests the trace through the probe listener, on a connection
// of its own: a kept-alive one would still go through the previous node.
// A probePort of 0 requests it directly.
func fetchTrace(ctx context.Context, probePort int, login *url.Userinfo) (string, error) {
	tr := &http.Transport{DisableKeepAlives: true}
	if probePort != 0 {
		tr.Proxy = http.ProxyURL(&url.URL{Scheme: "http", User: login, Host: "127.0.0.1:" + strconv.Itoa(probePort)})
	}
	client := &http.Client{Timeout: probeTimeout, Transport: tr}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, traceURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("trace: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", fmt.Errorf("trace: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("trace: %s", resp.Status)
	}
	return string(b), nil
}

// parseTrace reads the key=value lines of a /cdn-cgi/trace answer.
func parseTrace(body string) (ip, loc, colo string) {
	for _, line := range strings.Split(body, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "ip":
			ip = v
		case "loc":
			loc = v
		case "colo":
			colo = v
		}
	}
	return ip, loc, colo
}

// saveLocked writes the cache file atomically; l.mu is held.
func (l *Locator) saveLocked() error {
	if l.path == "" {
		return nil
	}
	b, err := json.MarshalIndent(l.cache, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(l.path), ".locate-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(b)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), l.path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}
