package tun

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// signed copies this test program to a file signed ad hoc as id.
func signed(t *testing.T, id string) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "mihomo")
	if err := os.WriteFile(p, b, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("/usr/bin/codesign", "--force", "--sign", "-", "-i", id, p).CombinedOutput(); err != nil {
		t.Skipf("codesign: %v\n%s", err, out)
	}
	return p
}

// TestSignedApart: a root copy signed as "mihomo", which a release before
// the core was renamed left, is to be replaced by a core signed with a
// dotted identifier; nothing else is, least of all a copy that cannot say.
func TestSignedApart(t *testing.T) {
	old, renamed, linker := signed(t, "mihomo"), signed(t, "dev.moho.app.core"), signed(t, "a.out")
	script := fakeCore(t, "Mihomo Meta v1.19.33 darwin arm64")
	for _, c := range []struct {
		name               string
		bundled, installed string
		want               bool
	}{
		{"old copy, renamed core", renamed, old, true},
		{"old copy, upstream's core", linker, old, true},
		{"copy of the renamed core", renamed, renamed, false},
		{"copy of upstream's core", renamed, linker, false},
		{"both old", old, old, false},
		{"copy without a signature", renamed, script, false},
		{"no copy", renamed, filepath.Join(t.TempDir(), "none"), false},
	} {
		if got := signedApart(c.bundled, c.installed); got != c.want {
			t.Errorf("%s: %v", c.name, got)
		}
	}
	if id := identifier(renamed); id != "dev.moho.app.core" {
		t.Errorf("identifier %q", id)
	}
}
