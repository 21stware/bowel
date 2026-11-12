package tun

// What Windows' installation is made of, apart from the calls that only
// Windows has, so that every system's tests read it.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"

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

// taskRev changes when what installStep does changes, so that an older
// installation asks to be made again.
const taskRev = "2"

// mark is what the installation leaves beside dst: a digest of everything
// the task was registered with.
func mark(dst, coreDir, sid string) string {
	h := sha256.Sum256([]byte(strings.Join([]string{taskRev, strings.ToLower(dst), strings.ToLower(coreDir), sid}, "\x00")))
	return hex.EncodeToString(h[:8])
}

// The directory's owner and its only writers are administrators and
// SYSTEM; users read and run what is in it.
const dirACL = `*S-1-5-32-544:(OI)(CI)F *S-1-5-18:(OI)(CI)F *S-1-5-32-545:(OI)(CI)RX`

// noFallbackPath is the file that says the installation has no second
// task: registering it failed, and what schtasks said is what the file
// holds. Like the mark it is in the directory only administrators write.
func noFallbackPath(dst string) string { return winDir(dst) + `\no-fallback` }

// installLogName is the file, in the directory the copy is staged in,
// that the elevated command reports to: the app sees nothing of that
// command but its exit code. The user can write there, as they can write
// the staged copy; what the file says is shown and decides nothing.
const installLogName = ".mihomo-install.log"

// installSteps are the parts of the elevated command, in order: each
// writes its name, in brackets, to the log before it begins, and what its
// commands print follows. The command stops at the first that fails, so
// the last name in the log is the part that failed, unless it is done.
// The first, which only looks for the staged copy, is named when the copy
// is missing: the copy has to stay the step's first path.
var installSteps = []string{"staged", "dir", "launcher", "core", "task", "fallback", "mark"}

const installDone = "done"

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
	case "task":
		return i18n.T("注册计划任务", "registering the scheduled task")
	case "fallback":
		return i18n.T("注册备用的计划任务", "registering the second scheduled task")
	case "mark":
		return i18n.T("写入安装标记", "writing the installation's mark")
	}
	return step
}

// winInstallStep is the elevated command. The copy in use may be
// running, and a running program's file can be renamed but not written:
// it steps aside as .old, which the next installation deletes. The copy
// to install is the step's first path, as Install expects.
//
// ltmp is the app's own program, which becomes the second task's
// launcher (see fallbackTaskXML); it is copied, not moved, so that it
// takes the directory's permissions and not the ones it had where the
// user could write it. Before the core is replaced both tasks are ended
// and the new launcher, elevated here, ends any core still running from
// dst: one that a task's end left behind, which the app has no right to
// end itself.
//
// Both tasks are registered, the second being the one startTask falls
// back to. The first is the installation: without it there is no mark.
// The second is for the accounts the first starts nothing for, and a
// machine that refuses it still has TUN: what schtasks said goes to
// noFallbackPath, which stays only when it failed, whatever was left of
// an older second task is deleted, and the mark is written all the same.
//
// log is where each part's name and output go (see installSteps); the
// app reads it afterwards, since the exit code is all it gets otherwise.
func winInstallStep(tmp, ltmp, dst, xml, task, xml2, task2, mark, log string) Step {
	dir := winDir(dst)
	old := dst + ".old"
	launcher := launcherPath(dst)
	lold := launcher + ".old"
	nofb := noFallbackPath(dst)
	return Step{
		"(if not exist ", tmp, " echo [staged]>> ", log, ") && (if not exist ", tmp, " exit /b 1) && (echo [dir])>> ", log,
		" && (if not exist ", dir, " mkdir ", dir, ") >> ", log, " 2>&1 && icacls ", dir, " /setowner *S-1-5-32-544 /T /C >> ", log,
		" 2>&1 && icacls ", dir, " /inheritance:r /grant:r " + dirACL + " >> ", log,
		" 2>&1 && (echo [launcher])>> ", log,
		" && (schtasks /End /TN ", task, " >nul 2>nul & schtasks /End /TN ", task2, " >nul 2>nul & ver >nul)" +
			" && (del /f /q ", lold, " 2>nul & ver >nul) && (if exist ", launcher, " move /y ", launcher, " ", lold,
		") >> ", log, " 2>&1 && copy /y /b ", ltmp, " ", launcher, " >> ", log,
		" 2>&1 && (del /f /q ", ltmp, ` 2>nul & start "" /wait `, launcher, " " + LauncherArg + " " + launcherKill + " ", dst,
		" & ver >nul) && (echo [core])>> ", log,
		" && (del /f /q ", old, " 2>nul & ver >nul) && (if exist ", dst, " move /y ", dst, " ", old,
		") >> ", log, " 2>&1 && copy /y /b ", tmp, " ", dst, " >> ", log,
		" 2>&1 && (echo [task])>> ", log, " && schtasks /Create /F /TN ", task, " /XML ", xml, " >> ", log,
		" 2>&1 && (echo [fallback])>> ", log,
		" && (schtasks /Create /F /TN ", task2, " /XML ", xml2, " > ", nofb, " 2>&1 && (del /f /q ", nofb,
		" 2>nul & ver >nul) || (schtasks /Delete /F /TN ", task2, " >nul 2>nul & ver >nul)) && (echo [mark])>> ", log,
		" && (echo " + mark + ")> ", markPath(dst), " && (echo [" + installDone + "])>> ", log,
	}
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
	done bool   // every part ran
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
			r.step, r.done, said = name, name == installDone, nil
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
	if name == installDone {
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

// failureNote is what the TUN entry says of an installation that stopped,
// from the log it left, "" when it left none that says so.
func failureNote(log string) string {
	if why := readReport(log).failure(); why != "" {
		return i18n.T("上一次安装没有完成：", "The last installation didn't finish: ") + why
	}
	return ""
}

// fallbackNote is what is said, once, of an installation whose second
// task could not be registered; said is what schtasks printed.
func fallbackNote(said string) string {
	said = clip(strings.Join(strings.Fields(said), " "))
	if said == "" {
		said = i18n.T("没有给出原因", "no reason given")
	}
	return fmt.Sprintf(i18n.T("TUN 已安装，但备用的计划任务没能注册（Windows 的输出：%s）。TUN 照常使用第一个任务；只有它启动不了内核时（多半是域账户连不上域控制器）才会受影响",
		"TUN is installed, but its second scheduled task couldn't be registered (Windows said: %s). TUN runs on the first task as usual; this only matters when that one can't start the core (most likely a domain account whose controller can't be reached)"), said)
}

// taskRunErr is why the task did not run, from what schtasks /Run said
// and what /Query says of the same task. A task the Task Scheduler does
// not have is an installation that is gone, whatever the mark says; what
// schtasks prints is in the system's language, so only whether the query
// failed is read.
func taskRunErr(run, query error) error {
	if query != nil {
		return ErrGone
	}
	return fmt.Errorf(i18n.T("无法通过计划任务启动内核：%s", "Couldn't start the core through its scheduled task: %s"), run)
}

// taskXML is the definition of the task the core is started through
// first. It runs as the user without their password being kept (S4U), so
// it has no window to show, at the highest run level the user has; it has
// no trigger, starts only on demand, and is never ended for running long
// or on battery. The core is the action itself, with nothing in between
// that could set its environment: ending the task ends that one process,
// and a shell around the core would be ended in its place. What the
// description tells the user to delete is what Uninstall deletes.
func taskXML(sid, bin, coreDir string) string {
	return taskDef(sid, "S4U", bin, coreArgs(coreDir), bin, coreDir, fallbackName(taskNameOf(sid)))
}

// fallbackTaskXML is the definition of the second task, for the accounts
// the first cannot start for: an S4U logon of a domain account asks the
// domain's controller, and fails when that cannot be reached (a laptop
// away from the office's network). This one takes the token of the
// session the user is logged on in (InteractiveToken), which asks nobody,
// and so runs only while the user is logged on at the desktop. There the
// core, a console program, would open a console window, and closing it
// would end TUN. So the action is not the core but a launcher: a copy of
// the app's own program beside the core, which has no console (a release
// build is a GUI program) and starts the core without one. Ending a task
// ends its one process and none that process started (the console host
// run headless, tried first, left the core running, elevated, where the
// app could not end it): the launcher therefore puts itself in a job that
// Windows ends with its last handle, which is the launcher's own, and the
// core goes with it. See Launch.
func fallbackTaskXML(sid, launcher, bin, coreDir string) string {
	return taskDef(sid, "InteractiveToken", launcher, LauncherArg+` "`+bin+`" `+coreArgs(coreDir), bin, coreDir, taskNameOf(sid))
}

// LauncherArg is the app's first argument when it is run as the second
// task's launcher: what follows is the core and its arguments, or
// launcherKill and the installed core's path.
const LauncherArg = "tun-core"

// launcherKill asks the launcher, in place of a core to start, to end
// every process running from the path that follows.
const launcherKill = "--kill"

// launcherPath is where the launcher is kept: beside the core, where only
// administrators can replace it.
func launcherPath(dst string) string { return winDir(dst) + `\bowel-tun.exe` }

// launchArgs reads what follows LauncherArg: a path to end the processes
// of, or the core and its arguments.
func launchArgs(args []string) (kill, core string, rest []string, ok bool) {
	switch {
	case len(args) == 2 && args[0] == launcherKill && args[1] != "":
		return args[1], "", nil, true
	case len(args) >= 1 && args[0] != "" && !strings.HasPrefix(args[0], "-"):
		return "", args[0], args[1:], true
	}
	return "", "", nil, false
}

// sameImage is whether a process's program, path, is the installed core
// bin, or that core moved aside by a later installation while it ran.
func sameImage(path, bin string) bool {
	return bin != "" && (strings.EqualFold(path, bin) || strings.EqualFold(path, bin+".old"))
}

// orphaned is why the core was not started when one from before is still
// running after both tasks were ended. It runs elevated, so the app
// cannot end it, and it holds the ports and the adapter the next would
// need. It is ErrGone to whoever asks: the installation is made again,
// and that, elevated, ends it.
type orphaned struct{}

func (orphaned) Error() string {
	return i18n.T("上一次的内核还在以管理员权限运行，bowel 没有权限结束它；重新开启 TUN 会再授权一次并结束它",
		"The core from an earlier run is still running with administrator rights, and bowel isn't allowed to end it; turning TUN on again asks for the authorization once more and ends it")
}

func (orphaned) Is(target error) bool { return target == ErrGone }

// coreArgs are the arguments the app runs its core with.
func coreArgs(coreDir string) string {
	return fmt.Sprintf(`-d "%s" -f "%s"`, coreDir, coreDir+`\config.yaml`)
}

// taskNameOf is the first task's name, one per user; fallbackName is the
// second's, from the first's.
func taskNameOf(sid string) string { return "bowel-tun-" + sid }

func fallbackName(task string) string { return task + "-interactive" }

// taskDef is a task that runs command with args in coreDir as the user
// sid, logged on the way logon says. bin is the installed core, whose
// folder the description names along with the installation's other task.
func taskDef(sid, logon, command, args, bin, coreDir, other string) string {
	return `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>bowel: runs the mihomo core with the rights TUN mode needs. To undo it, delete this task and the folder ` + xmlText(winDir(bin)) + `, and the task ` + xmlText(other) + `</Description>
  </RegistrationInfo>
  <Triggers />
  <Principals>
    <Principal id="Author">
      <UserId>` + xmlText(sid) + `</UserId>
      <LogonType>` + logon + `</LogonType>
      <RunLevel>HighestAvailable</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>false</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <WakeToRun>false</WakeToRun>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>4</Priority>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>` + xmlText(command) + `</Command>
      <Arguments>` + xmlText(args) + `</Arguments>
      <WorkingDirectory>` + xmlText(coreDir) + `</WorkingDirectory>
    </Exec>
  </Actions>
</Task>
`
}

func xmlText(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// utf16File is s as schtasks reads a definition: UTF-16, little-endian,
// with its byte order mark.
func utf16File(s string) []byte {
	u := utf16.Encode([]rune(strings.ReplaceAll(s, "\n", "\r\n")))
	b := make([]byte, 2, 2+2*len(u))
	b[0], b[1] = 0xff, 0xfe
	for _, c := range u {
		b = binary.LittleEndian.AppendUint16(b, c)
	}
	return b
}

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

// rememberFallback is what is kept, between starts, of which task started
// the core last when it was not the first.
const rememberFallback = "interactive"

// taskOrder is the order the tasks are tried in: the first task first,
// unless the second was the one that worked last time. An installation
// whose second task could not be registered (fallback is false) has the
// first alone, whatever was remembered.
func taskOrder(task, last string, fallback bool) []string {
	switch {
	case !fallback:
		return []string{task}
	case strings.TrimSpace(last) == rememberFallback:
		return []string{fallbackName(task), task}
	}
	return []string{task, fallbackName(task)}
}

// noFallback is why the core was not started when the first task started
// nothing and the installation, made by an older version, has no second
// task: it is ErrGone to whoever asks, so that the installation is made
// again, with both.
type noFallback struct{}

func (noFallback) Error() string {
	return i18n.T("这个账户现在无法用原来的方式启动内核（多半是连不上域控制器）；重新开启 TUN 会再授权一次，之后不再受此影响",
		"The core couldn't be started the usual way for this account (most likely its domain controller can't be reached); turning TUN on again asks for the authorization once more, and it won't depend on that afterwards")
}

func (noFallback) Is(target error) bool { return target == ErrGone }

func notStartedErr() error {
	return errors.New(i18n.T("计划任务没有把内核启动起来", "The scheduled task didn't start the core"))
}

// unregisteredErr is err, why the first task did not start the core,
// for an installation that has no second task because Windows refused
// it. Making the installation again would be refused the same, so this
// is not ErrGone.
func unregisteredErr(err error) error {
	return fmt.Errorf(i18n.T("%s；备用的计划任务在安装时没能注册，没有别的方式可以启动内核", "%s; the second scheduled task couldn't be registered when TUN was installed, so there is no other way to start the core"), err)
}

// startTask starts the core through one of the installation's two tasks
// and returns the name of the one that did. run and query are schtasks'
// /Run and /Query of a task; started waits for the core's process after a
// task ran and reports whether one appeared: a task whose logon fails
// runs, as far as schtasks can tell, and starts nothing.
//
// Without its first task the installation is gone. A second task that is
// missing is an older version's installation, which matters only once the
// first has started nothing; one the installation says it could not
// register (fallback is false) is not tried at all.
func startTask(task, last string, fallback bool, run, query func(name string) error, started func() bool) (string, error) {
	var failed, old error
	for _, name := range taskOrder(task, last, fallback) {
		err := run(name)
		if err == nil {
			if started() {
				return name, nil
			}
			err = notStartedErr()
		} else if query(name) != nil {
			if name == task {
				return "", ErrGone
			}
			old = noFallback{}
			continue
		} else {
			err = taskRunErr(err, nil)
		}
		if failed == nil || name == task {
			failed = err // the first task's reason is the one to show
		}
	}
	if old != nil {
		return "", old
	}
	if !fallback && failed != nil {
		return "", unregisteredErr(failed)
	}
	return "", failed
}
