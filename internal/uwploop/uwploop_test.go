package uwploop

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const (
	sidA = "S-1-15-2-1-2-3-4-5-6-7"
	sidB = "S-1-15-2-11-12-13-14-15-16-17"
	sidC = "S-1-15-2-21-22-23-24-25-26-27"
)

// fakeSystem stands in for Windows: the containers there are, the
// exemption list, CheckNetIsolation run as the user, which changes it
// only where asUser, and an elevated command that does to it what the
// script says, unless the user cancels or some SIDs are refused.
type fakeSystem struct {
	all, ex []string
	asUser  bool
	cancel  bool
	refuse  []string
	ran     []string // the elevated commands: one per UAC prompt
}

var lineRE = regexp.MustCompile(`^for /f "usebackq delims=" %s in \("([^"]+)"\) do @"[^"]+" LoopbackExempt (-a|-d) -p=%s >nul 2>&1$`)

func (f *fakeSystem) apply(sid string, add bool) {
	if slices.Contains(f.refuse, sid) {
		return
	}
	if add && !slices.Contains(f.ex, sid) {
		f.ex = append(f.ex, sid)
	}
	if !add {
		f.ex = slices.DeleteFunc(f.ex, func(s string) bool { return s == sid })
	}
}

func (f *fakeSystem) install(t *testing.T) {
	t.Helper()
	oc, oe, od, ol, ot := containers, exempted, direct, elevate, tool
	t.Cleanup(func() { containers, exempted, direct, elevate, tool = oc, oe, od, ol, ot })
	containers = func() ([]string, error) { return slices.Clone(f.all), nil }
	exempted = func() ([]string, error) { return slices.Clone(f.ex), nil }
	tool = func() string { return `C:\Windows\System32\CheckNetIsolation.exe` }
	direct = func(sid string, add bool) error {
		if !f.asUser {
			return errors.New("access denied")
		}
		f.apply(sid, add)
		return nil
	}
	elevate = func(cmd string) error {
		f.ran = append(f.ran, cmd)
		if f.cancel {
			return ErrCanceled
		}
		m := lineRE.FindStringSubmatch(cmd)
		if m == nil {
			t.Fatalf("command not as expected: %s", cmd)
		}
		file, err := os.Open(m[1])
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		sc := bufio.NewScanner(file)
		for sc.Scan() {
			if sid := strings.TrimSpace(sc.Text()); sid != "" {
				f.apply(sid, m[2] == "-a")
			}
		}
		return nil
	}
}

// Where the user may change the list as they are, nothing asks for an
// administrator.
func TestAllowAllAsUserAsksNothing(t *testing.T) {
	dir := t.TempDir()
	rec := filepath.Join(dir, "uwp.json")
	f := &fakeSystem{all: []string{sidA, sidB}, asUser: true}
	f.install(t)
	if err := AllowAll(rec, dir); err != nil {
		t.Fatal(err)
	}
	if err := Undo(rec, dir); err != nil {
		t.Fatal(err)
	}
	if len(f.ran) != 0 || len(f.ex) != 0 {
		t.Fatalf("prompts %d, exempt after undo %v", len(f.ran), f.ex)
	}
}

func TestAllowAllThenUndoLeavesOthersExemptions(t *testing.T) {
	dir := t.TempDir()
	rec := filepath.Join(dir, "uwp.json")
	f := &fakeSystem{all: []string{sidA, sidB, sidC}, ex: []string{sidB}} // sidB is someone else's
	f.install(t)

	if err := AllowAll(rec, dir); err != nil {
		t.Fatal(err)
	}
	if got := readRecord(rec); !slices.Equal(got, []string{sidA, sidC}) {
		t.Fatalf("record = %v, want the two bowel added", got)
	}
	st := status(f.all, f.ex, readRecord(rec))
	if st != (Status{Total: 3, Exempt: 3, Added: 2}) || !st.All() {
		t.Fatalf("status = %+v", st)
	}
	// Nothing left to allow: no second prompt.
	if err := AllowAll(rec, dir); err != nil || len(f.ran) != 1 {
		t.Fatalf("err %v, ran %d commands", err, len(f.ran))
	}

	if err := Undo(rec, dir); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.ex, []string{sidB}) {
		t.Fatalf("exempt after undo = %v, want only the one bowel didn't add", f.ex)
	}
	if _, err := os.Stat(rec); !os.IsNotExist(err) {
		t.Fatalf("record should be gone after undo: %v", err)
	}
	// The SID lists are temporary.
	if left, _ := filepath.Glob(filepath.Join(dir, "bowel-uwp-*")); len(left) != 0 {
		t.Fatalf("left behind: %v", left)
	}
}

func TestAllowAllCanceledChangesNothing(t *testing.T) {
	dir := t.TempDir()
	rec := filepath.Join(dir, "uwp.json")
	f := &fakeSystem{all: []string{sidA}, cancel: true}
	f.install(t)
	if err := AllowAll(rec, dir); err != ErrCanceled {
		t.Fatalf("err = %v, want ErrCanceled", err)
	}
	if len(f.ex) != 0 || readRecord(rec) != nil {
		t.Fatalf("exempt %v, record %v", f.ex, readRecord(rec))
	}
}

func TestAllowAllSaysWhatWasRefused(t *testing.T) {
	dir := t.TempDir()
	rec := filepath.Join(dir, "uwp.json")
	f := &fakeSystem{all: []string{sidA, sidB}, refuse: []string{sidB}}
	f.install(t)
	if err := AllowAll(rec, dir); err == nil || !strings.Contains(err.Error(), "1") {
		t.Fatalf("err = %v, want one refused", err)
	}
	// What did go on is still bowel's to undo.
	if got := readRecord(rec); !slices.Equal(got, []string{sidA}) {
		t.Fatalf("record = %v", got)
	}
}

func TestScriptRefusesPathsCmdWouldReparse(t *testing.T) {
	for _, p := range []string{`C:\a"b\list.txt`, `C:\%TEMP%\list.txt`, "C:\\a\nb"} {
		if _, err := script(`C:\Windows\System32\CheckNetIsolation.exe`, p, true); err == nil {
			t.Errorf("%q: want an error", p)
		}
	}
	cmd, err := script(`C:\Windows\System32\CheckNetIsolation.exe`, `C:\Users\Jo Doe\AppData\Local\Temp\l.txt`, false)
	if err != nil || !lineRE.MatchString(cmd) || !strings.Contains(cmd, " -d ") {
		t.Fatalf("cmd = %q, err %v", cmd, err)
	}
}

func TestRecordKeepsOnlySIDs(t *testing.T) {
	rec := filepath.Join(t.TempDir(), "uwp.json")
	if err := os.WriteFile(rec, []byte(`{"added":["`+sidA+`","x & calc","S-1-5-18"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readRecord(rec); !slices.Equal(got, []string{sidA}) {
		t.Fatalf("record = %v", got)
	}
}
