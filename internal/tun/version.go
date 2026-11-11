package tun

import (
	"context"
	"strings"
	"time"

	"github.com/oboo/moho/internal/nowindow"
)

// Stale reports whether the root copy at installed is another version of
// the core than the one the app ships at bundled. It goes by the version
// mihomo states and not by the bytes, which change with every release's
// signature. When either cannot say, it is not stale: a root copy that
// works is not to be replaced on a guess. On macOS a copy of the same
// version is stale as well when it is signed under a name that keeps it
// from telling applications apart and the bundled core is not
// (signedApart).
func Stale(bundled, installed string) bool {
	a, b := version(bundled), version(installed)
	return a != "" && b != "" && a != b || signedApart(bundled, installed)
}

// version is what `mihomo -v` calls the core's version, "" when it cannot
// be told: "Mihomo Meta v1.19.33 darwin arm64 with go1.26.10 …".
func version(bin string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := nowindow.CommandContext(ctx, bin, "-v").Output()
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(out), "\n")
	f := strings.Fields(line)
	if len(f) < 3 || f[0] != "Mihomo" {
		return ""
	}
	return f[2]
}
