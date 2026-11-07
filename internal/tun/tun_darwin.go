// Package tun installs the core so that it can open a TUN device: mihomo
// needs root for that, so Moho keeps a copy of the core in a user directory
// made setuid root after an administrator's authorisation (as Clash Verge
// does).
package tun

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/oboo/moho/internal/i18n"
)

// ErrCanceled is returned by Install and Uninstall when the user dismissed
// the administrator prompt.
var ErrCanceled error = canceled{}

// canceled says it in the interface's language when it is said.
type canceled struct{}

func (canceled) Error() string {
	return i18n.T("用户取消了授权", "The authorization was canceled")
}

// prompt is shown in the administrator authorisation dialog.
const prompt = "bowel 需要授权以启用 TUN 模式"

// Installed reports whether path is a regular file owned by root with the
// setuid bit.
func Installed(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Mode()&fs.ModeSetuid == 0 {
		return false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && st.Uid == 0
}

// Install copies src to dst (creating dst's directory), then asks for an
// administrator's authorisation through osascript to chown root:admin and
// chmod 4755 it. It returns ErrCanceled when the user canceled.
func Install(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	// Copy beside dst and rename over it: dst may be an older root-owned
	// copy, which cannot be written to but can be replaced, the directory
	// being the user's.
	tmp, err := copyTemp(src, filepath.Dir(dst))
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return runAdmin(installScript(dst))
}

// Uninstall removes dst. A root-owned file in a directory of the user's can
// be removed without privileges; otherwise it asks for an administrator's
// authorisation. A missing dst is not an error.
func Uninstall(dst string) error {
	err := os.Remove(dst)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if !errors.Is(err, fs.ErrPermission) {
		return err
	}
	return runAdmin(uninstallScript(dst))
}

func copyTemp(src, dir string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.CreateTemp(dir, ".mihomo-*")
	if err != nil {
		return "", err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(out.Name(), 0o755)
	}
	if err != nil {
		_ = os.Remove(out.Name())
		return "", err
	}
	return out.Name(), nil
}

// appleString quotes s as an AppleScript string literal.
func appleString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}

// shellExpr builds an AppleScript expression for a shell command: literal
// parts are AppleScript strings, paths go through `quoted form of` so the
// shell sees each as one word.
func shellExpr(parts ...string) string {
	var b []string
	for i, p := range parts {
		if i%2 == 0 {
			b = append(b, appleString(p))
		} else {
			b = append(b, "quoted form of "+appleString(p))
		}
	}
	return strings.Join(b, " & ")
}

func adminScript(expr string) string {
	return "do shell script " + expr + " with prompt " + appleString(i18n.T(prompt, "bowel needs authorization to turn on TUN mode")) + " with administrator privileges"
}

func installScript(dst string) string {
	return adminScript(shellExpr("/usr/sbin/chown root:admin ", dst, " && /bin/chmod 4755 ", dst))
}

func uninstallScript(dst string) string {
	return adminScript(shellExpr("/bin/rm -f ", dst))
}

func runAdmin(script string) error {
	out, err := exec.Command("/usr/bin/osascript", "-e", script).CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if strings.Contains(msg, "-128") || strings.Contains(msg, "User canceled") {
		return ErrCanceled
	}
	if msg == "" {
		msg = err.Error()
	}
	return fmt.Errorf(i18n.T("授权失败：%s", "Authorization failed: %s"), msg)
}
