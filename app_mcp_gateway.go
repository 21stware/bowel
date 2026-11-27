package main

// The gateway's tools, for agents on the socket or over HTTP as for the
// command line (`bowel gateway …` calls them on cli.sock): its state and
// what comes next, the check for DHCP servers, the switch, its second
// step, and the devices that use it.
//
// docs/gateway.md's runbook is built into them, for the agent that never
// read it. Turning automatic on is refused while another DHCP server
// answers or while this machine's address is leased (setGatewayAuto, for
// everyone). And an agent is refused what would take devices offline: the
// gateway off while bowel hands out the LAN's addresses or while devices
// go out through it, and manual while no other DHCP server would take the
// devices over. force goes ahead, after the user has heard why. The user's
// own command is never refused: they decide.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"runtime"
	"strings"

	"github.com/oboo/moho/internal/gateway"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/mcp"
)

// gatewayInstructions add the gateway to what the server tells an agent.
const gatewayInstructions = `This machine can be the LAN's gateway (a side router, macOS and Linux): get_gateway says where it stands and, in next, what comes next and whose step it is. Steps marked user are the user's, done where you can't reach: authorizing once in a dialog on this machine, pointing a device at the gateway, giving this machine a fixed address, turning off a router's DHCP. Say them and stop; never change a router yourself. set_gateway and set_gateway_auto follow the user's policy, and refuse with the reason what would take devices offline: tell the user the reason, and pass force only after they agreed. Don't turn on automatic (set_gateway_auto) unless the user asked for the whole LAN.

When the user has several bowels (home, office, a server), each is its own MCP server, named for its machine (bowel-home, bowel-vps): a call changes only the machine of the server it is made on.`

// gatewayTools are the gateway's tools.
func (a *App) gatewayTools() []mcp.Tool {
	force := map[string]any{"type": "boolean", "description": "Go ahead although devices would go offline; only after the user agreed to that."}
	return []mcp.Tool{
		{
			Name: "get_gateway", Title: "Gateway", ReadOnly: true,
			Description: "The gateway (this machine as the LAN's way out) as it is: supported (macOS and Linux), step (off, manual, automatic), gateway (status on, off, checking, paused, install or error with detail; ip, the address devices use as router and DNS; router, the LAN's real router; dhcp, wifi and firewall warnings; auto, the DHCP step), how many devices use it, and next: what comes next, each with who does it, user or agent.",
			Run: func(ctx context.Context, s *mcp.Session, _ json.RawMessage) (any, error) {
				snap := a.snapshot()
				g := snap.Takeover.Gateway
				online, auto := 0, 0
				for _, d := range snap.Devices {
					if d.Online {
						online++
						if d.Via == "auto" {
							auto++
						}
					}
				}
				return map[string]any{
					"supported": gateway.Supported(), "step": gatewayStepName(g), "gateway": g,
					"devices": map[string]int{"known": len(snap.Devices), "online": online, "onlineAuto": auto},
					"next":    gatewayNext(gateway.Supported(), g, snap.Devices),
				}, nil
			},
		},
		{
			Name: "check_dhcp", Title: "Check the LAN for DHCP servers", ReadOnly: true,
			Description: "Asks the LAN who hands out addresses, as a device that just joined would (it takes none), and returns the servers that answered. Takes a few seconds. An empty list means bowel may become the DHCP server (set_gateway_auto). A server listed is usually the router: turning its DHCP off is the user's to do, in the router's admin page.",
			Input:       obj(map[string]any{}),
			Run: func(ctx context.Context, s *mcp.Session, raw json.RawMessage) (any, error) {
				if _, err := a.checkDHCP(); err != nil {
					return nil, err
				}
				g := a.snapshot().Takeover.Gateway
				return map[string]any{"servers": g.Auto.Servers, "gateway": g}, nil
			},
		},
		{
			Name: "set_gateway", Title: "Set gateway mode", Destructive: true,
			Description: "Turns gateway mode on or off: this machine as the LAN's gateway, on macOS and Linux. On turns TUN and the takeover on too, and changes no other device: a device uses the gateway once it is pointed at it (router and DNS = get_gateway's gateway.ip, set on the device: the user's to do) or through set_gateway_auto. The first time, a desktop asks the user for an administrator's authorization in a dialog on this machine; a daemon needs root. Then poll get_gateway until gateway.status is on. Off puts forwarding and port 53 back as they were, and every device that goes out through this machine loses its way out until it is pointed back at the router. So an agent's off is refused while bowel hands out the LAN's addresses (go back to manual first) or while devices are online through it; force goes ahead, once the user agreed.",
			Input:       obj(map[string]any{"enabled": map[string]any{"type": "boolean"}, "force": force}, "enabled"),
			Run: func(ctx context.Context, s *mcp.Session, raw json.RawMessage) (any, error) {
				var p struct{ Enabled, Force bool }
				if err := mcp.Args(raw, &p); err != nil {
					return nil, err
				}
				old := a.settings.Get().Gateway
				if old != p.Enabled {
					if !p.Enabled && !p.Force && !isLocal(s) {
						snap := a.snapshot()
						if err := gatewayOffRisk(snap.Takeover.Gateway, snap.Devices); err != nil {
							return nil, err
						}
					}
					err := a.change(ctx, s, words{"更改网关模式", "Change gateway mode"}, fmt.Sprintf("gateway: %v → %v", old, p.Enabled),
						func() error { return a.setGateway(p.Enabled) })
					if err != nil {
						return nil, err
					}
				}
				return map[string]any{"changed": old != p.Enabled, "takeover": a.snapshot().Takeover}, nil
			},
		},
		{
			Name: "set_gateway_auto", Title: "Hand out the LAN's addresses", Destructive: true,
			Description: "The gateway's second step, only when the user asked for every device on the LAN. Enabled: bowel becomes the LAN's DHCP server, so every device uses this machine as its router and DNS by itself. It refuses while another DHCP server answers on the LAN (check_dhcp says who; turning the router's off is the user's to do, in its admin page), and while this machine's own address comes from DHCP (gateway.dhcp true): a fixed address set on the machine itself is the user's to do. Disabled: back to manual, where devices are pointed at the gateway one by one; devices that got their address here lose their way out until another DHCP server takes them over. So an agent's disable first checks the LAN (a few seconds) and is refused while no other DHCP server answers: the user turns the router's DHCP back on first. force goes ahead, once the user agreed.",
			Input:       obj(map[string]any{"enabled": map[string]any{"type": "boolean"}, "force": force}, "enabled"),
			Run: func(ctx context.Context, s *mcp.Session, raw json.RawMessage) (any, error) {
				var p struct{ Enabled, Force bool }
				if err := mcp.Args(raw, &p); err != nil {
					return nil, err
				}
				a.auto.mu.Lock()
				old := a.settings.Get().GatewayAuto
				// Manual also cancels the current enable before it saves the
				// setting, even if a superseded request cleared the UI busy flag.
				if !p.Enabled && a.auto.enabling {
					old = true
				}
				a.auto.mu.Unlock()
				if old != p.Enabled {
					if !p.Enabled && !p.Force && !isLocal(s) {
						others, err := a.checkDHCP()
						if err := gatewayManualRisk(a.snapshot().Takeover.Gateway, others, err); err != nil {
							return nil, err
						}
					}
					err := a.change(ctx, s, words{"更改网关的地址分配", "Change the gateway's handing out of addresses"}, fmt.Sprintf("gatewayAuto: %v → %v", old, p.Enabled),
						func() error { return a.setGatewayAuto(p.Enabled) })
					if err != nil {
						return nil, err
					}
				}
				return map[string]any{"changed": old != p.Enabled, "gateway": a.snapshot().Takeover.Gateway}, nil
			},
		},
		{
			Name: "list_devices", Title: "List the devices that use the gateway", ReadOnly: true,
			Description: "The other devices on the LAN that go out through this machine: address, name, whether each got its address from bowel (auto) or was pointed here by hand (manual), whether it is online, its open connections and when it came and went.",
			Input:       obj(map[string]any{}),
			Run: func(ctx context.Context, s *mcp.Session, raw json.RawMessage) (any, error) {
				snap := a.snapshot()
				devs := snap.Devices
				if devs == nil {
					devs = []DeviceView{}
				}
				return map[string]any{"devices": devs, "gateway": snap.Takeover.Gateway}, nil
			},
		},
	}
}

// gatewayStepName is the gateway's step: off, manual or automatic.
func gatewayStepName(g GatewayView) string {
	switch {
	case !g.Wanted:
		return "off"
	case g.Auto.Wanted:
		return "automatic"
	}
	return "manual"
}

// onlineDevices are the devices online through the gateway, by name or
// address.
func onlineDevices(devs []DeviceView) []string {
	var out []string
	for _, d := range devs {
		if d.Online {
			n := d.IP
			if d.Name != "" {
				n = d.Name + " (" + d.IP + ")"
			}
			out = append(out, n)
		}
	}
	return out
}

// gatewayOffRisk is why turning the gateway off now takes devices offline,
// nil when it takes none.
func gatewayOffRisk(g GatewayView, devs []DeviceView) error {
	if !g.Wanted {
		return nil
	}
	if g.Auto.Wanted {
		return fmt.Errorf(i18n.T(
			"bowel 正在给局域网分配地址（自动，%d 个租约）：现在关闭网关，从这里领到地址的设备都会没有路由器、断网。先由用户在路由器的管理页面重新打开它的 DHCP（这一步是用户的），再用 set_gateway_auto 退回手动（它会先检测），等 list_devices 里不再有在线的 auto 设备，再关闭网关。用户听过这些仍要现在关闭，才传 force: true",
			"bowel hands out the LAN's addresses (automatic, %d leases): turned off now, every device that got its address here is left without a router and goes offline. First the user turns the router's DHCP back on in its admin page (their step, not yours), then set_gateway_auto with enabled false goes back to manual (it checks first), then wait until list_devices shows no device online with via auto, and turn the gateway off then. Pass force: true only if the user, having heard this, wants it off now"),
			g.Auto.Leases)
	}
	if on := onlineDevices(devs); len(on) > 0 {
		return fmt.Errorf(i18n.T(
			"有 %d 台设备正经过这台电脑上网（%s）：关闭网关后，它们要等各自改回原来的路由器（%s）才能上网，这要用户在每台设备上改。先告诉用户是哪些设备；用户同意后再传 force: true",
			"%d devices go out through this machine now (%s): with the gateway off they have no way out until each is pointed back at the router (%s), which the user does on each device. Tell the user which; pass force: true once they agreed"),
			len(on), strings.Join(on, ", "), g.Router)
	}
	return nil
}

// gatewayManualRisk is why going back to manual now takes devices offline,
// nil when it takes none: another DHCP server answers (the router's, back
// on), so the devices that got their address here move over to it.
// others and checkErr are a check of the LAN made just now.
func gatewayManualRisk(g GatewayView, others []netip.Addr, checkErr error) error {
	if !g.Auto.Wanted {
		return nil
	}
	if checkErr != nil {
		return fmt.Errorf(i18n.T(
			"没法确认路由器的 DHCP 是否已重新打开：%s。退回手动后，从这里领到地址的设备可能断网；用户确认路由器的 DHCP 已打开后，再传 force: true",
			"Couldn't tell whether the router's DHCP is back on: %s. Back at manual, devices that got their address here may go offline; pass force: true once the user says the router's DHCP is on"), checkErr)
	}
	if len(others) == 0 {
		return fmt.Errorf(i18n.T(
			"局域网里没有别的 DHCP 服务器在应答：现在退回手动，从这里领到地址的设备（%d 个租约）续不了租，会断网。先由用户在路由器的管理页面重新打开它的 DHCP（这一步是用户的），然后再调用一次：它会先检测。只有用户听过这些仍要退回（比如已经没有路由器可回），才传 force: true",
			"No other DHCP server answers on the LAN: back at manual now, the devices that got their address here (%d leases) can't renew it and go offline. First the user turns the router's DHCP back on in its admin page (their step, not yours), then call again: it checks first. Pass force: true only if the user, having heard this, wants manual anyway (there is no router to go back to, say)"),
			g.Auto.Leases)
	}
	return nil
}

// gatewayTodo is one thing to do next, and whose it is: "user" (done
// where an agent can't reach) or "agent".
type gatewayTodo struct {
	Who string `json:"who"`
	Do  string `json:"do"`
}

// gatewayNext is what comes next for the gateway as it is.
func gatewayNext(supported bool, g GatewayView, devs []DeviceView) []gatewayTodo {
	user := func(s string) gatewayTodo { return gatewayTodo{"user", s} }
	agent := func(s string) gatewayTodo { return gatewayTodo{"agent", s} }
	if !supported {
		return []gatewayTodo{user("Gateway mode runs on macOS and Linux; this machine is " + runtime.GOOS + ". Another machine on the LAN can be the gateway.")}
	}
	if !g.Wanted {
		return []gatewayTodo{agent("Nothing, unless the user wants this machine as the LAN's gateway: then set_gateway with enabled true.")}
	}
	var out []gatewayTodo
	switch g.Status {
	case "paused":
		return append(out, agent("The takeover or TUN is off, so the gateway is too: set_gateway with enabled true turns both on again."))
	case "install":
		return append(out, user("Authorize once: the administrator dialog on this machine (the desktop app), or run bowel daemon as root."))
	case "checking":
		return append(out, agent("Not measured yet: call get_gateway again in a few seconds."))
	case "error":
		return append(out, agent("Something doesn't work: "+g.Detail+". See When it does not work in docs/gateway.md; most fixes are on this machine (the port 53 resolver, the firewall), some the user's."))
	}
	if g.DHCP {
		out = append(out, user("Give this machine a fixed address in its own network settings (same address, the router as its router): it is leased now, and when it changes every device pointed at it loses its way out. Automatic needs it."))
	}
	if g.Firewall {
		out = append(out, user("The firewall may drop what is forwarded or port 53: allow them from the LAN on this machine."))
	}
	if g.WiFi {
		out = append(out, user("This machine is on Wi-Fi: a cable is faster, since every packet crosses the air twice."))
	}
	if !g.Auto.Wanted {
		out = append(out, user(fmt.Sprintf("On each device that should use the gateway: IPv4 manual, router %s, DNS %s and no second DNS server.", g.IP, g.IP)))
		if len(onlineDevices(devs)) == 0 {
			out = append(out, agent("list_devices shows a device once it uses the gateway."))
		}
		return out
	}
	switch g.Auto.Status {
	case "starting":
		out = append(out, agent("bowel is making sure no other DHCP server answers before it starts: call get_gateway again in a few seconds."))
	case "error":
		if len(g.Auto.Servers) > 0 {
			out = append(out, user("Turn off the DHCP service of "+strings.Join(g.Auto.Servers, ", ")+" in its admin page: two DHCP servers fight over the devices. Or go back to manual."))
		} else if g.Auto.Detail != "" {
			out = append(out, agent("Automatic doesn't serve: "+g.Auto.Detail))
		}
	case "on":
		out = append(out, agent("Every device on the LAN depends on this machine now: don't turn the gateway off, go back to manual or stop bowel to test anything. To undo, the user turns the router's DHCP on first, then set_gateway_auto with enabled false."))
	}
	return out
}
