package settings

import (
	"errors"
	"fmt"
	"net"
	"net/netip"

	"github.com/oboo/moho/internal/i18n"
)

// GatewayDevice is a device of the gateway's LAN with a route of its own:
// all of its traffic goes to one route group, or it is left out of the
// gateway. The core knows a device by its address alone. Its hardware
// address, once known, is what keeps the route with the device when the
// address changes, and what bowel's DHCP (automatic) keeps the address for.
type GatewayDevice struct {
	// IP is its IPv4 address. "" while the address went to another device
	// and this one has not been seen since: the route waits for it.
	IP string `json:"ip,omitempty"`
	// MAC is its hardware address, in lower case with colons; "" until known.
	MAC string `json:"mac,omitempty"`
	// Name is what it was called when its route was set, for people.
	Name string `json:"name,omitempty"`
	// Route is the id of the route group all its traffic goes to; "" with
	// Bypass.
	Route string `json:"route,omitempty"`
	// Bypass leaves it out of the gateway: bowel's DHCP gives it the LAN's
	// router as its router and DNS, and while it still comes through here
	// (pointed at the gateway by hand) its traffic goes direct.
	Bypass bool `json:"bypass,omitempty"`
}

// MaxGatewayDevices is how many devices can have a route of their own.
const MaxGatewayDevices = 250

// Addr is the device's address, invalid while it has none.
func (d GatewayDevice) Addr() netip.Addr {
	a, err := netip.ParseAddr(d.IP)
	if err != nil || !a.Is4() {
		return netip.Addr{}
	}
	return a
}

// CanonMAC is a hardware address as GatewayDevice keeps it, "" for none
// that is one: six bytes, lower case, with colons.
func CanonMAC(s string) string {
	hw, err := net.ParseMAC(s)
	if err != nil || len(hw) != 6 {
		return ""
	}
	return hw.String()
}

func validDevices(list []GatewayDevice) error {
	if len(list) > MaxGatewayDevices {
		return fmt.Errorf(i18n.T("最多给 %d 台设备单独设路由", "At most %d devices can have a route of their own"), MaxGatewayDevices)
	}
	ips, macs := map[string]bool{}, map[string]bool{}
	for _, d := range list {
		who := d.IP
		if who == "" {
			who = d.MAC
		}
		switch a, err := netip.ParseAddr(d.IP); {
		case d.IP == "" && d.MAC == "":
			return errors.New(i18n.T("设备要有地址或硬件地址", "A device needs an address or a hardware address"))
		case d.IP == "":
		case err != nil || !a.Is4() || a.IsUnspecified() || a.IsLoopback() || a.IsMulticast() || a.String() != d.IP:
			return fmt.Errorf(i18n.T("设备的地址要是 IPv4 地址: %s", "A device's address must be an IPv4 address: %s"), d.IP)
		}
		if d.MAC != "" && CanonMAC(d.MAC) != d.MAC {
			return fmt.Errorf(i18n.T("硬件地址的写法不对: %s", "Malformed hardware address: %s"), d.MAC)
		}
		if (d.Route == "") == !d.Bypass {
			return fmt.Errorf(i18n.T("设备 %s 要么走一个路由组，要么不经过网关", "Device %s either goes to one route group or is left out of the gateway"), who)
		}
		if d.IP != "" && ips[d.IP] || d.MAC != "" && macs[d.MAC] {
			return fmt.Errorf(i18n.T("设备 %s 重复了", "Device %s is there twice"), who)
		}
		ips[d.IP], macs[d.MAC] = d.IP != "", d.MAC != ""
	}
	return nil
}
