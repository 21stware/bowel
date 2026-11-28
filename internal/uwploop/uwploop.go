// Package uwploop lets Windows' packaged apps (UWP: the Store, Mail,
// Weather, Xbox and the like) reach the system proxy.
//
// Those apps run in an AppContainer, which Windows keeps from connecting
// to the computer itself: to 127.0.0.1, or any of its own addresses. The
// system proxy bowel sets points there, so with it on they lose the
// network (issue #320). Windows lifts that one container at a time, for
// those on its loopback exemption list; CheckNetIsolation.exe is its own
// tool for changing it. Some users may change it as they are, others
// only as an administrator.
//
// Everything the user asks to allow is added at once: as the user first,
// then, for what that did not add, behind one UAC prompt. What bowel
// added is kept in a record, so that undoing takes away those and leaves
// the ones someone else exempted.
package uwploop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/oboo/moho/internal/i18n"
)

// ErrCanceled is a UAC prompt the user dismissed: nothing changed, and
// there is nothing to say.
var ErrCanceled = errors.New("uwploop: canceled")

// Status is how many of the computer's packaged apps may reach the proxy.
type Status struct {
	Total  int // the app containers there are
	Exempt int // those of them on the exemption list
	Added  int // those of them bowel put there, and can take away
}

// All is whether there is nothing left to allow.
func (s Status) All() bool { return s.Exempt >= s.Total }

// sidRE is an AppContainer's SID, S-1-15-2-… or a capability's
// S-1-15-3-…: what alone may be written for the elevated command.
var sidRE = regexp.MustCompile(`^S-1-15-[23](-\d{1,10}){1,15}$`)

func validSID(s string) bool { return sidRE.MatchString(s) }

// status counts the containers in all, and those of them in exempt and
// in added.
func status(all, exempt, added []string) Status {
	st := Status{Total: len(all)}
	for _, s := range all {
		if slices.Contains(exempt, s) {
			st.Exempt++
			if slices.Contains(added, s) {
				st.Added++
			}
		}
	}
	return st
}

// missing is what of all is not in exempt: what allowing them adds.
func missing(all, exempt []string) []string {
	var out []string
	for _, s := range all {
		if !slices.Contains(exempt, s) && validSID(s) {
			out = append(out, s)
		}
	}
	return out
}

// both is what of a is also in b.
func both(a, b []string) []string {
	var out []string
	for _, s := range a {
		if slices.Contains(b, s) {
			out = append(out, s)
		}
	}
	return out
}

// union is a and what of b it lacks.
func union(a, b []string) []string {
	out := slices.Clone(a)
	for _, s := range b {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// readRecord is what bowel added, as the record at path has it; none when
// there is no record.
func readRecord(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var r struct {
		Added []string `json:"added"`
	}
	if json.Unmarshal(b, &r) != nil {
		return nil
	}
	return slices.DeleteFunc(r.Added, func(s string) bool { return !validSID(s) })
}

// writeRecord keeps added at path, and removes the record when it is
// empty.
func writeRecord(path string, added []string) error {
	if len(added) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	b, err := json.MarshalIndent(struct {
		Added []string `json:"added"`
	}{added}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// script is the cmd.exe command that runs tool (CheckNetIsolation.exe)
// on each SID in the file list, one a line, adding it to the exemption
// list or, when add is false, taking it away. The SIDs are read by the
// loop, not written into the command: what a for variable stands for is
// not parsed again, and there can be more of them than a command line
// holds.
func script(tool, list string, add bool) (string, error) {
	for _, p := range []string{tool, list} {
		if strings.ContainsAny(p, "\"%\r\n") {
			return "", fmt.Errorf(i18n.T("路径里有不能使用的字符：%s", "The path has a character that can't be used: %s"), p)
		}
	}
	op := "-a"
	if !add {
		op = "-d"
	}
	return fmt.Sprintf(`for /f "usebackq delims=" %%s in ("%s") do @"%s" LoopbackExempt %s -p=%%s >nul 2>&1`, list, tool, op), nil
}

// What the system does, which the tests stand in for.
var (
	containers func() ([]string, error)         // the app containers' SIDs
	exempted   func() ([]string, error)         // the SIDs on the exemption list
	direct     func(sid string, add bool) error // runs CheckNetIsolation on one SID, as the user
	elevate    func(cmd string) error           // runs a cmd.exe command after UAC's prompt
	tool       func() string                    // CheckNetIsolation.exe's path
)

// Read is how many packaged apps may reach the proxy, with record the
// file that says which bowel added.
func Read(record string) (Status, error) {
	if !Supported {
		return Status{}, nil
	}
	all, err := containers()
	if err != nil {
		return Status{}, err
	}
	ex, err := exempted()
	if err != nil {
		return Status{}, err
	}
	return status(all, ex, readRecord(record)), nil
}

// AllowAll adds every packaged app not yet on the exemption list, and
// keeps in record those it added. ErrCanceled when the user dismissed
// the UAC prompt; what was added before it is kept all the same.
func AllowAll(record, tmpDir string) error {
	all, err := containers()
	if err != nil {
		return err
	}
	ex, err := exempted()
	if err != nil {
		return err
	}
	want := missing(all, ex)
	if len(want) == 0 {
		return nil
	}
	cerr := change(want, true, tmpDir)
	after, err := exempted()
	if err != nil {
		return err
	}
	got := both(want, after)
	if err := writeRecord(record, union(readRecord(record), got)); err != nil {
		return err
	}
	if cerr != nil {
		return cerr
	}
	if n := len(want) - len(got); n > 0 {
		return fmt.Errorf(i18n.T("有 %d 个应用没能加入例外", "%d apps could not be exempted"), n)
	}
	return nil
}

// Undo takes away from the exemption list what bowel added, and leaves
// the rest.
func Undo(record, tmpDir string) error {
	ex, err := exempted()
	if err != nil {
		return err
	}
	ours := readRecord(record)
	var cerr error
	if drop := both(ours, ex); len(drop) > 0 {
		cerr = change(drop, false, tmpDir)
		if ex, err = exempted(); err != nil {
			return err
		}
	}
	left := both(ours, ex)
	if err := writeRecord(record, left); err != nil {
		return err
	}
	if cerr != nil {
		return cerr
	}
	if len(left) > 0 {
		return fmt.Errorf(i18n.T("有 %d 个应用没能移出例外", "%d apps could not be taken off the exemptions"), len(left))
	}
	return nil
}

// change adds sids to the exemption list, or takes them away. Windows
// lets some users change it as they are, without elevation, so that is
// tried first, and only what it did not change is asked for in a UAC
// prompt: there CheckNetIsolation reads the SIDs from a file in tmpDir.
func change(sids []string, add bool, tmpDir string) error {
	for _, s := range sids {
		_ = direct(s, add) // whether it did is read from the list
	}
	ex, err := exempted()
	if err != nil {
		return err
	}
	var left []string
	for _, s := range sids {
		if slices.Contains(ex, s) != add {
			left = append(left, s)
		}
	}
	if len(left) == 0 {
		return nil
	}
	f, err := os.CreateTemp(tmpDir, "bowel-uwp-*.txt")
	if err != nil {
		return err
	}
	list := f.Name()
	defer os.Remove(list)
	_, err = f.WriteString(strings.Join(left, "\r\n") + "\r\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	cmd, err := script(tool(), list, add)
	if err != nil {
		return err
	}
	return elevate(cmd)
}
