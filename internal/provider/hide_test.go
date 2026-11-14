package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// A node the user removes stays out of what bowel reads of a subscription,
// through its updates and a restart, and comes back when restored (#160).
func TestHiddenNodesSurviveUpdates(t *testing.T) {
	body := "proxies:\n  - {name: a, type: ss, server: a.example, port: 1}\n  - {name: b, type: ss, server: b.example, port: 1}\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
	defer srv.Close()
	dir := t.TempDir()
	s, _ := Open(dir)
	p, err := s.Add(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if p, err = s.Hide(p.ID, "a", true); err != nil || !slices.Equal(p.Hidden, []string{"a"}) {
		t.Fatalf("hide: %+v %v", p, err)
	}
	if _, err = s.Hide(p.ID, "a", true); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get(p.ID); !slices.Equal(got.Hidden, []string{"a"}) {
		t.Fatalf("hiding twice: %v", got.Hidden)
	}
	if _, err := s.Update(context.Background(), srv.Client(), p.ID); err != nil {
		t.Fatal(err)
	}
	s, _ = Open(dir) // after a restart too
	if got := s.Names(p.ID); !slices.Equal(got, []string{"b"}) {
		t.Fatalf("names = %v", got)
	}
	if got := s.Servers(p.ID); len(got) != 1 || got["b"] != "b.example" {
		t.Fatalf("servers = %v", got)
	}
	if p, _ = s.Hide(p.ID, "a", false); p.Hidden != nil {
		t.Fatalf("restored: %v", p.Hidden)
	}
	if got := s.Names(p.ID); len(got) != 2 {
		t.Fatalf("names after restore = %v", got)
	}
	if _, err := s.Hide("nope", "a", true); err == nil {
		t.Fatal("hiding on no provider")
	}
}

// What List, Get and Snapshot handed out stays as it was when a node is
// restored or removed: Hide replaces the list.
func TestHideLeavesHandedOutProvidersAlone(t *testing.T) {
	s, _ := Open(t.TempDir())
	p, err := s.create(Provider{Kind: Link, Name: "x", Enabled: true}, []byte("trojan://p@h:443#a"))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"a", "b", "c"} {
		if _, err := s.Hide(p.ID, n, true); err != nil {
			t.Fatal(err)
		}
	}
	items, _ := s.Snapshot()
	got, _ := s.Get(p.ID)
	listed := s.List()
	if _, err := s.Hide(p.ID, "a", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Hide(p.ID, "d", true); err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "b", "c"}
	for _, h := range [][]string{items[0].Hidden, got.Hidden, listed[0].Hidden} {
		if !slices.Equal(h, want) {
			t.Errorf("handed out = %q", h)
		}
	}
	if now, _ := s.Get(p.ID); !slices.Equal(now.Hidden, []string{"b", "c", "d"}) {
		t.Errorf("now = %q", now.Hidden)
	}
}
