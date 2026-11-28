//go:build windows

package ruleset

import (
	"os"
	"testing"
	"time"
)

// TestSaveWhileTheCoreReads: on Windows, while the core has a set's file
// open, a rename onto it and a remove of it are refused. An update and a
// Replace wait for the core to let go instead of failing.
func TestSaveWhileTheCoreReads(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	save := func(id, domain string) {
		t.Helper()
		set, err := Parse([]byte("payload:\n  - DOMAIN-SUFFIX," + domain + "\n"))
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Save(id, &Checked{Set: set}); err != nil {
			t.Fatal(err)
		}
	}
	hold := func(id string) {
		t.Helper()
		f, err := os.Open(s.Path(id))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = f.Close() })
		time.AfterFunc(200*time.Millisecond, func() { _ = f.Close() })
	}
	save("kept", "old.example")
	save("gone", "gone.example")

	hold("kept")
	save("kept", "new.example")
	if _, ok := s.Match("kept", "a.new.example", ""); !ok {
		t.Fatal("the update did not take")
	}

	meta, files := s.Snapshot(map[string]bool{"kept": true})
	hold("kept")
	hold("gone")
	if err := s.Replace(meta, files); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Path("gone")); !os.IsNotExist(err) {
		t.Errorf("the removed set's file stayed: %v", err)
	}
	if _, ok := s.Match("kept", "a.new.example", ""); !ok {
		t.Error("the kept set does not match")
	}
}
