//go:build windows

package provider

import (
	"os"
	"testing"
	"time"
)

// TestReplaceWhileTheCoreReads: on Windows, while the core has a provider's
// file open, a rename onto it and a remove of it are refused. Replace waits
// for the core to let go instead of failing, as an import or its undo did.
func TestReplaceWhileTheCoreReads(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	kept, err := s.create(Provider{Kind: Link, Name: "kept", Enabled: true}, []byte("trojan://x@old.example:443#old"))
	if err != nil {
		t.Fatal(err)
	}
	gone, err := s.create(Provider{Kind: Link, Name: "gone", Enabled: true}, []byte("trojan://x@gone.example:443#gone"))
	if err != nil {
		t.Fatal(err)
	}
	var held []*os.File
	for _, id := range []string{kept.ID, gone.ID} {
		f, err := os.Open(s.Path(id))
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, f)
	}
	release := func() {
		for _, f := range held {
			_ = f.Close()
		}
	}
	t.Cleanup(release)
	go func() {
		time.Sleep(200 * time.Millisecond)
		release()
	}()

	if err := s.Replace([]Provider{kept}, map[string][]byte{kept.ID: []byte("trojan://x@new.example:443#new")}); err != nil {
		t.Fatal(err)
	}
	if c, _ := s.Content(kept.ID); string(c) != "trojan://x@new.example:443#new" {
		t.Errorf("the kept provider's file: %q", c)
	}
	if _, err := os.Stat(s.Path(gone.ID)); !os.IsNotExist(err) {
		t.Errorf("the removed provider's file stayed: %v", err)
	}
}
