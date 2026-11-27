package main

import (
	"errors"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"github.com/oboo/moho/internal/gateway"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/mcp"
	"github.com/oboo/moho/internal/settings"
)

func TestGatewayOffRisk(t *testing.T) {
	i18n.Set("en")
	t.Cleanup(func() { i18n.Set("zh") })
	tv := DeviceView{IP: "192.168.1.100", Name: "tv", Via: "auto", Online: true}
	phone := DeviceView{IP: "192.168.1.40", Via: "manual", Online: true}
	gone := DeviceView{IP: "192.168.1.41", Via: "manual"}
	manual := GatewayView{Wanted: true, Status: "on", Network: gateway.Network{IP: "192.168.1.20", Router: "192.168.1.1"}}
	auto := manual
	auto.Auto = AutoView{Wanted: true, Status: "on", Leases: 3}

	if err := gatewayOffRisk(GatewayView{}, []DeviceView{phone}); err != nil {
		t.Errorf("off already: %v", err)
	}
	if err := gatewayOffRisk(manual, []DeviceView{gone}); err != nil {
		t.Errorf("no device online: %v", err)
	}
	if err := gatewayOffRisk(manual, []DeviceView{phone, tv, gone}); err == nil || !strings.Contains(err.Error(), "2 devices") ||
		!strings.Contains(err.Error(), "tv (192.168.1.100)") || !strings.Contains(err.Error(), "192.168.1.1") || strings.Contains(err.Error(), "192.168.1.41") {
		t.Errorf("devices online: %v", err)
	}
	// Automatic: refused whoever is online, since every lease's device
	// would lose its router.
	if err := gatewayOffRisk(auto, nil); err == nil || !strings.Contains(err.Error(), "3 leases") || !strings.Contains(err.Error(), "set_gateway_auto") {
		t.Errorf("automatic: %v", err)
	}
}

func TestGatewayManualRisk(t *testing.T) {
	i18n.Set("en")
	t.Cleanup(func() { i18n.Set("zh") })
	auto := GatewayView{Wanted: true, Status: "on", Auto: AutoView{Wanted: true, Status: "on", Leases: 2}}
	router := []netip.Addr{netip.MustParseAddr("192.168.1.1")}

	if err := gatewayManualRisk(GatewayView{Wanted: true}, nil, nil); err != nil {
		t.Errorf("manual already: %v", err)
	}
	if err := gatewayManualRisk(auto, nil, nil); err == nil || !strings.Contains(err.Error(), "2 leases") {
		t.Errorf("nobody to take the devices over: %v", err)
	}
	if err := gatewayManualRisk(auto, nil, errors.New("no socket")); err == nil || !strings.Contains(err.Error(), "no socket") {
		t.Errorf("a failed check: %v", err)
	}
	if err := gatewayManualRisk(auto, router, nil); err != nil {
		t.Errorf("the router's DHCP is back: %v", err)
	}
}

func TestGatewayNext(t *testing.T) {
	who := func(todo []gatewayTodo) (out []string) {
		for _, x := range todo {
			out = append(out, x.Who)
		}
		return out
	}
	has := func(todo []gatewayTodo, who, part string) bool {
		return slices.ContainsFunc(todo, func(x gatewayTodo) bool { return x.Who == who && strings.Contains(x.Do, part) })
	}
	if n := gatewayNext(false, GatewayView{}, nil); len(n) != 1 || n[0].Who != "user" {
		t.Errorf("unsupported: %+v", n)
	}
	if n := gatewayNext(true, GatewayView{}, nil); len(n) != 1 || !has(n, "agent", "set_gateway") {
		t.Errorf("off: %+v", n)
	}
	if n := gatewayNext(true, GatewayView{Wanted: true, Status: "install"}, nil); !has(n, "user", "Authorize") {
		t.Errorf("install: %+v", n)
	}
	on := GatewayView{Wanted: true, Status: "on", Network: gateway.Network{IP: "192.168.1.20", Router: "192.168.1.1", DHCP: true}}
	if n := gatewayNext(true, on, nil); !has(n, "user", "fixed address") || !has(n, "user", "router 192.168.1.20, DNS 192.168.1.20") || !has(n, "agent", "list_devices") {
		t.Errorf("manual, leased: %v %+v", who(n), n)
	}
	on.DHCP = false
	on.Auto = AutoView{Wanted: true, Status: "error", Servers: []string{"192.168.1.1"}}
	if n := gatewayNext(true, on, nil); !has(n, "user", "DHCP service of 192.168.1.1") {
		t.Errorf("automatic, another server: %+v", n)
	}
	on.Auto = AutoView{Wanted: true, Status: "on"}
	if n := gatewayNext(true, on, nil); !has(n, "agent", "don't turn the gateway off") {
		t.Errorf("automatic on: %+v", n)
	}
}

// gatewayMCPApp is mcpApp with a gateway on the fake LAN, wanted, in step
// auto when auto is true.
func gatewayMCPApp(t *testing.T, auto bool) (*App, *gwFake) {
	t.Helper()
	f := &gwFake{t: t, ready: true, net: gateway.Network{IP: "192.168.1.20", Iface: "en0", Router: "192.168.1.1"}}
	saved := osGateway
	osGateway = func() gateway.System { return f }
	t.Cleanup(func() { osGateway = saved })
	a, _, _ := mcpApp(t)
	i18n.Set("en")
	if _, err := a.settings.Update(func(s *settings.Settings) {
		s.Gateway, s.GatewayAuto, s.Tun = true, auto, true
	}); err != nil {
		t.Fatal(err)
	}
	return a, f
}

// The gateway's tools are the agents' too, once: the command line's server
// has them from the agents', not a second time.
func TestGatewayToolsServed(t *testing.T) {
	a, _, _ := mcpApp(t)
	for _, srv := range []*mcp.Server{a.mcpServer(), a.cliServer()} {
		seen := map[string]bool{}
		for _, tool := range srv.Tools {
			if seen[tool.Name] {
				t.Errorf("%s: %s twice", srv.Name, tool.Name)
			}
			seen[tool.Name] = true
		}
		for _, n := range []string{"get_gateway", "check_dhcp", "set_gateway", "set_gateway_auto", "list_devices", "provider_add", "provider_remove"} {
			if !seen[n] {
				t.Errorf("%s: no %s", srv.Name, n)
			}
		}
	}
	for _, tool := range a.mcpServer().Tools {
		if (tool.Name == "set_gateway" || tool.Name == "set_gateway_auto" || tool.Name == "provider_remove") && !tool.Destructive {
			t.Errorf("%s is not marked destructive", tool.Name)
		}
	}
	if !strings.Contains(a.mcpServer().Instructions, "get_gateway") {
		t.Error("the instructions don't mention the gateway")
	}
}

// An agent's off is refused while bowel hands out the LAN's addresses,
// with the reason; force, or the user's own command, goes ahead.
func TestSetGatewayOffGuard(t *testing.T) {
	a, _ := gatewayMCPApp(t, true)
	setPolicy(t, a, "allow")
	srv := a.mcpServer()
	agent := &mcp.Session{}
	if _, err := callTool(t, srv, agent, "set_gateway", map[string]any{"enabled": false}); err == nil || !strings.Contains(err.Error(), "leases") {
		t.Fatalf("an agent turned automatic's gateway off: %v", err)
	}
	if s := a.settings.Get(); !s.Gateway || !s.GatewayAuto {
		t.Fatalf("refused, and changed: %+v", s)
	}
	if out, err := callTool(t, srv, agent, "get_gateway", nil); err != nil || out["step"] != "automatic" {
		t.Fatalf("get_gateway: %v %v", out, err)
	}
	// A container's docker exec with no terminal is refused as an agent's
	// is, and told why, since the command line has no force to pass.
	unattended := &mcp.Session{}
	unattended.Set("unattended", true)
	if _, err := callTool(t, a.cliServer(), unattended, "set_gateway", map[string]any{"enabled": false}); err == nil || !strings.Contains(err.Error(), "leases") || !strings.Contains(err.Error(), "docker exec -it") || !a.settings.Get().Gateway {
		t.Fatalf("an unattended off: %v", err)
	}
	_, _ = callTool(t, srv, agent, "set_gateway", map[string]any{"enabled": false, "force": true})
	if a.settings.Get().Gateway {
		t.Fatal("force didn't turn it off")
	}

	// The user at a shell is not asked to force anything.
	if _, err := a.settings.Update(func(s *settings.Settings) { s.Gateway, s.GatewayAuto = true, true }); err != nil {
		t.Fatal(err)
	}
	user := &mcp.Session{}
	user.Set("local", true)
	_, _ = callTool(t, a.cliServer(), user, "set_gateway", map[string]any{"enabled": false})
	if a.settings.Get().Gateway {
		t.Fatal("the user's own off was refused")
	}
}

// An agent's manual checks the LAN first: refused while no other DHCP
// server would take the devices over, let through once the router's is on.
func TestSetGatewayManualGuard(t *testing.T) {
	a, _ := gatewayMCPApp(t, true)
	setPolicy(t, a, "allow")
	l := fakeLAN(t)
	t.Cleanup(a.stopDHCP)
	srv := a.mcpServer()
	agent := &mcp.Session{}

	if _, err := callTool(t, srv, agent, "set_gateway_auto", map[string]any{"enabled": false}); err == nil || !strings.Contains(err.Error(), "No other DHCP server") {
		t.Fatalf("manual with nobody to take the devices: %v", err)
	}
	if l.asked() != 1 || !a.settings.Get().GatewayAuto {
		t.Fatalf("checks %d, auto %v", l.asked(), a.settings.Get().GatewayAuto)
	}
	l.set(func(l *lanFake) { l.others = []netip.Addr{netip.MustParseAddr("192.168.1.1")} })
	out, err := callTool(t, srv, agent, "set_gateway_auto", map[string]any{"enabled": false})
	if err != nil || out["changed"] != true || a.settings.Get().GatewayAuto {
		t.Fatalf("manual with the router's DHCP back: %v %v", out, err)
	}
}
