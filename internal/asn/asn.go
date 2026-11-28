// Package asn says which network an IP address belongs to: its autonomous
// system's number and the organization that runs it, from a GeoLite2-ASN
// database (the one mihomo reads for IP-ASN rules). The database is
// downloaded the first time it is asked for, in the background, so a lookup
// never waits on the network: until it is there, lookups find nothing.
package asn

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang"
)

// DefaultURL is where the database is downloaded from: MetaCubeX's copy of
// GeoLite2-ASN, through the same mirror as the other geo files.
const DefaultURL = "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/GeoLite2-ASN.mmdb"

const (
	maxSize = 64 << 20
	// refresh is how old the file may get before a newer one is fetched;
	// the old one is used meanwhile.
	refresh = 30 * 24 * time.Hour
	// retry is how long a failed download waits before the next.
	retry        = time.Hour
	fetchTimeout = 5 * time.Minute
	maxMemo      = 4096
)

// Info is an address's network.
type Info struct {
	Number uint32 // e.g. 13335
	Org    string // e.g. "Cloudflare, Inc."
}

// DB looks addresses up in the database at its path, downloading it when it
// is missing or old.
type DB struct {
	path, url string
	client    *http.Client
	now       func() time.Time
	// OnLoad, when set, is called after a database is loaded from a
	// download, so that what was shown without it can be shown again.
	OnLoad func()

	mu       sync.Mutex
	r        *maxminddb.Reader
	opened   bool // the file was tried
	fetching bool
	tried    time.Time // last download
	memo     map[string]Info
}

// New returns a database kept at path, downloaded with client (the default
// client if nil) from DefaultURL.
func New(path string, client *http.Client) *DB {
	if client == nil {
		client = http.DefaultClient
	}
	return &DB{path: path, url: DefaultURL, client: client, now: time.Now, memo: map[string]Info{}}
}

// Lookup returns ip's network, and false when it isn't known: a private or
// malformed address, one the database doesn't have, or no database yet.
func (d *DB) Lookup(ip string) (Info, bool) {
	addr := net.ParseIP(ip)
	if d == nil || addr == nil || addr.IsPrivate() || addr.IsLoopback() || addr.IsUnspecified() {
		return Info{}, false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if v, ok := d.memo[ip]; ok {
		return v, v.Number != 0
	}
	d.loadLocked()
	if d.r == nil {
		return Info{}, false
	}
	var rec struct {
		Number uint32 `maxminddb:"autonomous_system_number"`
		Org    string `maxminddb:"autonomous_system_organization"`
	}
	if err := d.r.Lookup(addr, &rec); err != nil {
		rec.Number, rec.Org = 0, ""
	}
	v := Info{Number: rec.Number, Org: strings.TrimSpace(rec.Org)}
	if len(d.memo) >= maxMemo {
		d.memo = map[string]Info{}
	}
	d.memo[ip] = v
	return v, v.Number != 0
}

// loadLocked opens the file the first time, and starts a download when it
// is missing or old. d.mu is held.
func (d *DB) loadLocked() {
	if !d.opened {
		d.opened = true
		if b, err := os.ReadFile(d.path); err == nil {
			if r, err := open(b); err == nil {
				d.r = r
			}
		}
	}
	stale := d.r == nil
	if !stale {
		if fi, err := os.Stat(d.path); err != nil || d.now().Sub(fi.ModTime()) > refresh {
			stale = true
		}
	}
	if stale && !d.fetching && (d.tried.IsZero() || d.now().Sub(d.tried) >= retry) {
		d.fetching, d.tried = true, d.now()
		go d.fetch()
	}
}

func (d *DB) fetch() {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	r, err := d.download(ctx)
	d.mu.Lock()
	d.fetching = false
	if err == nil {
		d.r, d.memo = r, map[string]Info{}
	}
	cb := d.OnLoad
	d.mu.Unlock()
	if err == nil && cb != nil {
		cb()
	}
}

// download fetches the database, checks that it is one, and puts it at
// d.path in place of the old one.
func (d *DB) download(ctx context.Context) (*maxminddb.Reader, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("asn: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("asn: %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, fmt.Errorf("asn: %w", err)
	}
	if len(b) > maxSize {
		return nil, errors.New("asn: database too large")
	}
	r, err := open(b)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(d.path), 0o755); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(d.path), ".asn-*")
	if err != nil {
		return nil, err
	}
	_, err = tmp.Write(b)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), d.path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return nil, err
	}
	return r, nil
}

// open reads a database from b, which it must be an ASN one.
func open(b []byte) (*maxminddb.Reader, error) {
	r, err := maxminddb.FromBytes(b)
	if err != nil {
		return nil, fmt.Errorf("asn: %w", err)
	}
	if !strings.Contains(r.Metadata.DatabaseType, "ASN") {
		return nil, fmt.Errorf("asn: %q is not an ASN database", r.Metadata.DatabaseType)
	}
	return r, nil
}
