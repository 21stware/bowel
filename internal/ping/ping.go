// Package ping measures how long reaching a node's server takes from this
// computer, without going through the node. It uses ICMP: a TCP handshake
// can be answered by a transparent proxy on the way (a router's, say),
// which reports a few milliseconds for a server across the ocean.
package ping

import "time"

// millis rounds up: 0 means no result.
func millis(d time.Duration) int {
	return max(1, int((d+time.Millisecond-1)/time.Millisecond))
}
