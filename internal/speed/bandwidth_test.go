package speed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// paced serves chunk bytes every tick, n times.
func paced(chunk, n int, tick time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, chunk)
		for i := 0; i < n; i++ {
			if _, err := w.Write(b); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			select {
			case <-time.After(tick):
			case <-r.Context().Done():
				return
			}
		}
	}
}

func TestDownloadMeasuresPastTheWarmup(t *testing.T) {
	// 10 KB every 10 ms is 8 Mbit/s; a slow first 100 ms must not count.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, 100))
		w.(http.Flusher).Flush()
		time.Sleep(100 * time.Millisecond)
		paced(10_000, 40, 10*time.Millisecond)(w, r)
	}))
	defer srv.Close()
	got, err := download(context.Background(), srv.Client(), srv.URL, 5*time.Second, 1<<30, 120*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if mbps := float64(got.BitsPerSec) / 1e6; mbps < 5 || mbps > 11 {
		t.Errorf("rate %.1f Mbit/s, want about 8", mbps)
	}
	if got.Bytes != 100+400_000 {
		t.Errorf("bytes %d", got.Bytes)
	}
}

func TestDownloadStopsAtTheCapAndTheTime(t *testing.T) {
	srv := httptest.NewServer(paced(10_000, 1000, 5*time.Millisecond))
	defer srv.Close()
	got, err := download(context.Background(), srv.Client(), srv.URL, 5*time.Second, 55_000, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if got.Bytes != 55_000 {
		t.Errorf("cap: read %d bytes", got.Bytes)
	}
	start := time.Now()
	got, err = download(context.Background(), srv.Client(), srv.URL, 150*time.Millisecond, 1<<30, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("time limit: ran %v", d)
	}
	if got.BitsPerSec <= 0 {
		t.Errorf("no rate: %+v", got)
	}
}

func TestDownloadShortIsMeasuredWhole(t *testing.T) {
	srv := httptest.NewServer(paced(10_000, 5, 10*time.Millisecond))
	defer srv.Close()
	got, err := download(context.Background(), srv.Client(), srv.URL, 5*time.Second, 1<<30, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got.BitsPerSec <= 0 || got.Bytes != 50_000 {
		t.Errorf("%+v", got)
	}
}

func TestDownloadFailures(t *testing.T) {
	notFound := httptest.NewServer(http.NotFoundHandler())
	defer notFound.Close()
	if _, err := download(context.Background(), notFound.Client(), notFound.URL, time.Second, 1<<20, 10*time.Millisecond); err == nil {
		t.Error("a 404 was measured")
	}
	empty := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer empty.Close()
	if _, err := download(context.Background(), empty.Client(), empty.URL, time.Second, 1<<20, 10*time.Millisecond); err == nil {
		t.Error("an empty answer was measured")
	}
}
