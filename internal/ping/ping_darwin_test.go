package ping

import "testing"

func TestParseTime(t *testing.T) {
	out := []byte("PING 1.1.1.1 (1.1.1.1): 56 data bytes\n64 bytes from 1.1.1.1: icmp_seq=0 ttl=52 time=12.345 ms\n")
	if ms, err := parseTime(out); err != nil || ms != 13 {
		t.Fatalf("got %d, %v", ms, err)
	}
	if _, err := parseTime([]byte("Request timeout for icmp_seq 0\n")); err == nil {
		t.Fatal("a timeout parsed as a reply")
	}
}
