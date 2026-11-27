package main

// The tool that gives a device of the gateway a route of its own
// (app_gateway_devices.go), for agents and the command line alike.

import (
	"context"
	"encoding/json"

	"github.com/oboo/moho/internal/mcp"
)

func (a *App) deviceRouteTool() mcp.Tool {
	return mcp.Tool{
		Name: "set_device_route", Title: "Give a gateway device a route of its own", Destructive: true,
		Description: "Gateway mode: sends all of one device's traffic one way, whatever the route groups' rules say. route is a route group's id or name (list_routes): direct sends the device direct, reject blocks everything it does; bypass leaves the device out of the gateway (with bowel handing out addresses it gets the LAN's router as its router and DNS at its next renewal, within 10 minutes; one pointed at the gateway by hand can only be sent direct); rules takes its own route away, so it follows the rules as every device does. " +
			"A device's route comes before every route group's rules but Block's and those of groups with the exit lock, which hold for every device; a blocked device has nothing before it. Like the groups' rules it holds in rule mode only, and not under a provider's own routing. " +
			"device is its address, hardware address or name as list_devices shows them; an address on the LAN that no device has used yet is taken too. The core knows a device by its address: bowel keeps the address for it when it hands out addresses, and moves the route with the device's hardware address when it turns up at another. The device's open connections are closed, so that what it opens next goes the new way.",
		Input: obj(map[string]any{
			"device": str("The device: its IPv4 address, hardware address or name, as list_devices shows them."),
			"route":  str("A route group's id or name, or bypass, or rules."),
		}, "device", "route"),
		Run: func(ctx context.Context, s *mcp.Session, raw json.RawMessage) (any, error) {
			var p struct{ Device, Route string }
			if err := mcp.Args(raw, &p); err != nil {
				return nil, err
			}
			ch, err := a.planDevice(p.Device, p.Route)
			if err != nil {
				return nil, err
			}
			var v DeviceView
			var closed int
			err = a.change(ctx, s, words{"更改设备的路由", "Change a device's route"}, ch.describe(a), func() error {
				var err error
				v, closed, err = a.setDeviceRoute(ch)
				return err
			})
			if err != nil {
				return nil, err
			}
			return map[string]any{"device": v, "closedConnections": closed, "note": v.Note}, nil
		},
	}
}
