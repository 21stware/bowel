package procinfo

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// running walks a Toolhelp snapshot for the processes of this logon
// session, by their paths: the services of session 0 are no one's apps.
// Those the session has of the system's own (csrss.exe, dwm.exe,
// svchost.exe, all under %SystemRoot%) are left out, and so are those that
// can't be opened, which are protected.
func running() []string {
	var own uint32
	if err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &own); err != nil {
		return nil
	}
	sys, err := windows.GetSystemWindowsDirectory()
	if err != nil || sys == "" {
		return nil
	}
	sys = strings.ToLower(strings.TrimRight(sys, `\`)) + `\`
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var out []string
	e := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		var s uint32
		if windows.ProcessIdToSessionId(e.ProcessID, &s) != nil || s != own {
			continue
		}
		if p := imagePath(e.ProcessID); p != "" && !strings.HasPrefix(strings.ToLower(p), sys) {
			out = append(out, p)
		}
	}
	return out
}

// imagePath is a process's executable, "" when it can't be opened.
func imagePath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}
