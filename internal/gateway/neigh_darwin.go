package gateway

import (
	"context"
	"net/netip"
	"os/exec"
	"time"
)

func neighbours() map[netip.Addr]string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/sbin/arp", "-an").Output()
	if err != nil {
		return nil
	}
	return parseARP(string(out))
}
