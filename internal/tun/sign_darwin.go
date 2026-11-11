package tun

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// signedApart reports whether the installed copy is signed under an
// identifier without a dot while the bundled core has one with a dot.
// macOS 27 shows the kernel's socket list only to the latter (see
// core.BinName), so such a copy, which versions before the bundled core
// was renamed made of a release build, runs TUN but matches no rule for an
// application. A copy whose identifier cannot be read is left alone.
func signedApart(bundled, installed string) bool {
	a, b := identifier(bundled), identifier(installed)
	return strings.Contains(a, ".") && b != "" && !strings.Contains(b, ".")
}

// identifier is the code-signing identifier of the program at bin, "" when
// it has none or codesign cannot say.
func identifier(bin string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// codesign -d prints to standard error.
	out, _ := exec.CommandContext(ctx, "/usr/bin/codesign", "-d", "-v", bin).CombinedOutput()
	for _, line := range strings.Split(string(out), "\n") {
		if id, ok := strings.CutPrefix(line, "Identifier="); ok {
			return strings.TrimSpace(id)
		}
	}
	return ""
}
