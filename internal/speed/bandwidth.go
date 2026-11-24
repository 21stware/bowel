package speed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// DownloadTime is how long a download test runs at most. A fixed size
	// would end in under a second on a fast node, before its rate settles.
	DownloadTime = 5 * time.Second
	// DownloadCap is the most a test downloads, whatever the time: it bounds
	// what one test can cost the user's traffic allowance.
	DownloadCap = 50 << 20
	// DownloadWarmup is left out of the rate: the connection's first moments
	// are its slowest, while TCP finds the link's speed.
	DownloadWarmup = 500 * time.Millisecond
)

// Throughput is how fast a download went once under way.
type Throughput struct {
	BitsPerSec int64 // the sustained rate, past the warm-up
	Bytes      int64 // everything received: what the test cost
}

// Download fetches url with client for up to limit, or until it has capBytes,
// and measures the rate after DownloadWarmup from the first byte. A download
// that ends before the warm-up is over is measured whole. The client should
// open a connection of its own: a kept-alive one may go out another way.
func Download(ctx context.Context, client *http.Client, url string, limit time.Duration, capBytes int64) (Throughput, error) {
	return download(ctx, client, url, limit, capBytes, DownloadWarmup)
}

func download(ctx context.Context, client *http.Client, url string, limit time.Duration, capBytes int64, warmup time.Duration) (Throughput, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Throughput{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Throughput{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Throughput{}, fmt.Errorf("download: %s", resp.Status)
	}
	buf := make([]byte, 64<<10)
	var total, warm int64 // warm: the bytes in hand when the warm-up ended
	var first, warmAt, last time.Time
	for total < capBytes {
		n, err := resp.Body.Read(buf[:min(int64(len(buf)), capBytes-total)])
		t := time.Now()
		if n > 0 {
			if first.IsZero() {
				first = t
			}
			total += int64(n)
			last = t
			if warmAt.IsZero() && t.Sub(first) >= warmup {
				warm, warmAt = total, t
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			if total == 0 {
				return Throughput{}, err
			}
			break // what came before the break still says how fast it went
		}
		if !first.IsZero() && t.Sub(first) >= limit {
			break
		}
	}
	if total == 0 {
		return Throughput{}, errors.New("download: nothing received")
	}
	out := Throughput{Bytes: total}
	// The rate past the warm-up, when enough of the test came after it to
	// count; else the whole download from its first byte.
	if !warmAt.IsZero() && last.Sub(warmAt) >= warmup {
		out.BitsPerSec = rate(total-warm, last.Sub(warmAt))
	} else if d := last.Sub(first); d > 0 {
		out.BitsPerSec = rate(total, d)
	} else {
		return Throughput{Bytes: total}, errors.New("download: too short to time")
	}
	return out, nil
}

func rate(bytes int64, d time.Duration) int64 {
	return int64(float64(bytes) * 8 / d.Seconds())
}
