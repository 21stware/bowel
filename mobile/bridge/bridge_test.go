package bridge

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oboo/moho/internal/mihomo"
)

// The core lists a provider's nodes, and their tests, only under the provider.
func TestRefreshKeepsNodeTests(t *testing.T) {
	dir, err := os.MkdirTemp("", "b")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, socketName)
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	const node = "东京 01"
	answer := func(path string, v any) (string, http.HandlerFunc) {
		return path, func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(v) }
	}
	mux := http.NewServeMux()
	mux.HandleFunc(answer("/proxies", map[string]any{"proxies": map[string]any{"默认": map[string]any{"name": "默认", "type": "Selector", "now": node}}}))
	mux.HandleFunc(answer("/providers/proxies", map[string]any{"providers": map[string]any{"p-a": map[string]any{
		"name":    "p-a",
		"proxies": []any{map[string]any{"name": node, "type": "Trojan", "history": []any{map[string]any{"time": time.Now(), "delay": 88}}}},
	}}}))
	srv := &http.Server{Handler: mux}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })

	set, err := openSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	x := &bridge{client: mihomo.NewUnix(sock), set: set, lastNode: map[string]string{}}
	if err := x.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	n := nodeView(node, "a", x.proxies[node])
	if n.Delay != 88 || len(n.H) != 1 {
		t.Fatalf("node = %+v, want its test of 88 ms", n)
	}
	if n.Where.Country != "JP" {
		t.Fatalf("where = %+v, want JP from the name", n.Where)
	}
	if x.proxies["默认"].Now != node {
		t.Fatalf("the groups were lost: %+v", x.proxies)
	}
}
