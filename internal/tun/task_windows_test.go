package tun

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// The test binary is also the launcher, the core and an app, as the
// tests below start it: what Windows elevates is replaced by a plain
// start, and everything else runs as it does for the app.
const fakeEnv = "BOWEL_TUN_FAKE"

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == LauncherArg {
		os.Exit(Launch(os.Args[2:]))
	}
	switch os.Getenv(fakeEnv) {
	case "core": // runs until it is ended
		time.Sleep(time.Hour)
		os.Exit(0)
	case "app": // starts a launcher for itself, says its id, and waits
		stop, _ := windows.CreateEvent(nil, 1, 0, nil)
		app, err := thisApp(stop)
		if err != nil {
			os.Exit(1)
		}
		self, _ := os.Executable()
		c := exec.Command(self, launcherArgs(app, self, os.TempDir())...)
		c.Env = fakeAs("core")
		if err := c.Start(); err != nil {
			os.Exit(1)
		}
		fmt.Println(c.Process.Pid)
		time.Sleep(time.Hour)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func fakeAs(role string) []string { return append(os.Environ(), fakeEnv+"="+role) }

// coreUnder waits for the core the process root started.
func coreUnder(t *testing.T, root uint32, exe string) uint32 {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if pid := coreOf(snapshot(), root, exe); pid != 0 {
			return pid
		}
	}
	t.Fatalf("no %s under %d", exe, root)
	return 0
}

// gone waits for pid to end, and reports whether it did.
func gone(pid uint32, within time.Duration) bool {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return true // no such process
	}
	defer windows.CloseHandle(h)
	ev, err := windows.WaitForSingleObject(h, uint32(within/time.Millisecond))
	return err == nil && ev == windows.WAIT_OBJECT_0
}

// The launcher ends the core when the app sets the event, and then ends
// itself.
func TestLaunchEndsTheCoreWhenAsked(t *testing.T) {
	stop, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(stop)
	app, err := thisApp(stop)
	if err != nil {
		t.Fatal(err)
	}
	self, _ := os.Executable()
	c := exec.Command(self, launcherArgs(app, self, t.TempDir())...)
	c.Env = fakeAs("core")
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	core := coreUnder(t, uint32(c.Process.Pid), filepath.Base(self))
	if gone(core, 300*time.Millisecond) {
		t.Fatal("the core ended by itself")
	}
	if err := windows.SetEvent(stop); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("the launcher: %v", err)
		}
	case <-time.After(10 * time.Second):
		_ = c.Process.Kill()
		t.Fatal("the launcher did not end")
	}
	if !gone(core, 5*time.Second) {
		t.Error("the core outlived its launcher")
	}
}

// An app that is gone, by a crash or by being ended, takes the core with
// it: nothing is left holding the ports and the adapter.
func TestLaunchEndsTheCoreWithTheApp(t *testing.T) {
	self, _ := os.Executable()
	app := exec.Command(self)
	app.Env = fakeAs("app")
	out, err := app.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Start(); err != nil {
		t.Fatal(err)
	}
	defer app.Process.Kill()
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	launcher, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatal(err)
	}
	core := coreUnder(t, uint32(launcher), filepath.Base(self))
	if gone(uint32(launcher), 300*time.Millisecond) {
		t.Fatal("the launcher ended while the app ran")
	}
	if err := app.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = app.Wait()
	if !gone(uint32(launcher), 10*time.Second) {
		t.Fatal("the launcher outlived the app")
	}
	if !gone(core, 5*time.Second) {
		t.Error("the core outlived the app")
	}
}

// The launcher starts nothing for a core outside its own directory, or
// for an app that is not the one its reference names.
func TestLaunchRefuses(t *testing.T) {
	stop, _ := windows.CreateEvent(nil, 1, 0, nil)
	defer windows.CloseHandle(stop)
	app, err := thisApp(stop)
	if err != nil {
		t.Fatal(err)
	}
	self, _ := os.Executable()
	elsewhere := filepath.Join(t.TempDir(), "mihomo.exe")
	other := app
	other.created++
	for _, c := range []struct {
		name string
		args []string
		code int
	}{
		{"a core elsewhere", launcherArgs(app, elsewhere, `C:\core`), launchNotBeside},
		{"another app", launcherArgs(other, self, `C:\core`), launchNoApp},
		{"no app", launcherArgs(appRef{pid: 0xfffffff0, created: 1, stop: 1}, self, `C:\core`), launchNoApp},
		{"an earlier version's arguments", []string{LauncherArg, self, "-d", `C:\core`}, launchBadArgs},
	} {
		cmd := exec.Command(self, c.args...)
		cmd.Env = fakeAs("core")
		err := cmd.Run()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != c.code {
			t.Errorf("%s: %v, want exit code %d", c.name, err, c.code)
		}
	}
}

// What the user's own programs may do is what this process may: a
// file of the user's is open to them, as is one that is missing; the
// system's own is not, unless the test runs elevated.
func TestMayOpen(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "mihomo.exe")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !mayOpen(f, windows.FILE_WRITE_DATA) || !mayOpen(dir, fileAddFile) || !mayOpen(filepath.Join(dir, "missing"), windows.WRITE_DAC) {
		t.Error("a file of the user's, or a missing one, is taken as closed")
	}
	if protected(f) {
		t.Error("a copy in the user's own folder is taken as protected")
	}
	if windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("elevated: the system's files are open to this process")
	}
	sys, _ := windows.GetSystemDirectory()
	cmd := filepath.Join(sys, "cmd.exe")
	for _, r := range fileRights {
		if mayOpen(cmd, r) {
			t.Errorf("cmd.exe is open to %#x", r)
		}
	}
	for _, r := range parentRights {
		if mayOpen(sys, r) {
			t.Errorf("System32 is open to %#x", r)
		}
	}
}

func TestSameFile(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("bowel", 300000) // more than one buffer
	files := map[string]string{"a": big, "b": big, "c": big[:len(big)-1] + "!", "d": big + "!", "e": ""}
	for name, s := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p := func(n string) string { return filepath.Join(dir, n) }
	if !sameFile(p("a"), p("b")) || !sameFile(p("e"), p("e")) {
		t.Error("the same bytes differ")
	}
	if sameFile(p("a"), p("c")) || sameFile(p("a"), p("d")) || sameFile(p("a"), p("missing")) {
		t.Error("other bytes are the same")
	}
}

// startInstead starts what elevate would, as this user and with the fake
// roles' environment, and returns the process; it remembers what it was
// asked to start.
func startInstead(t *testing.T, asked *[]string) func(file, params, dir string) (windows.Handle, error) {
	return func(file, params, dir string) (windows.Handle, error) {
		*asked = append(*asked, file)
		c := exec.Command(file)
		c.SysProcAttr = &syscall.SysProcAttr{CmdLine: windows.EscapeArg(file) + " " + params}
		c.Env = fakeAs("core")
		c.Dir = dir
		c.Stdout, c.Stderr = io.Discard, io.Discard
		if err := c.Start(); err != nil {
			return 0, err
		}
		h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, uint32(c.Process.Pid))
		if err != nil {
			return 0, err
		}
		go c.Wait()
		return h, nil
	}
}

// fakeTun makes a %ProgramData% of the test's, with this user's TUN
// directory in it, and the bundled core src.
func fakeTun(t *testing.T) (bin, src string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("ProgramData", root)
	tmp := t.TempDir()
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)
	old := CoreDir
	CoreDir = t.TempDir()
	t.Cleanup(func() { CoreDir = old })
	self, _ := os.Executable()
	bin = Path("", "mihomo.exe")
	if !Installed(bin) {
		t.Fatalf("%s is not where TUN runs from", bin)
	}
	src = filepath.Join(t.TempDir(), "mihomo.exe")
	copyTo(t, self, src)
	return bin, src
}

func copyTo(t *testing.T, from, to string) {
	t.Helper()
	b, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, b, 0o755); err != nil {
		t.Fatal(err)
	}
}

func replace[T any](t *testing.T, p *T, v T) {
	old := *p
	*p = v
	t.Cleanup(func() { *p = old })
}

// A copy that is current and closed to the user: the launcher beside it
// is what the prompt elevates, it starts the core, and the app ends it.
func TestSpawnTheLauncher(t *testing.T) {
	bin, src := fakeTun(t)
	self, _ := os.Executable()
	copyTo(t, self, bin)
	copyTo(t, self, launcherPath(bin))
	if err := os.WriteFile(markPath(bin), []byte(markText+"\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var asked []string
	replace(t, &elevate, startInstead(t, &asked))
	replace(t, &guarded, func(string) bool { return true })

	task, err := Spawn(bin, src)
	if err != nil || task == nil {
		t.Fatalf("spawn: %v, %v", task, err)
	}
	if len(asked) != 1 || asked[0] != launcherPath(bin) {
		t.Errorf("elevated %q, want the launcher alone", asked)
	}
	if task.Pid() == 0 || gone(uint32(task.Pid()), 300*time.Millisecond) {
		t.Fatalf("no core running: %d", task.Pid())
	}
	if p := coreOf(snapshot(), task.root, "mihomo.exe"); p != uint32(task.Pid()) {
		t.Errorf("the core is %d, under the launcher is %d", task.Pid(), p)
	}
	done := make(chan error, 1)
	go func() { done <- task.Wait() }()
	task.Stop()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("wait: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the core was not ended")
	}
	if !gone(uint32(task.Pid()), 5*time.Second) {
		t.Error("the core outlived Stop")
	}
	task.Stop() // after the end: nothing to do, and no handle reused
	if err := task.Wait(); err != nil {
		t.Errorf("a second wait: %v", err)
	}

	// The bundled core is another program's: nothing is spawned for it.
	if task, err := Spawn(src, src); task != nil || err != nil {
		t.Errorf("spawned the bundled core: %v, %v", task, err)
	}
}

// Anything that is not a current, closed copy goes through cmd.exe,
// which makes the copy first; one the user could change is never what is
// elevated.
func TestStartCommand(t *testing.T) {
	bin, src := fakeTun(t)
	self, _ := os.Executable()
	args := launcherArgs(appRef{1, 2, 3}, bin, CoreDir)
	sys, _ := windows.GetSystemDirectory()
	cmd := filepath.Join(sys, "cmd.exe")
	makeCurrent := func() {
		copyTo(t, self, bin)
		copyTo(t, self, launcherPath(bin))
		copyTo(t, self, src)
		_ = os.WriteFile(markPath(bin), []byte(markText+"\n"), 0o644)
	}
	for _, c := range []struct {
		name    string
		prep    func()
		guarded bool
		cmd     bool
	}{
		{"current and closed", func() {}, true, false},
		{"open to the user", func() {}, false, true},
		{"nothing made yet", func() { _ = os.RemoveAll(filepath.Dir(bin)) }, true, true},
		{"an earlier version's mark", func() { _ = os.WriteFile(markPath(bin), []byte("683192c73d93601c\n"), 0o644) }, true, true},
		{"another core", func() { _ = os.WriteFile(src, []byte("a newer core"), 0o755) }, true, true},
		{"another launcher", func() { _ = os.WriteFile(launcherPath(bin), []byte("an older bowel"), 0o755) }, true, true},
	} {
		makeCurrent()
		c.prep()
		g := c.guarded
		replace(t, &guarded, func(string) bool { return g })
		file, params, log, err := startCommand(bin, src, args)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if c.cmd {
			if file != cmd || log != installLogPath() || !strings.HasPrefix(params, `/d /s /c "`) ||
				!strings.Contains(params, "echo [mark]") || !strings.Contains(params, `start "" /wait "`+launcherPath(bin)+`" tun-core --app 1:2:3 "`+bin+`"`) {
				t.Errorf("%s: %s %s", c.name, file, params)
			}
		} else if file != launcherPath(bin) || log != "" || params != windows.ComposeCommandLine(args) {
			t.Errorf("%s: %s %s", c.name, file, params)
		}
	}
}

// A start through cmd.exe that stops while making the copy says where,
// with what Windows said there. Run as this user, cmd.exe is refused the
// first change of an owner, which is the folder's part.
func TestSpawnReportsTheCopy(t *testing.T) {
	if windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("elevated: the command would make the copy, and delete this user's old tasks")
	}
	bin, src := fakeTun(t)
	var asked []string
	replace(t, &elevate, startInstead(t, &asked))
	task, err := Spawn(bin, src)
	if task != nil || err == nil {
		t.Fatalf("spawn: %v, %v", task, err)
	}
	if len(asked) != 1 || !strings.EqualFold(filepath.Base(asked[0]), "cmd.exe") {
		t.Errorf("elevated %q", asked)
	}
	if !strings.Contains(err.Error(), "停在「建立安装目录并设置权限」这一步") {
		t.Errorf("error %v", err)
	}
	t.Log(err)
	if _, err := os.Stat(launcherPath(bin)); err == nil {
		t.Error("the launcher was copied past a failed step")
	}
}

// A prompt the user dismissed starts nothing, and says so.
func TestSpawnCanceled(t *testing.T) {
	bin, src := fakeTun(t)
	replace(t, &elevate, func(string, string, string) (windows.Handle, error) { return 0, windows.ERROR_CANCELLED })
	task, err := Spawn(bin, src)
	if task != nil || !errors.Is(err, ErrCanceled) {
		t.Fatalf("spawn: %v, %v", task, err)
	}
	if m, _ := filepath.Glob(filepath.Join(stageDir(""), ".mihomo-*")); len(m) != 0 {
		t.Errorf("staged files left: %q", m)
	}
}

// A launcher that ends without a core is a start that failed, with what
// its exit code means.
func TestSpawnLauncherEnds(t *testing.T) {
	bin, src := fakeTun(t)
	self, _ := os.Executable()
	copyTo(t, self, bin)
	copyTo(t, self, launcherPath(bin))
	copyTo(t, self, src)
	_ = os.WriteFile(markPath(bin), []byte(markText+"\n"), 0o644)
	replace(t, &guarded, func(string) bool { return true })
	var asked []string
	start := startInstead(t, &asked)
	// The launcher is given an app that is not this one.
	replace(t, &elevate, func(file, params, dir string) (windows.Handle, error) {
		return start(file, strings.Replace(params, "--app "+strconv.Itoa(os.Getpid())+":", "--app 1:", 1), dir)
	})
	task, err := Spawn(bin, src)
	if task != nil || err == nil || !strings.Contains(err.Error(), "退出码 "+strconv.Itoa(launchNoApp)) {
		t.Fatalf("spawn: %v, %v", task, err)
	}
}
