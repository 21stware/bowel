package tun

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/oboo/moho/internal/i18n"
)

// On Windows a TUN adapter (Wintun's, which the core carries) can only be
// made by an elevated process, and nothing like a setuid bit lets a
// program be elevated without asking. Versions before this one asked once,
// and registered a scheduled task that ran the core elevated whenever the
// user started it. Defender took that for malware (see Spawn), so there is
// nothing to install ahead: each start of the core is elevated through a
// UAC prompt of its own, and makes on the way what it needs.
//
//   - A copy of the core, in a directory under %ProgramData% that only
//     administrators and SYSTEM can write, where no program of the user's
//     can replace it: the prompt never elevates a file the user could
//     have written. It is made, or brought up to date, by the same
//     elevated command that starts it, which then deletes the tasks
//     earlier versions registered.
//   - Beside it, a copy of the app's own program, the launcher, which
//     starts the core in a job that ends it with the launcher, and ends
//     it when the app asks or is gone (see Launch).
//
// The elevated command is hidden and returns an exit code alone, so it
// writes what each of its parts did to a log in the directory the copy
// was staged in, which is where a failure's reason comes from.

// AsksEachStart is whether the system asks for the authorization at each
// start of the core TUN runs on, rather than once: there is nothing to
// install before, and starting that core is what asks.
const AsksEachStart = true

// Path is where the copy of the core that TUN runs on is kept: a
// directory of this user's under %ProgramData%, closed to all but
// administrators by the command that makes it.
func Path(data, name string) string {
	return filepath.Join(tunDir(), name)
}

func tunDir() string {
	root := os.Getenv("ProgramData")
	if root == "" {
		root = `C:\ProgramData`
	}
	return filepath.Join(root, "bowel", "tun", userSID())
}

// userSID is the user's security identifier, as S-1-5-21-…: a name no
// other user's directory has.
func userSID() string {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "user"
	}
	return u.User.Sid.String()
}

// taskName is the first scheduled task's name earlier versions
// registered, which the elevated command deletes.
func taskName() string { return taskNameOf(userSID()) }

// Installed reports whether path is where TUN's core runs from: then
// starting it asks for the authorization and makes the copy as it goes,
// so there is nothing to install first.
func Installed(path string) bool {
	return CoreDir != "" && strings.EqualFold(filepath.Dir(path), tunDir())
}

// stageDir is where the copies wait for the authorization: the user
// cannot write beside dst.
func stageDir(string) string { return filepath.Join(os.TempDir(), "bowel-tun") }

func checkDir(string) error { return nil }

func installStep(tmp, dst string) (Step, error) {
	// The app's own program, to be the launcher.
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	ltmp, err := copyTemp(exe, filepath.Dir(tmp))
	if err != nil {
		return nil, err
	}
	// The log starts empty and is the app's own file, which the elevated
	// command adds to.
	log := installLogPath()
	if err := os.WriteFile(log, nil, 0o600); err != nil {
		_ = os.Remove(ltmp)
		return nil, err
	}
	return winInstallStep(tmp, ltmp, dst, taskName(), log), nil
}

// installLogPath is where the elevated command's log is, and stays after
// a start that stopped, until the next one makes the copy.
func installLogPath() string { return filepath.Join(stageDir(""), installLogName) }

// consoleText is what a console program wrote to a file, as text: in the
// system's OEM code page, which is not UTF-8 where the system's language
// is not English.
func consoleText(b []byte) string {
	if len(b) == 0 || utf8.Valid(b) {
		return string(b)
	}
	const oem = 1 // CP_OEMCP
	n, err := windows.MultiByteToWideChar(oem, 0, &b[0], int32(len(b)), nil, 0)
	if err != nil || n <= 0 {
		return strings.ToValidUTF8(string(b), "?")
	}
	u := make([]uint16, n)
	if _, err := windows.MultiByteToWideChar(oem, 0, &b[0], int32(len(b)), &u[0], n); err != nil {
		return strings.ToValidUTF8(string(b), "?")
	}
	return string(utf16.Decode(u))
}

// Uninstall deletes the copy's directory, and the tasks an earlier
// version may have left, which takes an administrator. A missing dst is
// not an error.
func Uninstall(dst string) error {
	if _, err := os.Lstat(dst); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return runAdmin(Prompt(), []Step{{"(schtasks /Delete /F /TN ", taskName(), " >nul 2>nul & schtasks /Delete /F /TN ", fallbackName(taskName()),
		" >nul 2>nul & ver >nul) && rmdir /s /q ", filepath.Dir(dst)}})
}

// shellExecuteInfo is SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	size       uint32
	mask       uint32
	hwnd       windows.Handle
	verb       *uint16
	file       *uint16
	parameters *uint16
	directory  *uint16
	show       int32
	instApp    windows.Handle
	idList     uintptr
	class      *uint16
	keyClass   windows.Handle
	hotKey     uint32
	icon       windows.Handle
	process    windows.Handle
}

const (
	seeNoCloseProcess = 0x00000040 // SEE_MASK_NOCLOSEPROCESS
	seeNoUI           = 0x00000400 // SEE_MASK_FLAG_NO_UI: no error boxes of its own
)

var procShellExecuteEx = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

// shellRunas runs file with params elevated, hidden, in dir ("" for the
// caller's), after Windows' own prompt, and returns the process: a
// handle that waits and reads the exit code, and may be refused more.
// A prompt the user dismissed is windows.ERROR_CANCELLED.
func shellRunas(file, params, dir string) (windows.Handle, error) {
	verb, _ := windows.UTF16PtrFromString("runas")
	f, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return 0, err
	}
	p, err := windows.UTF16PtrFromString(params)
	if err != nil {
		return 0, err
	}
	var d *uint16
	if dir != "" {
		if d, err = windows.UTF16PtrFromString(dir); err != nil {
			return 0, err
		}
	}
	info := shellExecuteInfo{mask: seeNoCloseProcess | seeNoUI, verb: verb, file: f, parameters: p, directory: d, show: windows.SW_HIDE}
	info.size = uint32(unsafe.Sizeof(info))
	if r, _, err := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		return 0, err
	}
	if info.process == 0 {
		return 0, errors.New(i18n.T("授权失败：没有启动提权的进程", "Authorization failed: no elevated process was started"))
	}
	return info.process, nil
}

// runAdmin runs the steps in an elevated cmd.exe, hidden, and waits for
// it. Windows' own prompt names the program, and cannot show prompt.
func runAdmin(_ string, steps []Step) error {
	sh, err := winScript(steps)
	if err != nil {
		return err
	}
	// An installation among the steps reports to a log (see
	// winInstallStep): the exit code says only that something failed.
	log := installLog(steps)
	sys, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	// /s: what is between the outer quotes is the command, as written.
	h, err := shellRunas(filepath.Join(sys, "cmd.exe"), `/d /s /c "`+sh+`"`, "")
	if err != nil {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			if log != "" {
				_ = os.Remove(log) // nothing ran: there is nothing to say
			}
			return ErrCanceled
		}
		return fmt.Errorf(i18n.T("授权失败：%s", "Authorization failed: %s"), err)
	}
	defer windows.CloseHandle(h)
	if _, err := windows.WaitForSingleObject(h, windows.INFINITE); err != nil {
		return err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return err
	}
	if code != 0 {
		var said []byte
		if log != "" {
			said, _ = os.ReadFile(log)
		}
		return installErr(code, consoleText(said))
	}
	if log != "" {
		_ = os.Remove(log)
	}
	return nil
}
