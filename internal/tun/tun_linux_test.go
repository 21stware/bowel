package tun

import (
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestInstalled(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "mihomo")
	if err := os.WriteFile(f, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() != 0 && Installed(f) {
		t.Error("a file of the user's counts as installed")
	}
	// The setuid bit alone is not enough: the owner must be root.
	if err := os.Chmod(f, 0o4755); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() != 0 && Installed(f) {
		t.Error("a setuid file of the user's counts as installed")
	}
	if Installed(filepath.Join(dir, "missing")) || Installed(dir) {
		t.Error("a missing file or a directory counts as installed")
	}
}

// vfsCaps builds a security.capability attribute of revision 2.
func vfsCaps(effective bool, caps ...int) []byte {
	b := make([]byte, 20)
	magic := uint32(0x02000000)
	if effective {
		magic |= 1
	}
	binary.LittleEndian.PutUint32(b, magic)
	for _, c := range caps {
		off := 4 + 8*(c/32)
		binary.LittleEndian.PutUint32(b[off:], binary.LittleEndian.Uint32(b[off:])|1<<(c%32))
	}
	return b
}

func TestCapsAllow(t *testing.T) {
	admin := unix.CAP_NET_ADMIN
	if !capsAllow(vfsCaps(true, admin, unix.CAP_NET_BIND_SERVICE, unix.CAP_NET_RAW), admin) {
		t.Error("what setcap writes doesn't count")
	}
	if capsAllow(vfsCaps(false, admin), admin) {
		t.Error("permitted but not effective counts")
	}
	if capsAllow(vfsCaps(true, unix.CAP_NET_RAW), admin) {
		t.Error("another capability counts")
	}
	if !capsAllow(vfsCaps(true, unix.CAP_BPF), unix.CAP_BPF) {
		t.Error("a capability of the second word doesn't count")
	}
	for _, b := range [][]byte{nil, {1, 0, 0, 2}, vfsCaps(true, admin)[:11]} {
		if capsAllow(b, admin) {
			t.Errorf("a short attribute counts: %v", b)
		}
	}
	// Revision 1 has one word: nothing of the second is in it.
	v1 := vfsCaps(true, admin)[:12]
	binary.LittleEndian.PutUint32(v1, 0x01000001)
	if !capsAllow(v1, admin) || capsAllow(v1, unix.CAP_BPF) {
		t.Error("revision 1 read wrong")
	}
}

// The copy becomes root's before it gets its rights (chown drops them),
// and takes its name last. With no setcap it is setuid instead.
func TestInstallStep(t *testing.T) {
	st := installStepWith("/usr/sbin/setcap", "/h/a b/.mihomo-1", "/h/a b/mihomo")
	got := script([]Step{st})
	for _, w := range []string{" root:root '/h/a b/.mihomo-1' && ", " 0755 '/h/a b/.mihomo-1' && /usr/sbin/setcap " + fileCaps + " '/h/a b/.mihomo-1' && ", " -f '/h/a b/.mihomo-1' '/h/a b/mihomo'"} {
		if !strings.Contains(got, w) {
			t.Errorf("script lacks %q:\n%s", w, got)
		}
	}
	if strings.Contains(got, "4755") {
		t.Errorf("setuid with setcap there:\n%s", got)
	}
	if i, j, k := strings.Index(got, "chown"), strings.Index(got, "setcap"), strings.Index(got, "mv"); !(i < j && j < k) {
		t.Errorf("order:\n%s", got)
	}
	if len(st)%2 != 0 || st[len(st)-1] != "/h/a b/mihomo" || st[1] != "/h/a b/.mihomo-1" {
		t.Errorf("step = %q", st)
	}
	suid := script([]Step{installStepWith("setcap", "/t", "/d")})
	if !strings.Contains(suid, " 4755 '/t'") || strings.Contains(suid, "setcap") {
		t.Errorf("without setcap:\n%s", suid)
	}
}

// Paths are one word each whatever is in them, several steps stop at the
// first failure, and sh accepts the whole (nothing runs: sh -n only reads).
func TestScript(t *testing.T) {
	s := script([]Step{{"/bin/echo ", `/a b/it's "x"\y`}, {"/bin/true ", "/c;d", " ", "$(e)"}})
	want := `/bin/echo '/a b/it'\''s "x"\y' && /bin/true '/c;d' '$(e)'`
	if s != want {
		t.Fatalf("script:\n got %s\nwant %s", s, want)
	}
	if out, err := exec.Command("/bin/sh", "-n", "-c", s).CombinedOutput(); err != nil {
		t.Errorf("sh -n: %v: %s", err, out)
	}
	out, err := exec.Command("/bin/sh", "-c", script([]Step{{"printf %s ", `it's $HOME "x"`}})).Output()
	if err != nil || string(out) != `it's $HOME "x"` {
		t.Errorf("quoting: %q, %v", out, err)
	}
	if err := Admin("p"); err != nil {
		t.Fatalf("Admin with no steps: %v", err)
	}
}

func TestAdminError(t *testing.T) {
	e := errors.New("exit status")
	if adminError(126, "Request dismissed", e) != ErrCanceled {
		t.Error("a dismissed dialog is not ErrCanceled")
	}
	if err := adminError(127, "", e); err == ErrCanceled || !strings.Contains(err.Error(), "exit status") {
		t.Errorf("127: %v", err)
	}
	if err := adminError(1, "chown: no", e); !strings.Contains(err.Error(), "chown: no") {
		t.Errorf("1: %v", err)
	}
}

// TestInstallStepKeepsCurrentCopy: until the authorization, the copy TUN
// runs on stays as it is, and the step only names a copy beside it.
func TestInstallStepKeepsCurrentCopy(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "new"), filepath.Join(dir, "bin", "mihomo")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	left := filepath.Join(dir, "bin", ".mihomo-left")
	for p, c := range map[string]string{src: "new", dst: "old", left: "x"} {
		if err := os.WriteFile(p, []byte(c), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if checkDir(filepath.Dir(dst)) != nil {
		t.Skip("the test's directory is mounted nosuid")
	}
	step, err := InstallStep(src, dst)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "old" {
		t.Errorf("dst = %q before the authorization", b)
	}
	if _, err := os.Stat(left); err == nil {
		t.Error("the copy of a refused authorization is still there")
	}
	if b, _ := os.ReadFile(step[1]); string(b) != "new" {
		t.Errorf("copy = %q", b)
	}
	if step[len(step)-1] != dst {
		t.Errorf("step = %v", step)
	}
}

// With BOWEL_TUN_REAL naming a directory, the real thing: a copy that
// root owns and setcap has marked (as installStep's commands leave it)
// counts as installed, and one of them alone does not. The directory must
// hold "caps" (root's, with fileCaps), "plain" (root's, no rights) and
// "user" (the user's).
func TestInstalledReal(t *testing.T) {
	dir := os.Getenv("BOWEL_TUN_REAL")
	if dir == "" {
		t.Skip("BOWEL_TUN_REAL is not set")
	}
	if !Installed(filepath.Join(dir, "caps")) {
		t.Error("root's copy with capabilities doesn't count as installed")
	}
	if Installed(filepath.Join(dir, "plain")) {
		t.Error("root's copy with no rights counts as installed")
	}
	if os.Geteuid() != 0 && Installed(filepath.Join(dir, "user")) {
		t.Error("the user's copy counts as installed")
	}
}
