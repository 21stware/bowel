package main

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/sysproxy"
)

// TestQuitStepsNeverHang: a step that never returns costs the limit and
// no more, one that panics costs nothing, and the others still finish.
func TestQuitStepsNeverHang(t *testing.T) {
	finished := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	left := runQuitSteps(ctx, []quitStep{
		{"stuck", func(context.Context) { select {} }},
		{"broken", func(context.Context) { panic("boom") }},
		{"fine", func(context.Context) { close(finished) }},
	})
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("took %v", took)
	}
	if !reflect.DeepEqual(left, []string{"stuck"}) {
		t.Errorf("unfinished: %v", left)
	}
	select {
	case <-finished:
	default:
		t.Error("the fine step did not run")
	}
}

func TestQuitStepsAllDone(t *testing.T) {
	if left := runQuitSteps(context.Background(), []quitStep{{"a", func(context.Context) {}}}); left != nil {
		t.Errorf("unfinished: %v", left)
	}
}

// TestDisableProxyOutOfTime: when time runs out the copy of the system
// proxy stays for the next launch, and no more is done to the system.
func TestDisableProxyOutOfTime(t *testing.T) {
	port, err := core.FreePort() // never the running app's port
	if err != nil {
		t.Fatal(err)
	}
	saved := osProxy
	defer func() { osProxy = saved }()
	osProxy.disable = func() error { t.Error("disabled after time ran out"); return nil }
	osProxy.restore = func(ctx context.Context, _ sysproxy.Saved) error {
		<-ctx.Done()
		return ctx.Err()
	}
	a := &App{dataDir: t.TempDir()}
	if err := os.WriteFile(a.proxyBackup(), []byte(`{"services":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := a.disableProxy(ctx, port); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v", err)
	}
	if !exists(a.proxyBackup()) {
		t.Error("the copy of the system proxy is gone")
	}
}

// TestNoProxyChangeWhileQuitting: a settings change or a core coming up
// late cannot point the system at a core that is about to go. a has no
// settings, so getting past the guard would panic.
func TestNoProxyChangeWhileQuitting(t *testing.T) {
	a := &App{quitting: true}
	a.applySystemProxy()
}
