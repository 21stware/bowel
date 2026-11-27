package cli

// bowel gateway device: a route of its own for one device of the gateway.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/oboo/moho/internal/i18n"
)

// gatewayDevice is a device as list_devices and set_device_route give it.
type gatewayDevice struct {
	IP        string `json:"ip"`
	MAC       string `json:"mac"`
	Name      string `json:"name"`
	Via       string `json:"via"`
	Online    bool   `json:"online"`
	Route     string `json:"route"`
	RouteName string `json:"routeName"`
	Bypass    bool   `json:"bypass"`
	Away      bool   `json:"away"`
}

// deviceRoute is a device's route in a word or two.
func deviceRoute(d gatewayDevice) cell {
	switch {
	case d.Bypass:
		return cell{i18n.T("不经过网关", "left out"), yellow}
	case d.Route != "":
		name := d.RouteName
		if name == "" {
			name = d.Route
		}
		if d.Away {
			return cell{name + " " + i18n.T("（等它回来）", "(waiting for it)"), gray}
		}
		return cell{name, green}
	}
	return cell{i18n.T("按规则", "rules"), gray}
}

func deviceTitle(d gatewayDevice) string {
	switch {
	case d.Name != "" && d.IP != "":
		return d.Name + " (" + d.IP + ")"
	case d.Name != "":
		return d.Name
	case d.IP != "":
		return d.IP
	}
	return d.MAC
}

// runGatewayDevice shows one device's route, or sets it:
// bowel gateway device <device> [<route>|bypass|rules].
func runGatewayDevice(c *Ctx) error {
	usage := "bowel gateway device " + i18n.T("<设备> [<路由组>|bypass|rules]", "<device> [<route group>|bypass|rules]")
	if len(c.args) < 2 {
		return usagef(c.path, i18n.T("缺少参数：%s", "Missing arguments: %s"), usage)
	}
	who := c.args[1]
	if len(c.args) == 2 {
		var out struct {
			Devices []json.RawMessage `json:"devices"`
		}
		if _, err := c.get("list_devices", nil, &out); err != nil {
			return err
		}
		for _, one := range out.Devices {
			var d gatewayDevice
			_ = json.Unmarshal(one, &d)
			if d.IP == who || strings.EqualFold(d.MAC, who) || strings.EqualFold(d.Name, who) {
				if c.json {
					c.printJSON(one, false)
					return nil
				}
				r := deviceRoute(d)
				fmt.Fprintf(c.out, "%s  %s\n", deviceTitle(d), c.paint(r.color, r.text))
				return nil
			}
		}
		return fmt.Errorf(i18n.T("没有这台设备: %s（用 bowel gateway devices 看看有哪些）", "No such device: %s (bowel gateway devices lists them)"), who)
	}
	var out struct {
		Device gatewayDevice `json:"device"`
		Closed int           `json:"closedConnections"`
		Note   string        `json:"note"`
	}
	raw, err := c.get("set_device_route", map[string]any{"device": who, "route": c.args[2]}, &out)
	if err != nil {
		return err
	}
	d := out.Device
	switch {
	case d.Bypass:
		c.done(raw, fmt.Sprintf(i18n.T("%s 不经过网关", "%s is left out of the gateway"), deviceTitle(d)))
	case d.Route != "":
		c.done(raw, fmt.Sprintf(i18n.T("%s 的全部流量走「%s」", "All of %s's traffic goes to %s"), deviceTitle(d), deviceRoute(d).text))
	default:
		c.done(raw, fmt.Sprintf(i18n.T("%s 按规则分流，和其他设备一样", "%s follows the rules, as every device does"), deviceTitle(d)))
	}
	if !c.json && out.Note != "" {
		c.warn(out.Note)
	}
	return nil
}
