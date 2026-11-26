// Package geodata reads which lists the core's GeoSite and GeoIP data
// hold, so that a rule naming one can be checked before the core sees it:
// mihomo refuses a whole config over one GEOSITE or GEOIP rule whose list
// isn't there (config.parseRules, v1.19.33).
package geodata

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/oboo/moho/internal/routes"
)

// The files the core reads, next to its config.
const (
	SiteFile = "GeoSite.dat"
	IPFile   = "GeoIP.dat"
)

// Use makes rules check GeoSite and GeoIP lists against the data in dir,
// the core's (routes.GeoLists), and reads it in the background, so that
// the first rule typed doesn't wait for it.
func Use(dir string) {
	routes.GeoLists = func(kind string) ([]string, bool) {
		f := SiteFile
		if kind == routes.KindGeoIP {
			f = IPFile
		}
		names, err := Lists(filepath.Join(dir, f))
		return names, err == nil
	}
	go func() {
		for _, f := range []string{SiteFile, IPFile} {
			_, _ = Lists(filepath.Join(dir, f))
		}
	}()
}

type cached struct {
	size  int64
	mod   time.Time
	names []string
}

var (
	mu    sync.Mutex
	cache = map[string]cached{}
)

// Lists are the names of the lists in a GeoSite.dat or a GeoIP.dat, in
// lower case, as the core looks them up, and sorted. A file is read again
// only when it changes.
func Lists(path string) ([]string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	c, ok := cache[path]
	mu.Unlock()
	if ok && c.size == fi.Size() && c.mod.Equal(fi.ModTime()) {
		return c.names, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	names, err := parse(data)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	cache[path] = cached{fi.Size(), fi.ModTime(), names}
	mu.Unlock()
	return names, nil
}

var errFormat = errors.New("geodata: not a GeoSite or GeoIP file")

// parse reads the names out of a GeoSiteList or a GeoIPList, which are
// protocol buffers of the same shape: entries (field 1), each with its
// name first (field 1, country_code) and then its domains or ranges, which
// are skipped.
func parse(data []byte) ([]string, error) {
	var names []string
	for len(data) > 0 {
		field, wire, n := tag(data)
		if n <= 0 {
			return nil, errFormat
		}
		data = data[n:]
		if wire != 2 {
			if n = skip(data, wire); n < 0 {
				return nil, errFormat
			}
			data = data[n:]
			continue
		}
		size, n := binary.Uvarint(data)
		if n <= 0 || size > uint64(len(data)-n) {
			return nil, errFormat
		}
		entry := data[n : n+int(size)]
		data = data[n+int(size):]
		if field != 1 {
			continue
		}
		if name, ok := nameOf(entry); ok {
			names = append(names, strings.ToLower(name))
		}
	}
	slices.Sort(names)
	return slices.Compact(names), nil
}

// nameOf is an entry's field 1.
func nameOf(entry []byte) (string, bool) {
	for len(entry) > 0 {
		field, wire, n := tag(entry)
		if n <= 0 {
			return "", false
		}
		entry = entry[n:]
		if wire != 2 {
			if n = skip(entry, wire); n < 0 {
				return "", false
			}
			entry = entry[n:]
			continue
		}
		size, n := binary.Uvarint(entry)
		if n <= 0 || size > uint64(len(entry)-n) {
			return "", false
		}
		if field == 1 {
			return string(entry[n : n+int(size)]), true
		}
		entry = entry[n+int(size):]
	}
	return "", false
}

func tag(b []byte) (field uint64, wire int, n int) {
	v, n := binary.Uvarint(b)
	return v >> 3, int(v & 7), n
}

// skip is the length of a value of wire type wire other than 2, or -1.
func skip(b []byte, wire int) int {
	switch wire {
	case 0:
		if _, n := binary.Uvarint(b); n > 0 {
			return n
		}
	case 1:
		if len(b) >= 8 {
			return 8
		}
	case 5:
		if len(b) >= 4 {
			return 4
		}
	}
	return -1
}
