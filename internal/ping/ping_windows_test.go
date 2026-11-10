package ping

import (
	"context"
	"slices"
	"testing"
	"time"
)

func TestPingArgs(t *testing.T) {
	if got := pingArgs("1.1.1.1", "192.168.1.23", 2500*time.Millisecond); !slices.Equal(got, []string{"-n", "1", "-w", "2500", "-S", "192.168.1.23", "1.1.1.1"}) {
		t.Errorf("args %q", got)
	}
	if got := pingArgs("1.1.1.1", "", 0); !slices.Equal(got, []string{"-n", "1", "-w", "1", "1.1.1.1"}) {
		t.Errorf("args %q", got)
	}
}

// Windows' ping really answers for this machine itself.
func TestICMPLoopback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ms, err := ICMP(ctx, "127.0.0.1", "", time.Second)
	if err != nil || ms < 1 {
		t.Errorf("%d ms, %v", ms, err)
	}
	if _, err := ICMP(ctx, "::1", "", time.Second); err == nil {
		t.Error("an IPv6 address was taken")
	}
	if _, err := ICMP(ctx, "127.0.0.1", "no such adapter", time.Second); err == nil {
		t.Error("an adapter that doesn't exist was taken")
	}
}
