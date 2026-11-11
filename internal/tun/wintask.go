package tun

// What Windows' installation is made of, apart from the calls that only
// Windows has, so that every system's tests read it.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
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
const taskRev = "1"

// mark is what the installation leaves beside dst: a digest of everything
// the task was registered with.
func mark(dst, coreDir, sid string) string {
	h := sha256.Sum256([]byte(strings.Join([]string{taskRev, strings.ToLower(dst), strings.ToLower(coreDir), sid}, "\x00")))
	return hex.EncodeToString(h[:8])
}

// The directory's owner and its only writers are administrators and
// SYSTEM; users read and run what is in it.
const dirACL = `*S-1-5-32-544:(OI)(CI)F *S-1-5-18:(OI)(CI)F *S-1-5-32-545:(OI)(CI)RX`

// winInstallStep is the elevated command. The copy in use may be
// running, and a running program's file can be renamed but not written:
// it steps aside as .old, which the next installation deletes. The copy
// to install is the step's first path, as Install expects.
func winInstallStep(tmp, dst, xml, task, mark string) Step {
	dir := winDir(dst)
	old := dst + ".old"
	return Step{
		"(if not exist ", tmp, " exit /b 1) && (if not exist ", dir, " mkdir ", dir,
		") && icacls ", dir, " /setowner *S-1-5-32-544 /T /C >nul && icacls ", dir, " /inheritance:r /grant:r " + dirACL + " >nul" +
			" && (del /f /q ", old, " 2>nul & ver >nul) && (if exist ", dst, " move /y ", dst, " ", old,
		" >nul) && copy /y /b ", tmp, " ", dst, " >nul && schtasks /Create /F /TN ", task, " /XML ", xml,
		" >nul && (echo " + mark + ")> ", markPath(dst),
	}
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

// taskXML is the task's definition. It runs as the user without their
// password being kept (S4U), so it has no window to show, at the highest
// run level the user has; it has no trigger, starts only on demand, and
// is never ended for running long or on battery. The core is the action
// itself, with nothing in between that could set its environment: ending
// the task ends that one process, and a shell around the core would be
// ended in its place. What the description tells the user to delete is
// what Uninstall deletes.
func taskXML(sid, bin, coreDir string) string {
	args := fmt.Sprintf(`-d "%s" -f "%s"`, coreDir, coreDir+`\config.yaml`)
	return `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>bowel: runs the mihomo core with the rights TUN mode needs. To undo it, delete this task and the folder ` + xmlText(winDir(bin)) + `</Description>
  </RegistrationInfo>
  <Triggers />
  <Principals>
    <Principal id="Author">
      <UserId>` + xmlText(sid) + `</UserId>
      <LogonType>S4U</LogonType>
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
      <Command>` + xmlText(bin) + `</Command>
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
