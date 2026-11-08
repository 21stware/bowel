package main

import (
	"context"
	"reflect"
	"testing"

	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/settings"
	"github.com/oboo/moho/internal/sysproxy"
)

// TestProxyBackup: enabling keeps what the system had, without Moho's own
// leftovers; disabling puts that back and leaves no file behind. All of it
// through fakes: the machine's proxy is never touched.
func TestProxyBackup(t *testing.T) {
	port, err := core.FreePort() // never the running app's port
	if err != nil {
		t.Fatal(err)
	}
	before := sysproxy.Saved{Services: []sysproxy.Service{
		{Name: "Wi-Fi", Web: sysproxy.Endpoint{On: true, Host: "proxy.corp", Port: 8080}, Bypass: []string{"corp"}},
		{Name: "Ethernet", Web: sysproxy.Endpoint{Host: "127.0.0.1", Port: port}, Bypass: []string{"*.local"}},
	}}
	var enabled, disabled, snapshots int
	var restored []sysproxy.Saved
	saved := osProxy
	defer func() { osProxy = saved }()
	osProxy.enable = func(host string, p int, _ []string) error {
		if host != "127.0.0.1" || p != port {
			t.Errorf("enable %s:%d", host, p)
		}
		enabled++
		return nil
	}
	osProxy.disable = func() error { disabled++; return nil }
	osProxy.snapshot = func() (sysproxy.Saved, error) { snapshots++; return before, nil }
	osProxy.restore = func(_ context.Context, s sysproxy.Saved) error { restored = append(restored, s); return nil }

	a := &App{dataDir: t.TempDir()}
	s := settings.Settings{MixedPort: port}
	if err := a.enableProxy(s); err != nil {
		t.Fatal(err)
	}
	if enabled != 1 || snapshots != 1 || !exists(a.proxyBackup()) {
		t.Fatalf("enable: enabled %d, snapshots %d, backup %v", enabled, snapshots, exists(a.proxyBackup()))
	}
	if err := a.disableProxy(context.Background(), port); err != nil {
		t.Fatal(err)
	}
	want := sysproxy.Saved{Services: []sysproxy.Service{
		before.Services[0],
		{Name: "Ethernet", Bypass: sysproxy.DefaultBypass}, // Moho's leftover is cleared, with its bypass list
	}}
	if len(restored) != 1 || !reflect.DeepEqual(restored[0], want) {
		t.Fatalf("restored %+v\nwant %+v", restored, want)
	}
	if disabled != 0 || exists(a.proxyBackup()) {
		t.Fatalf("after restore: disabled %d, backup left %v", disabled, exists(a.proxyBackup()))
	}
	// With no copy kept, disabling just turns the proxies off.
	if err := a.disableProxy(context.Background(), port); err != nil {
		t.Fatal(err)
	}
	if disabled != 1 || len(restored) != 1 {
		t.Fatalf("no backup: disabled %d, restored %d", disabled, len(restored))
	}
}
