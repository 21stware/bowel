package tun

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstalled(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "mihomo")
	if err := os.WriteFile(f, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if Installed(f) {
		t.Error("a file of the user's counts as installed")
	}
	// The setuid bit alone is not enough: the owner must be root.
	if err := os.Chmod(f, 0o4755); err != nil {
		t.Fatal(err)
	}
	if Installed(f) {
		t.Error("a setuid file of the user's counts as installed")
	}
	if Installed(filepath.Join(dir, "missing")) || Installed(dir) {
		t.Error("a missing file or a directory counts as installed")
	}
}

func TestScripts(t *testing.T) {
	dst := `/Users/a b/it's "x"\y/mihomo`
	got := installScript(dst)
	q := `quoted form of "/Users/a b/it's \"x\"\\y/mihomo"`
	want := `do shell script "/usr/sbin/chown root:admin " & ` + q + ` & " && /bin/chmod 4755 " & ` + q +
		` with prompt "bowel 需要授权以启用 TUN 模式" with administrator privileges`
	if got != want {
		t.Fatalf("installScript:\n got %s\nwant %s", got, want)
	}
	un := uninstallScript(dst)
	if !strings.HasPrefix(un, `do shell script "/bin/rm -f " & `+q+" with prompt") {
		t.Fatalf("uninstallScript: %s", un)
	}
	// The scripts compile (osacompile only compiles; nothing runs).
	if _, err := exec.LookPath("osacompile"); err != nil {
		t.Skip("no osacompile")
	}
	for _, s := range []string{got, un} {
		out := filepath.Join(t.TempDir(), "s.scpt")
		if b, err := exec.Command("osacompile", "-o", out, "-e", s).CombinedOutput(); err != nil {
			t.Errorf("osacompile: %v: %s\n%s", err, b, s)
		}
	}
}

// Several steps run under one authorisation, joined by &&, each path
// quoted on its own; the script compiles (nothing runs).
func TestAdminScriptSteps(t *testing.T) {
	s := adminScript("p", installStep("/a b/mihomo"), Step{"/bin/echo ", "/c'd"})
	want := `do shell script "/usr/sbin/chown root:admin " & quoted form of "/a b/mihomo" & " && /bin/chmod 4755 " & quoted form of "/a b/mihomo" & " && " & "/bin/echo " & quoted form of "/c'd" with prompt "p" with administrator privileges`
	if s != want {
		t.Fatalf("adminScript:\n got %s\nwant %s", s, want)
	}
	if err := Admin("p"); err != nil {
		t.Fatalf("Admin with no steps: %v", err)
	}
	if _, err := exec.LookPath("osacompile"); err != nil {
		t.Skip("no osacompile")
	}
	out := filepath.Join(t.TempDir(), "s.scpt")
	if b, err := exec.Command("osacompile", "-o", out, "-e", s).CombinedOutput(); err != nil {
		t.Errorf("osacompile: %v: %s", err, b)
	}
}
