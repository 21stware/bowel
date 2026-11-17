package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/netsetup"
	"github.com/oboo/moho/internal/sysproxy"
)

// adminRefused is networksetup's refusal as sysproxy and sysdns wrap it.
var adminRefused = fmt.Errorf("networksetup -setwebproxy: %w", netsetup.ErrAdmin)

// TestFailureTellsWhereTheOptionIs: the refusal of a Mac that asks for an
// administrator password names the option and where it is; anything else
// is its first line, as before (issue 189).
func TestFailureTellsWhereTheOptionIs(t *testing.T) {
	i18n.Set("en")
	defer i18n.Set("zh")
	got := failure(errors.Join(errors.New("networksetup -setwebproxystate: The parameters were not valid."), adminRefused))
	if !strings.Contains(got, "Privacy & Security › Advanced") || !strings.Contains(got, "administrator password") {
		t.Errorf("admin refusal: %q", got)
	}
	if got := failure(errors.New("first\nsecond")); got != "first" {
		t.Errorf("other failure: %q", got)
	}
}

// TestProxyRefusedForAdmin: the system proxy's entry carries the
// explanation, not networksetup's words.
func TestProxyRefusedForAdmin(t *testing.T) {
	i18n.Set("en")
	defer i18n.Set("zh")
	a := retakeApp(t)
	saved := coreUp
	coreUp = func(*App) bool { return true }
	t.Cleanup(func() { coreUp = saved })
	osProxy.snapshot = func() (sysproxy.Saved, error) { return sysproxy.Saved{}, nil }
	osProxy.enable = func(string, int, []string) error { return adminRefused }
	a.applySystemProxy()
	if a.proxyErr != failure(adminRefused) || strings.Contains(a.proxyErr, "networksetup") {
		t.Errorf("proxyErr %q", a.proxyErr)
	}
}

// TestDNSRefusedForAdmin: so does the DNS issue's reason.
func TestDNSRefusedForAdmin(t *testing.T) {
	i18n.Set("en")
	defer i18n.Set("zh")
	f := twoServices()
	f.refuse = fmt.Errorf("networksetup -setdnsservers: %w", netsetup.ErrAdmin)
	f.install(t)
	a := &App{dataDir: t.TempDir()}
	a.syncDNS(context.Background(), true, true)
	why := a.dnsErr.Load()
	if why == nil || *why != failure(f.refuse) {
		t.Errorf("dnsErr %v", why)
	}
}
