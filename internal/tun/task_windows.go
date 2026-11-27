package tun

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/nowindow"
)

// Task is the core as a start of TUN runs it: elevated through a UAC
// prompt, by the launcher beside it, not a child of the app. Or one
// another run of the app left running elevated (left: see Attach).
type Task struct {
	exe  string // the core's file name, as the process list has it
	root uint32 // the elevated process: the launcher, or cmd.exe around it
	pid  int
	left bool // Attach's: proc is the core's own, 0 where refused, and there is no stop

	proc windows.Handle // root's, as the prompt gave it
	stop windows.Handle // the event that asks the launcher to end the core (see appRef)

	mu     sync.Mutex
	asked  bool // Stop was called
	closed bool // proc and stop are closed: root has ended

	once sync.Once
	done chan struct{}
	err  error
}

// What Spawn depends on. Tests replace them.
var (
	elevate   = shellRunas
	guarded   = protected
	spawnWait = 30 * time.Second
)

// Spawn starts the core at bin, elevated, when bin is the copy TUN runs
// on, and returns nil for any other: that one runs as a child. src is the
// bundled core, which the copy is made from.
//
// Windows asks each time: a program that registers a scheduled task to
// run elevated (versions before this one did, so that the user was asked
// once) is what Defender's behaviour monitoring takes for malware when
// the program is unsigned and rarely seen, as bowel is. So nothing is
// registered, and the UAC prompt elevates one of two programs. The
// launcher beside the copy, when the copy is current and closed to the
// user (see current); otherwise cmd.exe, which first makes the copy as
// the installation did (winInstallStep) and then runs the launcher
// (winStartStep). Either way what is elevated is in the directory only
// administrators write, or is the system's own.
//
// The prompt's answer is waited for, and then the core: until it appears
// under the elevated process, or that process ends without it, which is a
// failure, with the reason the command's log gives.
func Spawn(bin, src string) (*Task, error) {
	if !Installed(bin) {
		return nil, nil
	}
	stop, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, err
	}
	app, err := thisApp(stop)
	if err != nil {
		windows.CloseHandle(stop)
		return nil, err
	}
	file, params, log, err := startCommand(bin, src, launcherArgs(app, bin, CoreDir))
	if err != nil {
		windows.CloseHandle(stop)
		return nil, err
	}
	proc, err := elevate(file, params, CoreDir)
	if err != nil {
		windows.CloseHandle(stop)
		if log != "" {
			clearStage()
		}
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return nil, startCanceled{}
		}
		return nil, fmt.Errorf(i18n.T("无法以管理员身份启动 TUN 的内核：%s", "Couldn't start TUN's core as administrator: %s"), err)
	}
	t := &Task{exe: filepath.Base(bin), proc: proc, stop: stop, done: make(chan struct{})}
	if t.root, err = windows.GetProcessId(proc); err != nil || t.root == 0 {
		// Without its id the core cannot be told apart: it is ended unseen.
		if err == nil {
			err = errors.New("no process id")
		}
		t.Stop()
		go t.Wait()
		return nil, fmt.Errorf(i18n.T("无法以管理员身份启动 TUN 的内核：%s", "Couldn't start TUN's core as administrator: %s"), err)
	}
	if err := t.await(log); err != nil {
		return nil, err
	}
	return t, nil
}

// thisApp is the app's reference for the launcher, with stop as its event.
func thisApp(stop windows.Handle) (appRef, error) {
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(windows.CurrentProcess(), &created, &exited, &kernel, &user); err != nil {
		return appRef{}, err
	}
	return appRef{pid: windows.GetCurrentProcessId(), created: filetime(created), stop: uint64(stop)}, nil
}

// startCommand is what the prompt elevates to start the core at bin with
// the launcher's args: the launcher itself, or cmd.exe with the copy to
// make first, made from src; log is that command's, "" for the launcher.
func startCommand(bin, src string, args []string) (file, params, log string, err error) {
	launcher := launcherPath(bin)
	if current(bin, src) {
		return launcher, windows.ComposeCommandLine(args), "", nil
	}
	step, err := InstallStep(src, bin)
	if err != nil {
		return "", "", "", err
	}
	log = installLogPath()
	sh, err := winScript([]Step{step, winStartStep(launcher, args, log)})
	if err != nil {
		clearStage()
		return "", "", "", err
	}
	sys, err := windows.GetSystemDirectory()
	if err != nil {
		clearStage()
		return "", "", "", err
	}
	// /s: what is between the outer quotes is the command, as written.
	return filepath.Join(sys, "cmd.exe"), `/d /s /c "` + sh + `"`, log, nil
}

// clearStage deletes what a start that did not happen staged.
func clearStage() {
	if old, err := filepath.Glob(filepath.Join(stageDir(""), ".mihomo-*")); err == nil {
		for _, p := range old {
			_ = os.Remove(p)
		}
	}
}

// current is whether the launcher can be elevated as it is to start the
// core at bin: an installation of this kind made the directory (its
// mark), it is closed to the user (protected), no core from an earlier
// version still runs from bin, and the core and the launcher are the
// bundled core src and the app's own program, byte for byte. A refresh
// costs nothing but a moment, the prompt being there anyway, so any doubt
// is answered with one.
func current(bin, src string) bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	b, err := os.ReadFile(markPath(bin))
	return err == nil && strings.TrimSpace(string(b)) == markText && guarded(bin) &&
		len(running(bin)) == 0 && sameFile(src, bin) && sameFile(exe, launcherPath(bin))
}

// The rights that would let a program change what an elevated start
// runs: a file's contents, its place, who may change it; for the
// directory, what is in it as well; for the two folders above it, what
// takes its place (a user may make files and folders beside it, as
// %ProgramData% lets everyone).
var (
	fileRights   = []uint32{windows.FILE_WRITE_DATA, windows.FILE_APPEND_DATA, windows.DELETE, windows.WRITE_DAC, windows.WRITE_OWNER}
	dirRights    = []uint32{fileAddFile, fileAddSubdirectory, fileDeleteChild, windows.DELETE, windows.WRITE_DAC, windows.WRITE_OWNER}
	parentRights = []uint32{fileDeleteChild, windows.DELETE, windows.WRITE_DAC, windows.WRITE_OWNER}
)

// A folder's rights, which share their bits with a file's.
const (
	fileAddFile         = 0x0002 // FILE_ADD_FILE
	fileAddSubdirectory = 0x0004 // FILE_ADD_SUBDIRECTORY
	fileDeleteChild     = 0x0040 // FILE_DELETE_CHILD
)

// protected is whether no program of the user's can change what an
// elevated start of bin runs. It asks Windows for each right in turn
// with this process's own token, which is not elevated: what that is
// refused, every other program the user runs is refused as well, and
// Windows weighs the owner, the groups, the denials and the integrity
// level as it will for them. An answer other than a refusal (a file in
// use, one that is missing) counts as a right granted.
func protected(bin string) bool {
	dir := winDir(bin)
	for _, c := range []struct {
		path   string
		rights []uint32
	}{
		{bin, fileRights}, {launcherPath(bin), fileRights}, {dir, dirRights},
		{winDir(dir), parentRights}, {winDir(winDir(dir)), parentRights},
	} {
		for _, r := range c.rights {
			if mayOpen(c.path, r) {
				return false
			}
		}
	}
	return true
}

func mayOpen(path string, access uint32) bool {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return true
	}
	const share = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE
	// The link or junction itself, not what it points at; and a folder can
	// only be opened with backup semantics.
	h, err := windows.CreateFile(p, access, share, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err == nil {
		windows.CloseHandle(h)
		return true
	}
	return !errors.Is(err, windows.ERROR_ACCESS_DENIED)
}

// sameFile is whether a and b hold the same bytes.
func sameFile(a, b string) bool {
	fa, err := os.Open(a)
	if err != nil {
		return false
	}
	defer fa.Close()
	fb, err := os.Open(b)
	if err != nil {
		return false
	}
	defer fb.Close()
	sa, err1 := fa.Stat()
	sb, err2 := fb.Stat()
	if err1 != nil || err2 != nil || sa.Size() != sb.Size() {
		return false
	}
	ba, bb := make([]byte, 1<<20), make([]byte, 1<<20)
	for {
		na, erra := io.ReadFull(fa, ba)
		nb, errb := io.ReadFull(fb, bb)
		if na != nb || !bytes.Equal(ba[:na], bb[:nb]) {
			return false
		}
		if erra != nil || errb != nil {
			return (erra == io.EOF || erra == io.ErrUnexpectedEOF) && (errb == io.EOF || errb == io.ErrUnexpectedEOF)
		}
	}
}

// await waits for the core under the elevated process, or for that process
// to end. log is the command's, "" for the launcher alone.
func (t *Task) await(log string) error {
	deadline := time.Now().Add(spawnWait)
	for {
		if pid := coreOf(snapshot(), t.root, t.exe); pid != 0 {
			t.pid = int(pid)
			return nil
		}
		ev, err := windows.WaitForSingleObject(t.proc, 100)
		if err != nil {
			t.Stop()
			go t.Wait()
			return err
		}
		if ev == windows.WAIT_OBJECT_0 {
			var code uint32
			_ = windows.GetExitCodeProcess(t.proc, &code)
			t.release()
			var said []byte
			if log != "" {
				said, _ = os.ReadFile(log) // it stays, for whoever looks
			}
			return endedErr(code, consoleText(said))
		}
		if time.Now().After(deadline) {
			// The event stays set until the process is gone: a launcher
			// that starts after this ends at once.
			t.Stop()
			go t.Wait()
			return fmt.Errorf(i18n.T("以管理员身份启动后 %d 秒内没有看到内核", "No core appeared within %d seconds of starting it as administrator"), int(spawnWait/time.Second))
		}
	}
}

// Attach is the running core pid as a task, for a core another run of the
// app left running elevated from bin (core.Manager.Detach, or a crash);
// nil when bin is not the copy TUN runs on, or pid is a core the app may
// end itself as any process. The app has no right to end this one. A core
// this version's launcher started ends with the app that started it,
// which is gone, so it is only waited for: an update's restart cannot
// hand it over. One that an earlier version's scheduled task started is
// ended through that task, which the user may end unelevated, until the
// next start's elevated command deletes the task and ends whatever still
// runs from bin.
func Attach(bin string, pid int) *Task {
	if !Installed(bin) {
		return nil
	}
	if h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid)); err == nil {
		own := !sameImage(imagePath(h), bin)
		windows.CloseHandle(h)
		if own {
			return nil
		}
	}
	t := &Task{exe: filepath.Base(bin), pid: pid, left: true, done: make(chan struct{})}
	if h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid)); err == nil {
		t.proc = h
	}
	return t
}

// endLeft ends a core Attach found: through an earlier version's tasks,
// and as a process where the app happens to be allowed (an elevated app).
func (t *Task) endLeft() {
	for _, name := range []string{taskName(), fallbackName(taskName())} {
		_ = schtasks("/End", "/TN", name)
	}
	if h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(t.pid)); err == nil {
		_ = windows.TerminateProcess(h, 1)
		windows.CloseHandle(h)
	}
}

// listed is whether the process list has pid running exe: for a process
// whose handle the app is refused.
func listed(pid uint32, exe string) bool {
	for _, p := range snapshot() {
		if p.pid == pid && strings.EqualFold(p.exe, exe) {
			return true
		}
	}
	return false
}

// schtasks runs the Task Scheduler's command, hidden.
func schtasks(args ...string) error {
	sys, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	return nowindow.Command(filepath.Join(sys, "schtasks.exe"), args...).Run()
}

func (t *Task) Pid() int { return t.pid }

// Wait returns once the elevated process is gone, which the launcher is
// only once the core is. A core Attach found is waited for by its own
// handle, or where that is refused by asking the process list twice a
// second.
func (t *Task) Wait() error {
	t.once.Do(func() {
		defer close(t.done)
		if t.left && t.proc == 0 {
			for listed(uint32(t.pid), t.exe) {
				time.Sleep(500 * time.Millisecond)
			}
			t.release()
			return
		}
		if _, err := windows.WaitForSingleObject(t.proc, windows.INFINITE); err != nil {
			t.err = err
			return // the handles stay: Stop may still need them
		}
		var code uint32
		if !t.left && windows.GetExitCodeProcess(t.proc, &code) == nil && code != 0 {
			t.err = fmt.Errorf("exit status %d", code)
		}
		t.release()
	})
	<-t.done
	return t.err
}

// Stop asks the launcher to end the core. Asked again, it also ends the
// elevated process itself, where the prompt's handle allows it: the
// launcher's job takes the core with it, and cmd.exe's end leaves the
// launcher to the event. A core Attach found is ended as endLeft can.
func (t *Task) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	if t.left {
		t.endLeft()
		return
	}
	if t.asked {
		_ = windows.TerminateProcess(t.proc, 1)
	}
	t.asked = true
	_ = windows.SetEvent(t.stop)
}

// release closes the handles, once the elevated process has ended: no
// launcher reads the event after that.
func (t *Task) release() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.closed {
		for _, h := range []windows.Handle{t.proc, t.stop} {
			if h != 0 {
				windows.CloseHandle(h)
			}
		}
		t.closed = true
	}
}

// running are the processes whose program is the installed core bin.
// Another program's core has the same name and is none of them; nor is
// one whose path the system will not tell.
func running(bin string) []int {
	var out []int
	for _, p := range snapshot() {
		if !strings.EqualFold(p.exe, filepath.Base(bin)) && !strings.EqualFold(p.exe, filepath.Base(bin)+".old") {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, p.pid)
		if err != nil {
			continue
		}
		if sameImage(imagePath(h), bin) {
			out = append(out, int(p.pid))
		}
		windows.CloseHandle(h)
	}
	return out
}

// imagePath is the path of the program the process h runs, "" when it
// cannot be told.
func imagePath(h windows.Handle) string {
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// snapshot is every process, with its parent and its program's name.
func snapshot() []coreProc {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var out []coreProc
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		out = append(out, coreProc{pid: e.ProcessID, parent: e.ParentProcessID, exe: windows.UTF16ToString(e.ExeFile[:])})
	}
	return out
}
