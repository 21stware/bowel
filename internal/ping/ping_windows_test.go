package ping

import (
	"context"
	"errors"
	"net"
	"slices"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
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

// A source address goes to the API as IPAddr: network byte order in a
// machine word, which is what the address' bytes read as they are stored.
func TestEchoAddr(t *testing.T) {
	if got := addr(nil); got != 0 {
		t.Errorf("no source = %#x, want 0 (Windows chooses)", got)
	}
	if got := addr(net.ParseIP("127.0.0.1")); got != 0x0100007f {
		t.Errorf("127.0.0.1 = %#x, want 0x0100007f", got)
	}
	if got := addr(net.ParseIP("192.168.10.183")); got != 0xb70aa8c0 {
		t.Errorf("192.168.10.183 = %#x, want 0xb70aa8c0", got)
	}
	if got := addr(net.ParseIP("::1")); got != 0 {
		t.Errorf("an IPv6 source = %#x, want 0", got)
	}
}

// The reply is Windows' ICMP_ECHO_REPLY, not a 32-bit reading of it:
// OptionsData is a pointer, so it is 40 bytes where pointers are 8 and 28
// where they are 4. Status and RoundTripTime keep their places either way.
func TestEchoReplyLayout(t *testing.T) {
	want := uintptr(28)
	if unsafe.Sizeof(uintptr(0)) == 8 {
		want = 40
	}
	if got := unsafe.Sizeof(icmpEchoReply{}); got != want {
		t.Errorf("sizeof(ICMP_ECHO_REPLY) = %d, want %d", got, want)
	}
}

// A call that answered nothing says which kind of nothing it was: a timeout
// is a reply that never came, not an API that cannot be used.
func TestEchoError(t *testing.T) {
	if err := echoError(syscall.Errno(ipReqTimedOut)); err == nil || err.Error() != "ping: no reply" {
		t.Errorf("a timeout: %v", err)
	}
	if err := echoError(windows.ERROR_NOT_SUPPORTED); !errors.Is(err, errNoIPHelper) {
		t.Errorf("the API saying no: %v", err)
	}
	if err := echoError(windows.ERROR_INVALID_FUNCTION); !errors.Is(err, errNoIPHelper) {
		t.Errorf("the API not knowing the call: %v", err)
	}
	if err := echoError(syscall.Errno(windows.ERROR_ACCESS_DENIED)); errors.Is(err, errNoIPHelper) || err == nil {
		t.Errorf("another failure: %v", err)
	}
}

// An echo no one may answer is over at its deadline, not later, and says so
// rather than reporting no reply.
func TestICMPDeadline(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if _, err := ICMP(ctx, "127.0.0.1", "", time.Second); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("a deadline already past: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if _, err := ICMP(ctx, "127.0.0.1", "", time.Second); !errors.Is(err, context.Canceled) {
		t.Errorf("a context already cancelled: %v", err)
	}
}
