package tun

import (
	"encoding/xml"
	"strings"
	"testing"
	"unicode/utf16"
)

// The task runs the protected copy with the app's own arguments, as the
// user, elevated, with no trigger; and the definition is well-formed XML
// whatever the paths hold.
func TestTaskXML(t *testing.T) {
	x := taskXML("S-1-5-21-1-2-3-1001", `C:\ProgramData\bowel\tun\S-1-5-21-1-2-3-1001\mihomo.exe`, `C:\Users\张 & 三\AppData\Roaming\bowel\core`)
	var task struct {
		Principals struct {
			Principal struct {
				UserId    string
				LogonType string
				RunLevel  string
			}
		}
		Triggers struct {
			Inner string `xml:",innerxml"`
		}
		Settings struct {
			ExecutionTimeLimit         string
			AllowStartOnDemand         bool
			DisallowStartIfOnBatteries bool
			StopIfGoingOnBatteries     bool
			MultipleInstancesPolicy    string
		}
		Actions struct {
			Exec struct {
				Command          string
				Arguments        string
				WorkingDirectory string
			}
		}
	}
	// The declaration names the file's encoding, not this string's.
	body := strings.TrimPrefix(x, `<?xml version="1.0" encoding="UTF-16"?>`)
	if body == x {
		t.Fatal("no UTF-16 declaration")
	}
	if err := xml.Unmarshal([]byte(body), &task); err != nil {
		t.Fatalf("not XML: %v\n%s", err, x)
	}
	p := task.Principals.Principal
	if p.UserId != "S-1-5-21-1-2-3-1001" || p.LogonType != "S4U" || p.RunLevel != "HighestAvailable" {
		t.Errorf("principal %+v", p)
	}
	e := task.Actions.Exec
	if e.Command != `C:\ProgramData\bowel\tun\S-1-5-21-1-2-3-1001\mihomo.exe` ||
		e.Arguments != `-d "C:\Users\张 & 三\AppData\Roaming\bowel\core" -f "C:\Users\张 & 三\AppData\Roaming\bowel\core\config.yaml"` ||
		e.WorkingDirectory != `C:\Users\张 & 三\AppData\Roaming\bowel\core` {
		t.Errorf("action %+v", e)
	}
	s := task.Settings
	if s.ExecutionTimeLimit != "PT0S" || !s.AllowStartOnDemand || s.DisallowStartIfOnBatteries || s.StopIfGoingOnBatteries || s.MultipleInstancesPolicy != "IgnoreNew" {
		t.Errorf("settings %+v", s)
	}
	if strings.TrimSpace(task.Triggers.Inner) != "" {
		t.Errorf("the task has a trigger: %q", task.Triggers.Inner)
	}
}

func TestUTF16File(t *testing.T) {
	b := utf16File("a\n张")
	if len(b) != 2+2*4 || b[0] != 0xff || b[1] != 0xfe {
		t.Fatalf("bytes % x", b)
	}
	u := make([]uint16, 0, 4)
	for i := 2; i < len(b); i += 2 {
		u = append(u, uint16(b[i])|uint16(b[i+1])<<8)
	}
	if got := string(utf16.Decode(u)); got != "a\r\n张" {
		t.Errorf("decoded %q", got)
	}
}

// The elevated command closes the directory before anything is put in
// it, moves a copy in use aside instead of writing over it, and leaves
// the mark last, once everything else has worked.
func TestWinInstallStep(t *testing.T) {
	dst := `C:\ProgramData\bowel\tun\S-1\mihomo.exe`
	st := winInstallStep(`C:\Users\a b\Temp\bowel-tun\.mihomo-1`, dst, `C:\Users\a b\Temp\bowel-tun\.mihomo-task.xml`, "bowel-tun-S-1", "abcd")
	if len(st)%2 != 0 || st[len(st)-1] != `C:\ProgramData\bowel\tun\S-1\installed` {
		t.Fatalf("step doesn't end on the mark: %q", st)
	}
	got, err := winScript([]Step{st})
	if err != nil {
		t.Fatal(err)
	}
	if st[1] != `C:\Users\a b\Temp\bowel-tun\.mihomo-1` {
		t.Errorf("the copy is not the first path: %q", st[1])
	}
	want := `(if not exist "C:\Users\a b\Temp\bowel-tun\.mihomo-1" exit /b 1)` +
		` && (if not exist "C:\ProgramData\bowel\tun\S-1" mkdir "C:\ProgramData\bowel\tun\S-1")` +
		` && icacls "C:\ProgramData\bowel\tun\S-1" /setowner *S-1-5-32-544 /T /C >nul` +
		` && icacls "C:\ProgramData\bowel\tun\S-1" /inheritance:r /grant:r *S-1-5-32-544:(OI)(CI)F *S-1-5-18:(OI)(CI)F *S-1-5-32-545:(OI)(CI)RX >nul` +
		` && (del /f /q "C:\ProgramData\bowel\tun\S-1\mihomo.exe.old" 2>nul & ver >nul)` +
		` && (if exist "C:\ProgramData\bowel\tun\S-1\mihomo.exe" move /y "C:\ProgramData\bowel\tun\S-1\mihomo.exe" "C:\ProgramData\bowel\tun\S-1\mihomo.exe.old" >nul)` +
		` && copy /y /b "C:\Users\a b\Temp\bowel-tun\.mihomo-1" "C:\ProgramData\bowel\tun\S-1\mihomo.exe" >nul` +
		` && schtasks /Create /F /TN "bowel-tun-S-1" /XML "C:\Users\a b\Temp\bowel-tun\.mihomo-task.xml" >nul` +
		` && (echo abcd)> "C:\ProgramData\bowel\tun\S-1\installed"`
	if got != want {
		t.Fatalf("script:\n got %s\nwant %s", got, want)
	}
	// Only administrators and SYSTEM get more than reading and running.
	if strings.Count(dirACL, "F") != 2 || !strings.Contains(dirACL, "*S-1-5-32-545:(OI)(CI)RX") {
		t.Errorf("ACL %s", dirACL)
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

// The mark changes with anything the task was registered with, and not
// with the case of a path.
func TestMark(t *testing.T) {
	m := mark(`C:\P\mihomo.exe`, `C:\U\core`, "S-1")
	if len(m) != 16 || m != mark(`c:\p\MIHOMO.exe`, `C:\u\core`, "S-1") {
		t.Errorf("mark %q", m)
	}
	for _, o := range []string{mark(`C:\P2\mihomo.exe`, `C:\U\core`, "S-1"), mark(`C:\P\mihomo.exe`, `C:\U2\core`, "S-1"), mark(`C:\P\mihomo.exe`, `C:\U\core`, "S-2")} {
		if o == m {
			t.Error("the mark is the same for another installation")
		}
	}
}
