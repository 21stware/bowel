package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type barOut struct {
	Text    string
	Alt     string
	Tooltip string
	Class   []string
}

func decodeBar(t *testing.T, line string) barOut {
	t.Helper()
	var v barOut
	if err := json.Unmarshal([]byte(line), &v); err != nil {
		t.Fatalf("not one JSON object: %v %q", err, line)
	}
	return v
}

// TestBarOnce: the speeds as the text, the state, mode, routes and today in
// the tooltip, and the takeover and a state that isn't ok as classes.
func TestBarOnce(t *testing.T) {
	socket, _ := fake(t, map[string]func(map[string]any) (any, error){
		"get_status": func(map[string]any) (any, error) { return statusJSON, nil },
	})
	code, out, errOut := run(t, Env{Socket: socket}, "bar", "--once")
	if code != 0 || strings.Count(out, "\n") != 1 {
		t.Fatalf("bar --once: code %d %q %q", code, out, errOut)
	}
	v := decodeBar(t, strings.TrimSpace(out))
	if v.Text != "↓ 2.00 KB/s ↑ 10 B/s" {
		t.Errorf("text %q", v.Text)
	}
	if v.Alt != "on" || strings.Join(v.Class, ",") != "on,degraded" {
		t.Errorf("alt %q class %v", v.Alt, v.Class)
	}
	for _, want := range []string{"Degraded: AI has no node", "Mode Rule", "System proxy on", "TUN off", "其他流量 → 🇭🇰 香港 01 (Auto)", "Direct → Direct", "Today ↓ 5.00 GB"} {
		if !strings.Contains(v.Tooltip, want) {
			t.Errorf("tooltip lacks %q:\n%s", want, v.Tooltip)
		}
	}

	_, out, _ = run(t, Env{Socket: socket}, "bar", "--once", "--format", "text")
	if out != "↓ 2.00 KB/s ↑ 10 B/s\n" {
		t.Errorf("--format text: %q", out)
	}
}

// TestBarClasses: off, paused, and markup in names escaped.
func TestBarClasses(t *testing.T) {
	paused := status{}
	paused.Takeover.System = entry{Wanted: true, Status: "paused"}
	if c := barClass(paused, true); c != "paused" {
		t.Errorf("paused: %q", c)
	}
	if c := barClass(status{}, true); c != "off" {
		t.Errorf("off: %q", c)
	}
	st := status{State: "ok", Routes: []route{{Name: "A&B <x>", NodeLabel: "n"}}}
	v := decodeBar(t, barLine(st, true, "waybar"))
	if !strings.Contains(v.Tooltip, "A&amp;B &lt;x&gt; → n") || strings.Join(v.Class, ",") != "off" {
		t.Errorf("tooltip %q class %v", v.Tooltip, v.Class)
	}
}

// TestBarWaits: with bowel not running the bar says so and goes on, and
// takes up the speeds once bowel is there, without being started again.
func TestBarWaits(t *testing.T) {
	dir, err := os.MkdirTemp("", "bb")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	w := &lineWriter{lines: make(chan string, 100)}
	env := Env{Socket: filepath.Join(dir, "none.sock"), Context: ctx, Poll: 20 * time.Millisecond, Stdout: w}
	done := make(chan int)
	go func() { env.Args = []string{"bar"}; done <- Run(env) }()

	if v := decodeBar(t, <-w.lines); v.Alt != "stopped" || v.Text != "bowel" {
		t.Fatalf("not running: %+v", v)
	}
	// bowel comes up: the bar is at a socket of its own name, so point it there.
	socket, _ := fake(t, map[string]func(map[string]any) (any, error){
		"get_status": func(map[string]any) (any, error) { return statusJSON, nil },
	})
	if err := os.Rename(socket, env.Socket); err != nil {
		t.Skip("can't move the socket here:", err)
	}
	for {
		select {
		case l := <-w.lines:
			if decodeBar(t, l).Alt == "on" {
				cancel()
				if code := <-done; code != 0 {
					t.Errorf("code %d", code)
				}
				return
			}
		case <-ctx.Done():
			t.Fatal("the bar never saw bowel come up")
		}
	}
}

// TestBarStopsWithTheBar: a write that fails ends the command.
func TestBarStopsWithTheBar(t *testing.T) {
	socket, _ := fake(t, map[string]func(map[string]any) (any, error){
		"get_status": func(map[string]any) (any, error) { return statusJSON, nil },
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	env := Env{Socket: socket, Context: ctx, Poll: 20 * time.Millisecond, Stdout: failWriter{}, Args: []string{"bar"}}
	start := time.Now()
	if code := Run(env); code != 0 || time.Since(start) > time.Second {
		t.Errorf("code %d after %v", code, time.Since(start))
	}
}

type lineWriter struct{ lines chan string }

func (w *lineWriter) Write(p []byte) (int, error) {
	for _, l := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		w.lines <- l
	}
	return len(p), nil
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }
