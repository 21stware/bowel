package tun

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

// The command that makes the copy: the folders above it and its own are
// made the administrators', the earlier versions' tasks go, the copy in
// use is renamed aside, the launcher goes in first and ends what still
// runs from the core's path before the core is replaced, and the mark is
// written last. Ending and deleting the tasks cannot fail the step. Each
// part names itself in the log before it begins and its commands print
// there, so that the part that stopped the command can be told.
func TestWinInstallStep(t *testing.T) {
	dst := `C:\ProgramData\bowel\tun\S-1\mihomo.exe`
	const log = `C:\Users\a b\Temp\bowel-tun\.mihomo-install.log`
	st := winInstallStep(`C:\Users\a b\Temp\bowel-tun\.mihomo-1`, `C:\Users\a b\Temp\bowel-tun\.mihomo-2`, dst, "bowel-tun-S-1", log)
	if len(st)%2 != 0 || st[len(st)-1] != log {
		t.Fatalf("step doesn't end on a path, the log's: %q", st)
	}
	got, err := winScript([]Step{st})
	if err != nil {
		t.Fatal(err)
	}
	if st[1] != `C:\Users\a b\Temp\bowel-tun\.mihomo-1` {
		t.Errorf("the copy is not the first path: %q", st[1])
	}
	if installLog([]Step{{"rem ", `C:\x`}, st}) != log || installLog([]Step{{"rmdir /s /q ", `C:\ProgramData\bowel\tun\S-1`}}) != "" {
		t.Error("the log is not found among the steps, or one is found where there is none")
	}
	const to = ` >> "C:\Users\a b\Temp\bowel-tun\.mihomo-install.log"`
	want := `(if not exist "C:\Users\a b\Temp\bowel-tun\.mihomo-1" echo [staged]` + to[1:] + `)` +
		` && (if not exist "C:\Users\a b\Temp\bowel-tun\.mihomo-1" exit /b 1)` +
		` && (echo [dir])` + to[1:] +
		` && (if not exist "C:\ProgramData\bowel\tun\S-1" mkdir "C:\ProgramData\bowel\tun\S-1")` + to + ` 2>&1` +
		` && icacls "C:\ProgramData\bowel" /setowner *S-1-5-32-544 /L` + to + ` 2>&1` +
		` && icacls "C:\ProgramData\bowel" /reset /L` + to + ` 2>&1` +
		` && icacls "C:\ProgramData\bowel\tun" /setowner *S-1-5-32-544 /L` + to + ` 2>&1` +
		` && icacls "C:\ProgramData\bowel\tun" /reset /L` + to + ` 2>&1` +
		` && icacls "C:\ProgramData\bowel\tun\S-1" /setowner *S-1-5-32-544 /L` + to + ` 2>&1` +
		` && icacls "C:\ProgramData\bowel\tun\S-1" /setowner *S-1-5-32-544 /T /C /L` + to + ` 2>&1` +
		` && icacls "C:\ProgramData\bowel\tun\S-1" /inheritance:r /grant:r *S-1-5-32-544:(OI)(CI)F *S-1-5-18:(OI)(CI)F *S-1-5-32-545:(OI)(CI)RX /L` + to + ` 2>&1` +
		` && (echo [launcher])` + to[1:] +
		` && (schtasks /End /TN "bowel-tun-S-1" >nul 2>nul & schtasks /End /TN "bowel-tun-S-1-interactive" >nul 2>nul` +
		` & schtasks /Delete /F /TN "bowel-tun-S-1" >nul 2>nul & schtasks /Delete /F /TN "bowel-tun-S-1-interactive" >nul 2>nul & ver >nul)` +
		` && (del /f /q "C:\ProgramData\bowel\tun\S-1\bowel-tun.exe.old" 2>nul & ver >nul)` +
		` && (if exist "C:\ProgramData\bowel\tun\S-1\bowel-tun.exe" move /y "C:\ProgramData\bowel\tun\S-1\bowel-tun.exe" "C:\ProgramData\bowel\tun\S-1\bowel-tun.exe.old")` + to + ` 2>&1` +
		` && copy /y /b "C:\Users\a b\Temp\bowel-tun\.mihomo-2" "C:\ProgramData\bowel\tun\S-1\bowel-tun.exe"` + to + ` 2>&1` +
		` && (del /f /q "C:\Users\a b\Temp\bowel-tun\.mihomo-2" 2>nul & start "" /wait "C:\ProgramData\bowel\tun\S-1\bowel-tun.exe" tun-core --kill "C:\ProgramData\bowel\tun\S-1\mihomo.exe" & ver >nul)` +
		` && (echo [core])` + to[1:] +
		` && (del /f /q "C:\ProgramData\bowel\tun\S-1\mihomo.exe.old" 2>nul & ver >nul)` +
		` && (if exist "C:\ProgramData\bowel\tun\S-1\mihomo.exe" move /y "C:\ProgramData\bowel\tun\S-1\mihomo.exe" "C:\ProgramData\bowel\tun\S-1\mihomo.exe.old")` + to + ` 2>&1` +
		` && copy /y /b "C:\Users\a b\Temp\bowel-tun\.mihomo-1" "C:\ProgramData\bowel\tun\S-1\mihomo.exe"` + to + ` 2>&1` +
		` && (del /f /q "C:\Users\a b\Temp\bowel-tun\.mihomo-1" 2>nul & ver >nul)` +
		` && (echo [mark])` + to[1:] +
		` && (echo per-start-1)> "C:\ProgramData\bowel\tun\S-1\installed"` +
		` && (echo [done])` + to[1:]
	if got != want {
		t.Fatalf("script:\n got %s\nwant %s", got, want)
	}
	// Only administrators and SYSTEM get more than reading and running.
	if strings.Count(dirACL, "F") != 2 || !strings.Contains(dirACL, "*S-1-5-32-545:(OI)(CI)RX") {
		t.Errorf("ACL %s", dirACL)
	}
}

// Nothing is registered to run later: no task, no service, no Run key.
func TestWinInstallStepRegistersNothing(t *testing.T) {
	st := winInstallStep(`C:\T\.mihomo-1`, `C:\T\.mihomo-2`, `C:\P\b\tun\S\mihomo.exe`, "t", `C:\T\`+installLogName)
	start := winStartStep(`C:\P\b\tun\S\bowel-tun.exe`, launcherArgs(appRef{1, 2, 3}, `C:\P\b\tun\S\mihomo.exe`, `C:\core`), `C:\T\`+installLogName)
	sh, err := winScript([]Step{st, start})
	if err != nil {
		t.Fatal(err)
	}
	low := strings.ToLower(sh)
	for _, bad := range []string{"/create", "sc create", "sc.exe", "new-service", "reg add", "reg.exe", `\run`, "startup"} {
		if strings.Contains(low, bad) {
			t.Errorf("the command has %q:\n%s", bad, sh)
		}
	}
	if n := strings.Count(low, "schtasks "); n != 4 || strings.Count(low, "schtasks /delete") != 2 || strings.Count(low, "schtasks /end") != 2 {
		t.Errorf("schtasks does more than end and delete the two old tasks:\n%s", sh)
	}
}

// The parts of the command, read off the script: every part is named in
// the log in the order installSteps has them, then done, then the start;
// each name is one the report reads; and nothing the command prints is
// thrown away except where a failure is expected and ignored.
func TestWinInstallStepParts(t *testing.T) {
	log := `C:\T\` + installLogName
	st := winInstallStep(`C:\T\.mihomo-1`, `C:\T\.mihomo-2`, `C:\P\b\tun\S\mihomo.exe`, "t", log)
	start := winStartStep(`C:\P\b\tun\S\bowel-tun.exe`, launcherArgs(appRef{1, 2, 3}, `C:\P\b\tun\S\mihomo.exe`, `C:\core`), log)
	sh, err := winScript([]Step{st, start})
	if err != nil {
		t.Fatal(err)
	}
	at := 0
	for _, name := range append(append([]string{}, installSteps...), installDone, startPart) {
		i := strings.Index(sh[at:], "echo ["+name+"]")
		if i < 0 {
			t.Fatalf("part %q is not named, or out of order:\n%s", name, sh)
		}
		at += i
		if got, ok := stepName("[" + name + "]"); !ok || got != name {
			t.Errorf("the report does not read %q", name)
		}
		if name != installDone && name != startPart && (stepText(name) == name || stepText(name) == "") {
			t.Errorf("part %q has no description", name)
		}
	}
	for _, c := range strings.Split(sh, " && ") {
		if strings.Contains(c, "nul") && !strings.Contains(c, "& ver >nul)") {
			t.Errorf("a command that can stop the installation says nothing: %s", c)
		}
		// icacls ends with 0 when /C let it go past a failure: only the
		// files the copy's folder holds may be changed so.
		if strings.HasPrefix(c, "icacls ") && strings.Contains(c, " /C") && !strings.Contains(c, `S" /setowner *S-1-5-32-544 /T /C`) {
			t.Errorf("a change of rights that cannot fail the command: %s", c)
		}
		// And none follows a junction to change what it points at.
		if strings.HasPrefix(c, "icacls ") && !strings.Contains(c, " /L >> ") {
			t.Errorf("a change of rights that follows links: %s", c)
		}
	}
	// cmd.exe takes 8191 characters; with paths as long as a profile's get.
	long := `C:\Users\` + strings.Repeat("u", 40) + `\AppData\Local\Temp\bowel-tun\`
	sid := "S-1-5-21-1234567890-1234567890-1234567890-500001"
	dst := `C:\ProgramData\bowel\tun\` + sid + `\mihomo.exe`
	st = winInstallStep(long+".mihomo-1234567890", long+".mihomo-2234567890", dst, taskNameOf(sid), long+installLogName)
	app := appRef{4294967295, 18446744073709551615, 18446744073709551615}
	start = winStartStep(launcherPath(dst), launcherArgs(app, dst, `C:\Users\`+strings.Repeat("u", 40)+`\AppData\Roaming\bowel\core`), long+installLogName)
	if sh, _ = winScript([]Step{st, start}); len(sh) > 7000 {
		t.Errorf("the command is %d characters long", len(sh))
	}
}

// The start runs the launcher with the app's reference and the core's own
// arguments, waits for it, and keeps what start says of it in the log.
func TestWinStartStep(t *testing.T) {
	bin := `C:\ProgramData\bowel\tun\S-1\mihomo.exe`
	args := launcherArgs(appRef{pid: 42, created: 133, stop: 7}, bin, `C:\Users\张 & 三\AppData\Roaming\bowel\core`)
	st := winStartStep(launcherPath(bin), args, `C:\T\.mihomo-install.log`)
	if len(st)%2 != 0 {
		t.Fatalf("parts don't alternate: %q", st)
	}
	got, err := winScript([]Step{st})
	if err != nil {
		t.Fatal(err)
	}
	want := `(echo [start])>> "C:\T\.mihomo-install.log" && start "" /wait "C:\ProgramData\bowel\tun\S-1\bowel-tun.exe" tun-core --app 42:133:7` +
		` "C:\ProgramData\bowel\tun\S-1\mihomo.exe" -d "C:\Users\张 & 三\AppData\Roaming\bowel\core" -f "C:\Users\张 & 三\AppData\Roaming\bowel\core\config.yaml"` +
		` 2>> "C:\T\.mihomo-install.log"`
	if got != want {
		t.Fatalf("start:\n got %s\nwant %s", got, want)
	}
	// What the launcher reads is what it was given.
	l, ok := launchArgs(args[1:])
	if !ok || l.core != bin || l.app != (appRef{42, 133, 7}) || strings.Join(l.args, " ") != `-d C:\Users\张 & 三\AppData\Roaming\bowel\core -f C:\Users\张 & 三\AppData\Roaming\bowel\core\config.yaml` {
		t.Errorf("read back: %+v %v", l, ok)
	}
}

// What the log says of a start: the part the copy stopped at and what
// Windows printed there; nothing when the copy was made, or never begun.
func TestInstallReport(t *testing.T) {
	ok := "[staged]\r\n[dir]\r\nprocessed file: C:\\P\r\nSuccessfully processed 1 files; Failed processing 0 files\r\n[launcher]\r\n        1 file(s) copied.\r\n[core]\r\n"
	for _, c := range []struct {
		name, log  string
		step, said string
		done       bool
		err        string // what the error adds to the exit code, "" for nothing
	}{
		{name: "nothing was written", log: ""},
		{name: "the staged copy was gone", log: "[staged]\r\n", step: "staged",
			err: "停在「找到待安装的内核副本」这一步，Windows 没有给出原因"},
		{name: "a file in use", log: "[staged]\n[dir]\n[launcher]\n[core]\n        1 file(s) moved.\nThe process cannot access the file because it is being used by another process.\n        0 file(s) copied.\n",
			step: "core", said: "1 file(s) moved. The process cannot access the file because it is being used by another process. 0 file(s) copied.",
			err: "停在「替换内核 mihomo.exe」这一步，Windows 的输出：1 file(s) moved. The process"},
		{name: "the system's language", log: "[staged]\n[dir]\n错误: 拒绝访问。\r\n", step: "dir", said: "错误: 拒绝访问。", err: "错误: 拒绝访问。"},
		{name: "the mark could not be written", log: ok + "        1 file(s) copied.\r\n[mark]\r\nAccess is denied.\r\n", step: "mark", said: "Access is denied.",
			err: "停在「写入安装标记」这一步"},
		{name: "done", log: ok + "[mark]\r\n[done]\r\n", step: "done", done: true},
		{name: "done, and started", log: ok + "[mark]\r\n[done]\r\n[start]\r\n", step: "start", done: true},
		{name: "brackets a command printed are not a part", log: "[staged]\n[dir]\n[C:\\P]\n[done] \nno\n[launcher]x\n", step: "done", said: "no [launcher]x", done: true},
	} {
		r := readReport(c.log)
		if r.step != c.step || r.said != c.said || r.done != c.done {
			t.Errorf("%s: read %+v", c.name, r)
		}
		err := installErr(1, c.log).Error()
		if !strings.Contains(err, "退出码 1") || (c.err == "") != !strings.Contains(err, "：") || !strings.Contains(err, c.err) {
			t.Errorf("%s: error %q", c.name, err)
		}
	}
	// What is shown stays short, however much a command printed.
	if r := readReport("[dir]\n" + strings.Repeat("拒绝访问 ", 400)); len([]rune(r.said)) > 601 || !strings.HasSuffix(r.said, "…") {
		t.Errorf("%d characters kept", len([]rune(r.said)))
	}
}

// A start whose elevated process ended before the core was seen: a copy
// that stopped half way says where; past it, what start said of the
// launcher, or what the launcher's exit code means.
func TestEndedErr(t *testing.T) {
	copied := "[staged]\n[dir]\n[launcher]\n[core]\n[mark]\n[done]\n"
	for _, c := range []struct {
		name string
		code uint32
		log  string
		want string
	}{
		{"the copy stopped", 1, "[staged]\n[dir]\nAccess is denied.\n", "授权后的安装没有完成（退出码 1）：停在「建立安装目录并设置权限」这一步，Windows 的输出：Access is denied."},
		{"start could not run the launcher", 225, copied + "[start]\nOperation did not complete successfully because the file contains a virus or potentially unwanted software.\n",
			"启动器没有运行起来，Windows 的输出：Operation did not complete successfully because the file contains a virus"},
		{"no job", launchNoJob, copied + "[start]\n", "随它结束内核的作业对象"},
		{"the launcher alone, no job", launchNoJob, "", "随它结束内核的作业对象"},
		{"no core", launchNoCore, "", "启动器没能运行内核"},
		{"the core quit", 1, copied + "[start]\n", "内核刚启动就退出了（退出码 1）"},
		{"the launcher alone, the core quit", 1, "", "内核刚启动就退出了（退出码 1）"},
		// What was copied before is not what this start said.
		{"a log of the copy only", 1, copied, "内核刚启动就退出了（退出码 1）"},
	} {
		if got := endedErr(c.code, c.log).Error(); !strings.Contains(got, c.want) {
			t.Errorf("%s: %q, want %q in it", c.name, got, c.want)
		}
	}
}

func TestWinScriptQuoting(t *testing.T) {
	s, err := winScript([]Step{{"del ", `C:\a b\c&d^e`}, {"echo ok"}})
	if err != nil || s != `del "C:\a b\c&d^e" && echo ok` {
		t.Fatalf("script %q, %v", s, err)
	}
	for _, bad := range []string{`C:\a"b`, `C:\%TEMP%\x`, "C:\\a\nb"} {
		if _, err := winScript([]Step{{"del ", bad}}); err == nil {
			t.Errorf("%q was taken", bad)
		}
	}
}

// The mark is none an earlier version wrote: those were a digest, sixteen
// hexadecimal digits, of what their task was registered with.
func TestMarkText(t *testing.T) {
	if regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(markText) || strings.ContainsAny(markText, `"%&|<>^() `) {
		t.Errorf("mark %q", markText)
	}
	if markPath(`C:\ProgramData\bowel\tun\S & 1\mihomo.exe`) != `C:\ProgramData\bowel\tun\S & 1\installed` {
		t.Error("the mark is not beside the copy")
	}
	if launcherPath(`C:\ProgramData\bowel\tun\S-1\mihomo.exe`) != `C:\ProgramData\bowel\tun\S-1\bowel-tun.exe` {
		t.Error("the launcher is not beside the core")
	}
	if fallbackName(taskNameOf("S-1")) != "bowel-tun-S-1-interactive" {
		t.Error("the second task's name")
	}
}

// What follows the launcher's own argument is the app's reference, the
// core and its arguments, or a path to end the processes of, as the
// installation asks; anything else starts nothing.
func TestLaunchArgs(t *testing.T) {
	bin := `C:\ProgramData\bowel\tun\S-1\mihomo.exe`
	l, ok := launchArgs([]string{"--app", "1:2:3", bin, "-d", `C:\core`, "-f", `C:\core\config.yaml`})
	if !ok || l.kill != "" || l.core != bin || l.app != (appRef{1, 2, 3}) || strings.Join(l.args, " ") != `-d C:\core -f C:\core\config.yaml` {
		t.Errorf("a core to start: %+v %v", l, ok)
	}
	l, ok = launchArgs([]string{"--kill", bin})
	if !ok || l.kill != bin || l.core != "" || l.args != nil {
		t.Errorf("a path to end: %+v %v", l, ok)
	}
	for _, bad := range [][]string{nil, {""}, {"--kill"}, {"--kill", ""}, {"--kill", bin, "more"}, {"-d", `C:\core`},
		// The form the second task of an earlier version used: no app.
		{bin, "-d", `C:\core`},
		{"--app", "1:2:3"}, {"--app", "1:2", bin}, {"--app", "x:2:3", bin}, {"--app", "0:2:3", bin}, {"--app", "1:2:0", bin}, {"--app", "1:2:3", "-d"}} {
		if _, ok := launchArgs(bad); ok {
			t.Errorf("%q was taken", bad)
		}
	}
	if args := launcherArgs(appRef{1, 2, 3}, bin, `C:\core`); args[0] != LauncherArg || strings.Join(args[1:3], " ") != "--app 1:2:3" {
		t.Errorf("launcher args %q", args)
	}
}

func TestAppRef(t *testing.T) {
	for _, a := range []appRef{{1, 0, 1}, {4294967295, 18446744073709551615, 18446744073709551615}, {1234, 133456789012345678, 0x2a4}} {
		if b, ok := parseAppRef(a.String()); !ok || b != a {
			t.Errorf("%+v read back as %+v, %v", a, b, ok)
		}
	}
	for _, bad := range []string{"", "1", "1:2", "1:2:3:4", "-1:2:3", "4294967296:2:3", "1:2:x", " 1:2:3"} {
		if _, ok := parseAppRef(bad); ok {
			t.Errorf("%q was taken", bad)
		}
	}
}

// The launcher starts no core but the one in its own directory.
func TestBesideLauncher(t *testing.T) {
	self := `C:\ProgramData\bowel\tun\S-1\bowel-tun.exe`
	for core, want := range map[string]bool{
		`C:\ProgramData\bowel\tun\S-1\mihomo.exe`:            true,
		`c:\programdata\BOWEL\tun\s-1\MIHOMO.EXE`:            true,
		`C:\ProgramData\bowel\tun\S-1\sub\mihomo.exe`:        false,
		`C:\Users\u\AppData\Local\Programs\bowel\mihomo.exe`: false,
		"": false,
	} {
		if besideLauncher(core, self) != want {
			t.Errorf("besideLauncher(%q) is not %v", core, want)
		}
	}
	if besideLauncher(`C:\x\mihomo.exe`, "") {
		t.Error("a launcher with no path of its own starts a core")
	}
}

// The installed core is told from another program's of the same name by
// its path, whatever its case, and is still itself once a later
// installation has moved it aside.
func TestSameImage(t *testing.T) {
	bin := `C:\ProgramData\bowel\tun\S-1\mihomo.exe`
	for path, want := range map[string]bool{
		bin: true,
		`c:\programdata\BOWEL\tun\s-1\MIHOMO.EXE`: true,
		bin + ".old": true,
		`C:\Program Files\Clash Verge\mihomo.exe`: false,
		"": false,
	} {
		if sameImage(path, bin) != want {
			t.Errorf("sameImage(%q) is not %v", path, want)
		}
	}
	if sameImage("", "") {
		t.Error("no core is every process")
	}
}

// The core is the one the elevated process started, directly or through
// the cmd.exe around the launcher; another program's core of the same
// name is not.
func TestCoreOf(t *testing.T) {
	procs := []coreProc{
		{pid: 4, parent: 0, exe: "System"},
		{pid: 100, parent: 4, exe: "cmd.exe"},         // the elevated command
		{pid: 200, parent: 100, exe: "bowel-tun.exe"}, // its launcher
		{pid: 300, parent: 200, exe: "MIHOMO.EXE"},    // its core
		{pid: 400, parent: 9, exe: "mihomo.exe"},      // Clash Verge's
		{pid: 500, parent: 600, exe: "mihomo.exe"},    // a launcher alone's
		{pid: 600, parent: 1, exe: "bowel-tun.exe"},
	}
	for _, c := range []struct {
		root uint32
		want uint32
	}{{100, 300}, {200, 300}, {600, 500}, {4, 0}, {9, 400}, {700, 0}} {
		if got := coreOf(procs, c.root, "mihomo.exe"); got != c.want {
			t.Errorf("under %d: %d, want %d", c.root, got, c.want)
		}
	}
	if coreOf(procs, 100, "bowel-tun.exe") != 200 {
		t.Error("a launcher under cmd.exe is a process named so under it")
	}
	if coreOf([]coreProc{{pid: 7, parent: 7, exe: "mihomo.exe"}}, 7, "mihomo.exe") != 0 {
		t.Error("the elevated process is its own core")
	}
}

// Dismissing the prompt of a start is ErrCanceled to whoever asks, and
// says how to be asked again.
func TestStartCanceled(t *testing.T) {
	var err error = startCanceled{}
	if !errors.Is(err, ErrCanceled) || err.Error() == ErrCanceled.Error() || !strings.Contains(err.Error(), "关掉再打开 TUN") {
		t.Errorf("canceled: %v", err)
	}
}
