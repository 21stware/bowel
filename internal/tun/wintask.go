package tun

// What Windows' TUN is made of, apart from the calls that only Windows
// has, so that every system's tests read it.

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/oboo/moho/internal/i18n"
)

func markPath(dst string) string { return winDir(dst) + `\installed` }

// winDir is filepath.Dir for a Windows path, on any system.
func winDir(p string) string {
	if i := strings.LastIndexAny(p, `\/`); i > 0 {
		return p[:i]
	}
	return p
}

// markText is what the elevated command writes beside the copy once it
// has made the directory, put this version's launcher and core in it and
// deleted the scheduled tasks earlier versions registered. An earlier
// version's mark, a digest of what its task was registered with, is never
// this, so the first start after an update from one of those goes through
// the whole command, and the tasks go then. The mark decides nothing about
// safety: protected does (see current).
const markText = "per-start-1"

// The directory's owner and its only writers are administrators and
// SYSTEM; users read and run what is in it.
const dirACL = `*S-1-5-32-544:(OI)(CI)F *S-1-5-18:(OI)(CI)F *S-1-5-32-545:(OI)(CI)RX`

// installLogName is the file, in the directory the copy is staged in,
// that the elevated command reports to: the app sees nothing of that
// command but its exit code. The user can write there, as they can write
// the staged copy; what the file says is shown and decides nothing.
const installLogName = ".mihomo-install.log"

// installSteps are the parts of the elevated command that make the copy,
// in order: each writes its name, in brackets, to the log before it
// begins, and what its commands print follows. The command stops at the
// first that fails, so the last name in the log is the part that failed,
// unless it is done. The first, which only looks for the staged copy, is
// named when the copy is missing: the copy has to stay the step's first
// path.
var installSteps = []string{"staged", "dir", "launcher", "core", "mark"}

const installDone = "done"

// startPart is the name the start of the core writes to the log after
// the copy is done (see winStartStep): a log that ends there tells of a
// launcher that did not run, not of a copy that was not made.
const startPart = "start"

// stepText is a part of the installation as the user is told of it.
func stepText(step string) string {
	switch step {
	case "staged":
		return i18n.T("找到待安装的内核副本", "finding the copy of the core to install")
	case "dir":
		return i18n.T("建立安装目录并设置权限", "making the installation's folder and setting its permissions")
	case "launcher":
		return i18n.T("替换启动器 bowel-tun.exe", "replacing the launcher bowel-tun.exe")
	case "core":
		return i18n.T("替换内核 mihomo.exe", "replacing the core mihomo.exe")
	case "mark":
		return i18n.T("写入安装标记", "writing the installation's mark")
	}
	return step
}

// winInstallStep is the part of the elevated command that makes the copy
// the core is started from, for a start that finds it missing, another
// version's, or not closed to the user (see current). The copy in use may
// be running, and a running program's file can be renamed but not written:
// it steps aside as .old, which the next installation deletes. The copy to
// install is the step's first path, as Install expects.
//
// The folders above the copy's are made the administrators' too, with no
// rights of their own: a user who owned one could move the copy's folder
// aside and put another in its place between this command and the start.
// None of the owners is changed with /C, which makes icacls end with 0
// when it failed: only the files inside the copy's folder are, which the
// command replaces anyway. Every icacls has /L, so that a junction a user
// put at one of these paths is what it changes, and not the folder,
// anywhere on the system, that the junction points at.
//
// ltmp is the app's own program, which becomes the launcher (see Launch);
// both are copied, not moved, so that they take the directory's
// permissions and not the ones they had where the user could write them.
// Before the launcher is replaced the scheduled tasks of earlier versions
// (task and its fallbackName) are ended and deleted, which nothing fails
// on, and the new launcher, elevated here, ends any core still running
// from dst: one such a task started, which the app has no right to end.
//
// log is where each part's name and output go (see installSteps); the
// app reads it when the command ends before the core is seen.
func winInstallStep(tmp, ltmp, dst, task, log string) Step {
	dir := winDir(dst)
	parent := winDir(dir)
	top := winDir(parent)
	old := dst + ".old"
	launcher := launcherPath(dst)
	lold := launcher + ".old"
	task2 := fallbackName(task)
	return Step{
		"(if not exist ", tmp, " echo [staged]>> ", log, ") && (if not exist ", tmp, " exit /b 1) && (echo [dir])>> ", log,
		" && (if not exist ", dir, " mkdir ", dir, ") >> ", log,
		" 2>&1 && icacls ", top, " /setowner *S-1-5-32-544 /L >> ", log, " 2>&1 && icacls ", top, " /reset /L >> ", log,
		" 2>&1 && icacls ", parent, " /setowner *S-1-5-32-544 /L >> ", log, " 2>&1 && icacls ", parent, " /reset /L >> ", log,
		" 2>&1 && icacls ", dir, " /setowner *S-1-5-32-544 /L >> ", log,
		" 2>&1 && icacls ", dir, " /setowner *S-1-5-32-544 /T /C /L >> ", log,
		" 2>&1 && icacls ", dir, " /inheritance:r /grant:r " + dirACL + " /L >> ", log,
		" 2>&1 && (echo [launcher])>> ", log,
		" && (schtasks /End /TN ", task, " >nul 2>nul & schtasks /End /TN ", task2, " >nul 2>nul & schtasks /Delete /F /TN ", task,
		" >nul 2>nul & schtasks /Delete /F /TN ", task2, " >nul 2>nul & ver >nul)" +
			" && (del /f /q ", lold, " 2>nul & ver >nul) && (if exist ", launcher, " move /y ", launcher, " ", lold,
		") >> ", log, " 2>&1 && copy /y /b ", ltmp, " ", launcher, " >> ", log,
		" 2>&1 && (del /f /q ", ltmp, ` 2>nul & start "" /wait `, launcher, " " + LauncherArg + " " + launcherKill + " ", dst,
		" & ver >nul) && (echo [core])>> ", log,
		" && (del /f /q ", old, " 2>nul & ver >nul) && (if exist ", dst, " move /y ", dst, " ", old,
		") >> ", log, " 2>&1 && copy /y /b ", tmp, " ", dst, " >> ", log,
		" 2>&1 && (del /f /q ", tmp, " 2>nul & ver >nul) && (echo [mark])>> ", log,
		" && (echo " + markText + ")> ", markPath(dst), " && (echo [" + installDone + "])>> ", log,
	}
}

// winStartStep is the last part of that command: it starts the launcher,
// with args (see launcherArgs), and waits for it, so that the command
// ends with the core and with its exit code. What start says when it
// cannot run the launcher (Defender's verdict on it, say) goes to the
// log; the launcher keeps that file open while it runs, and prints
// nothing to it.
func winStartStep(launcher string, args []string, log string) Step {
	// args are LauncherArg, launcherApp and the app's reference, which need
	// no quoting, then the core and its arguments: paths, and the flags
	// between them.
	st := Step{"(echo [" + startPart + "])>> ", log, ` && start "" /wait `, launcher, " " + strings.Join(args[:3], " ") + " "}
	for i, a := range args[3:] {
		if i%2 == 0 { // the core, its directory, its config
			st = append(st, a)
		} else {
			st = append(st, " "+a+" ")
		}
	}
	return append(st, " 2>> ", log)
}

// installLog is the log among the steps' paths, "" when none of them is
// an installation.
func installLog(steps []Step) string {
	for _, st := range steps {
		for i := 1; i < len(st); i += 2 {
			if j := strings.LastIndexAny(st[i], `\/`); st[i][j+1:] == installLogName {
				return st[i]
			}
		}
	}
	return ""
}

// installReport is what the elevated command left in its log.
type installReport struct {
	step string // the last part begun, "" when nothing was written
	said string // what that part's commands printed
	done bool   // every part of the installation ran
}

// readReport reads the log: the last part named in it and what follows
// that name. What the commands print is in the system's language and is
// passed on as it is, on one line.
func readReport(log string) installReport {
	var r installReport
	var said []string
	for _, line := range strings.Split(log, "\n") {
		line = strings.TrimSpace(line)
		if name, ok := stepName(line); ok {
			r.step, said = name, nil
			r.done = name == installDone || name == startPart
			continue
		}
		if line != "" {
			said = append(said, line)
		}
	}
	r.said = clip(strings.Join(said, " "))
	return r
}

func stepName(line string) (string, bool) {
	if !strings.HasPrefix(line, "[") || !strings.HasSuffix(line, "]") {
		return "", false
	}
	name := line[1 : len(line)-1]
	if name == installDone || name == startPart {
		return name, true
	}
	for _, s := range installSteps {
		if s == name {
			return name, true
		}
	}
	return "", false
}

// clip keeps what a command said short enough to show.
func clip(s string) string {
	const max = 600
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}

// failure is why the installation stopped, "" when the log does not say
// that it did: nothing in it, or all of it done.
func (r installReport) failure() string {
	if r.step == "" || r.done {
		return ""
	}
	if r.said == "" {
		return fmt.Sprintf(i18n.T("停在「%s」这一步，Windows 没有给出原因", "it stopped at %s, and Windows gave no reason"), stepText(r.step))
	}
	return fmt.Sprintf(i18n.T("停在「%s」这一步，Windows 的输出：%s", "it stopped at %s, where Windows said: %s"), stepText(r.step), r.said)
}

// installErr is what the installation's authorisation returns when the
// elevated command ended with code: with the part it stopped at when the
// log has one.
func installErr(code uint32, log string) error {
	if why := readReport(log).failure(); why != "" {
		return fmt.Errorf(i18n.T("授权后的安装没有完成（退出码 %d）：%s", "The installation didn't finish after the authorization (exit code %d): %s"), code, why)
	}
	return fmt.Errorf(i18n.T("授权后的安装没有完成（退出码 %d）", "The installation didn't finish after the authorization (exit code %d)"), code)
}

// endedErr is why the core did not start when the elevated process, the
// launcher or the command around it, ended with code before the core was
// seen. log is what that command left in its log, "" for a launcher
// started by itself: a copy that stopped half way says where, and
// otherwise it was the start that failed.
func endedErr(code uint32, log string) error {
	r := readReport(log)
	if r.failure() != "" {
		return installErr(code, log)
	}
	said := ""
	if r.step == startPart {
		said = r.said
	}
	return launchErr(code, said)
}

// The launcher's exit codes for what keeps it from starting the core.
// Once the core runs, the launcher's code is the core's.
const (
	launchBadArgs   = 70 // what followed LauncherArg is not something it does
	launchNoApp     = 71 // the app it names is not running (see appRef)
	launchNotBeside = 72 // the core is not the one beside it
	launchNoJob     = 73 // no job to end the core with it
	launchNoCore    = 74 // the core could not be run
)

// launchErr is why a start of the core ended with code before the core
// was seen; said is what Windows said of running the launcher, when the
// command around it heard anything.
func launchErr(code uint32, said string) error {
	if said != "" {
		return fmt.Errorf(i18n.T("启动器没有运行起来，Windows 的输出：%s", "The launcher didn't run; Windows said: %s"), said)
	}
	switch code {
	case launchNoJob:
		return errors.New(i18n.T("Windows 没有让启动器建立随它结束内核的作业对象，内核没有启动", "Windows didn't let the launcher make the job that ends the core along with it, so the core wasn't started"))
	case launchNoCore:
		return errors.New(i18n.T("启动器没能运行内核", "The launcher couldn't run the core"))
	}
	return fmt.Errorf(i18n.T("内核刚启动就退出了（退出码 %d）", "The core quit as soon as it started (exit code %d)"), code)
}

// startCanceled is ErrCanceled as a start of the core has it: the
// prompt comes with every start, and the TUN entry says how to see it
// again.
type startCanceled struct{}

func (startCanceled) Error() string {
	return i18n.T("用户账户控制的授权被取消了，关掉再打开 TUN 会再请求一次", "The User Account Control prompt was canceled; turn TUN off and on to be asked again")
}

func (startCanceled) Is(target error) bool { return target == ErrCanceled }

// LauncherArg is the app's first argument when it is run as the launcher:
// what follows is launcherApp, the app's reference and the core with its
// arguments, or launcherKill and the installed core's path.
const LauncherArg = "tun-core"

// launcherKill asks the launcher, in place of a core to start, to end
// every process running from the path that follows.
const launcherKill = "--kill"

// launcherApp comes before the reference to the app the core is started
// for (see appRef).
const launcherApp = "--app"

// launcherPath is where the launcher is kept: beside the core, where only
// administrators can replace it.
func launcherPath(dst string) string { return winDir(dst) + `\bowel-tun.exe` }

// appRef is how the launcher finds the app it starts the core for: its
// process, which it ends the core with, told from a later process with
// the same id by when it was created; and the event in it that asks for
// the core to end, a handle of the app's own, which the launcher copies
// out of the app's process. The event has no name: only a program that
// may open the app's process to copy its handles can set it, which is
// one that could end the app anyway. It ends the core, and asks nothing
// else.
type appRef struct {
	pid     uint32
	created uint64 // the process's creation time, as a FILETIME
	stop    uint64 // the event's handle, in the app's process
}

func (a appRef) String() string {
	return strconv.FormatUint(uint64(a.pid), 10) + ":" + strconv.FormatUint(a.created, 10) + ":" + strconv.FormatUint(a.stop, 10)
}

func parseAppRef(s string) (appRef, bool) {
	f := strings.Split(s, ":")
	if len(f) != 3 {
		return appRef{}, false
	}
	pid, err1 := strconv.ParseUint(f[0], 10, 32)
	created, err2 := strconv.ParseUint(f[1], 10, 64)
	stop, err3 := strconv.ParseUint(f[2], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil || pid == 0 || stop == 0 {
		return appRef{}, false
	}
	return appRef{uint32(pid), created, stop}, true
}

// launcherArgs are the launcher's arguments for starting the core at bin
// for app, the app's own arguments for its core after the core.
func launcherArgs(app appRef, bin, coreDir string) []string {
	return []string{LauncherArg, launcherApp, app.String(), bin, "-d", coreDir, "-f", coreDir + `\config.yaml`}
}

// launch is what follows LauncherArg, read: a path to end the processes
// of, or the core to start for app and its arguments.
type launch struct {
	kill string
	app  appRef
	core string
	args []string
}

func launchArgs(args []string) (launch, bool) {
	switch {
	case len(args) == 2 && args[0] == launcherKill && args[1] != "":
		return launch{kill: args[1]}, true
	case len(args) >= 3 && args[0] == launcherApp && args[2] != "" && !strings.HasPrefix(args[2], "-"):
		app, ok := parseAppRef(args[1])
		if !ok {
			return launch{}, false
		}
		return launch{app: app, core: args[2], args: args[3:]}, true
	}
	return launch{}, false
}

// besideLauncher is whether core is in the launcher's own directory, the
// one only administrators write: the only core it starts.
func besideLauncher(core, self string) bool {
	return core != "" && self != "" && strings.EqualFold(winDir(core), winDir(self))
}

// sameImage is whether a process's program, path, is the installed core
// bin, or that core moved aside by a later installation while it ran.
func sameImage(path, bin string) bool {
	return bin != "" && (strings.EqualFold(path, bin) || strings.EqualFold(path, bin+".old"))
}

// taskNameOf is the name of the scheduled task versions before this one
// started the core through, one per user; fallbackName is their second
// task's, from the first's. The elevated command deletes both.
func taskNameOf(sid string) string { return "bowel-tun-" + sid }

func fallbackName(task string) string { return task + "-interactive" }

// cmdQuote makes p one word of a cmd.exe command line. A quote cannot be
// part of a file's name; a percent sign would be read as a variable's.
func cmdQuote(p string) (string, error) {
	if strings.ContainsAny(p, "\"%\r\n") {
		return "", fmt.Errorf(i18n.T("路径里有不能使用的字符：%s", "The path has a character that can't be used: %s"), p)
	}
	return `"` + p + `"`, nil
}

// winScript is the steps as one cmd.exe command line, stopping at the first
// that fails.
func winScript(steps []Step) (string, error) {
	cmds := make([]string, len(steps))
	for i, st := range steps {
		var b strings.Builder
		for j, p := range st {
			if j%2 == 1 {
				q, err := cmdQuote(p)
				if err != nil {
					return "", err
				}
				p = q
			}
			b.WriteString(p)
		}
		cmds[i] = b.String()
	}
	return strings.Join(cmds, " && "), nil
}

// coreProc is a process as a snapshot of them lists it.
type coreProc struct {
	pid, parent uint32
	exe         string
}

// coreOf is the core the elevated process root started: a process named
// exe whose parent is root (the launcher, started by itself) or a child
// of root (the launcher, started by cmd.exe). 0 when there is none yet.
func coreOf(procs []coreProc, root uint32, exe string) uint32 {
	parent := make(map[uint32]uint32, len(procs))
	for _, p := range procs {
		parent[p.pid] = p.parent
	}
	for _, p := range procs {
		if !strings.EqualFold(p.exe, exe) || p.pid == root {
			continue
		}
		if p.parent == root || (p.parent != 0 && parent[p.parent] == root) {
			return p.pid
		}
	}
	return 0
}
