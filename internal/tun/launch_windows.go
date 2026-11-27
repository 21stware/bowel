package tun

import (
	"errors"
	"os"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/oboo/moho/internal/nowindow"
)

// Launch is what the app does when it is run as the launcher, elevated
// through the prompt a start of TUN's core shows (see Spawn): args are
// what followed LauncherArg, and what it returns is the exit code.
//
// It starts the core, without a console window, for the app the
// arguments name, and ends it when the app sets the event it named or
// is gone: the app's handle on an elevated process may lack the right to
// end it, and a crash of the app must not leave behind an elevated core
// that holds the ports and the adapter. It first puts itself in a job
// that ends its processes when its last handle is closed, which is this
// process's own: the core is started in the same job, as every process a
// job's process starts is, and goes with the launcher however that ends.
// Without the job no core is started: one that cannot be ended is worse
// than none, and the app then runs the bundled core and says why.
//
// It starts no core but the one beside it, in the directory only
// administrators write, and takes nothing from the app but the event's
// one meaning: there is no way for another program to steer it.
//
// With launcherKill it ends the processes running from a path instead,
// for the installation.
func Launch(args []string) int {
	l, ok := launchArgs(args)
	if !ok {
		return launchBadArgs
	}
	if l.kill != "" {
		killImage(l.kill)
		return 0
	}
	self, err := os.Executable()
	if err != nil || !besideLauncher(l.core, self) {
		return launchNotBeside
	}
	app, stop, err := openApp(l.app)
	if err != nil {
		return launchNoApp
	}
	defer windows.CloseHandle(app)
	defer windows.CloseHandle(stop)
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return launchNoJob
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		return launchNoJob
	}
	if err := windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
		return launchNoJob
	}
	core := nowindow.Command(l.core, l.args...)
	if err := core.Start(); err != nil {
		return launchNoCore
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(core.Process.Pid))
	if err != nil {
		_ = core.Process.Kill()
		return launchNoCore
	}
	defer windows.CloseHandle(h)
	ev, err := windows.WaitForMultipleObjects([]windows.Handle{h, app, stop}, false, windows.INFINITE)
	if err == nil && ev == windows.WAIT_OBJECT_0 { // the core ended by itself
		var exit *exec.ExitError
		switch err := core.Wait(); {
		case err == nil:
			return 0
		case errors.As(err, &exit):
			return exit.ExitCode()
		}
		return launchNoCore
	}
	// The app asked, or is gone. A core that outlasts the wait goes with
	// the job when this process ends.
	if windows.TerminateProcess(h, 1) == nil {
		_, _ = windows.WaitForSingleObject(h, 5000)
	}
	return 0
}

// openApp opens the process app names, and copies its stop event out of
// it. A process with the same id that was created at another time is not
// the app, and neither is a missing one. When the prompt was answered
// with another account's password (a standard user's), this process is
// that account's, which the app's process does not let in: an
// administrator's debug privilege does.
func openApp(app appRef) (proc, stop windows.Handle, err error) {
	const access = windows.SYNCHRONIZE | windows.PROCESS_DUP_HANDLE | windows.PROCESS_QUERY_LIMITED_INFORMATION
	proc, err = windows.OpenProcess(access, false, app.pid)
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) && enableDebug() == nil {
		proc, err = windows.OpenProcess(access, false, app.pid)
	}
	if err != nil {
		return 0, 0, err
	}
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(proc, &created, &exited, &kernel, &user); err != nil || filetime(created) != app.created {
		windows.CloseHandle(proc)
		return 0, 0, errors.New("not the app")
	}
	if err := windows.DuplicateHandle(proc, windows.Handle(app.stop), windows.CurrentProcess(), &stop, windows.SYNCHRONIZE, false, 0); err != nil {
		windows.CloseHandle(proc)
		return 0, 0, err
	}
	return proc, stop, nil
}

func filetime(t windows.Filetime) uint64 { return uint64(t.HighDateTime)<<32 | uint64(t.LowDateTime) }

// enableDebug turns on the debug privilege this process's token has,
// unused, as an administrator's elevated one does.
func enableDebug() error {
	var tok windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &tok); err != nil {
		return err
	}
	defer tok.Close()
	name, err := windows.UTF16PtrFromString("SeDebugPrivilege")
	if err != nil {
		return err
	}
	p := windows.Tokenprivileges{PrivilegeCount: 1}
	if err := windows.LookupPrivilegeValue(nil, name, &p.Privileges[0].Luid); err != nil {
		return err
	}
	p.Privileges[0].Attributes = windows.SE_PRIVILEGE_ENABLED
	// Without the privilege this succeeds all the same, and opening the
	// app fails again.
	return windows.AdjustTokenPrivileges(tok, false, &p, 0, nil, nil)
}

// killImage ends every process whose program is bin (see sameImage), and
// waits a moment for each to be gone: the installation replaces the file
// next.
func killImage(bin string) {
	for _, pid := range running(bin) {
		h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(pid))
		if err != nil {
			continue
		}
		if windows.TerminateProcess(h, 1) == nil {
			_, _ = windows.WaitForSingleObject(h, 5000)
		}
		windows.CloseHandle(h)
	}
}
