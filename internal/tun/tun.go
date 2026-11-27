// Package tun installs the core so that it can open a TUN device, which
// takes rights a user's process does not have. Each system gives them its
// own way:
//
//   - macOS: a copy of the core in a user directory, made setuid root (as
//     Clash Verge does), after an administrator's authorisation asked once.
//   - Linux: the same copy, owned by root and given the file capabilities
//     TUN needs (CAP_NET_ADMIN and two more), through pkexec; setuid root
//     where setcap is missing. Asked once as well.
//   - Windows: a copy only administrators can write, started elevated
//     through a UAC prompt each time the core starts (see Spawn): an
//     unsigned program that registers a task to run elevated without
//     asking is what Defender blocks as malware.
package tun

import (
	"io"
	"os"
	"path/filepath"
	"sync/atomic"

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

// CoreDir is the directory the core runs in (its -d; the config is
// config.yaml inside). Windows passes both to the launcher that starts
// the elevated core, which gets no environment from the app (an elevated
// process gets the user's own), so every file its config names has to be
// inside this directory: mihomo reads nothing outside it without a
// SAFE_PATHS variable. The other systems do not read it. Set once, before
// anything else is called.
var CoreDir string

// OnPrompt, if set, is told when Windows is about to show the UAC prompt
// of a start of TUN's core (Spawn). It returns the window the prompt is to
// belong to, 0 for none, and what to call once the prompt is answered. A
// prompt asked for with no window of the app's in front goes to the
// taskbar, where it goes unseen at a start with the window hidden, and
// Windows cancels it unanswered after about two minutes: the app brings
// its window forward with this, and says what it waits for. The other
// systems do not call it. Set once, before anything else is called.
var OnPrompt func() (owner uintptr, answered func())

// device is the TUN device's name in the settings, "" for the core's own
// choice.
var device atomic.Value

// SetDevice tells the package what the settings call the TUN device. Linux
// names it in the polkit rule an installation leaves; the other systems
// do not read it.
func SetDevice(name string) { device.Store(name) }

// Step is one shell command run as root (on Windows, elevated): its parts
// alternate between literal text and paths, which the shell sees as one
// word each.
type Step []string

// Prompt is what the administrator dialog says for TUN alone, where the
// system lets the dialog say anything (macOS).
func Prompt() string {
	return i18n.T("bowel 需要授权以启用 TUN 模式", "bowel needs authorization to turn on TUN mode")
}

// Install copies src to where dst is installed from, then asks for an
// administrator's authorisation to make it the core TUN runs on. It
// returns ErrCanceled when the user canceled, and dst is as it was.
//
// Where the system asks at each start (AsksEachStart) there is nothing
// to do here: that start makes the copy under its own authorisation.
func Install(src, dst string) error {
	if AsksEachStart {
		return nil
	}
	step, err := InstallStep(src, dst)
	if err != nil {
		return err
	}
	if err := Admin(Prompt(), step); err != nil {
		_ = os.Remove(step[1]) // the copy; dst is as it was
		return err
	}
	return nil
}

// InstallStep copies src beside dst as Install does and returns what is
// left to do as root, for Admin to run together with other steps under
// one authorisation. The copy is the step's first path.
func InstallStep(src, dst string) (Step, error) {
	dir := stageDir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := checkDir(dir); err != nil {
		return nil, err
	}
	// What an authorization that was refused left behind.
	if old, err := filepath.Glob(filepath.Join(dir, ".mihomo-*")); err == nil {
		for _, p := range old {
			_ = os.Remove(p)
		}
	}
	// A copy that becomes dst only once it is root's: dst may be the copy
	// TUN runs on now, and must outlive a refused authorization.
	tmp, err := copyTemp(src, dir)
	if err != nil {
		return nil, err
	}
	step, err := installStep(tmp, dst)
	if err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}
	return step, nil
}

// Admin runs the steps, one after the other and stopping at the first
// that fails, as root under a single administrator authorisation; the
// dialog shows prompt where it can. It returns ErrCanceled when the user
// canceled.
func Admin(prompt string, steps ...Step) error {
	if len(steps) == 0 {
		return nil
	}
	return runAdmin(prompt, steps)
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
