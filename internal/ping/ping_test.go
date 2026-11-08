package ping

import (
	"testing"
	"time"
)

func TestMillisNeverZero(t *testing.T) {
	if got := millis(100 * time.Microsecond); got != 1 {
		t.Fatalf("got %d", got)
	}
	if got := millis(12 * time.Millisecond); got != 12 {
		t.Fatalf("got %d", got)
	}
}
