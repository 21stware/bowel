package procinfo

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// running walks a Toolhelp snapshot for the processes of this logon
// session, by their file names: the services of session 0 are no one's
// apps.
func running() []string {
	var own uint32
	if err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &own); err != nil {
		return nil
	}
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
		if name := windows.UTF16ToString(e.ExeFile[:]); name != "" {
			out = append(out, name)
		}
	}
	return out
}
