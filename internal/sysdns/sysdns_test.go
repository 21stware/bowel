package sysdns

import (
	"reflect"
	"slices"
	"testing"
)

func sv(name string, servers ...string) Service { return Service{Name: name, Servers: servers} }

func TestParseGet(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
		bad  bool
	}{
		{"There aren't any DNS Servers set on Wi-Fi.\n", nil, false},
		{"192.168.1.1\n2400:3200::1\n", []string{"192.168.1.1", "2400:3200::1"}, false},
		{"", nil, false},
		{"Wi-Fi is not a recognized network service.\n** Error: The parameters were not valid.\n", nil, true},
		{"1.1.1.1\nThere aren't any DNS Servers set on Wi-Fi.\n", nil, true},
	} {
		got, err := parseGet(c.in)
		if (err != nil) != c.bad || !slices.Equal(got, c.want) {
			t.Errorf("parseGet(%q) = %v, %v", c.in, got, err)
		}
	}
}

func TestParseOrder(t *testing.T) {
	out := `An asterisk (*) denotes that a network service is disabled.
(1) Ethernet
(Hardware Port: Ethernet, Device: en0)

(2) Thunderbolt Bridge
(Hardware Port: Thunderbolt Bridge, Device: bridge0)

(*) Wi-Fi (2)
(Hardware Port: Wi-Fi, Device: en1)

(4) My VPN
(Hardware Port: com.example.vpn, Device: )
`
	want := map[string]string{"Ethernet": "en0", "Thunderbolt Bridge": "bridge0", "Wi-Fi (2)": "en1"}
	if got := parseOrder(out); !reflect.DeepEqual(got, want) {
		t.Errorf("parseOrder = %v", got)
	}
}

func TestParsePacket(t *testing.T) {
	out := "op = BOOTREPLY\nrouter (ip_mult): {192.168.50.1}\ndomain_name_server (ip_mult): {192.168.50.1, 192.168.50.2}\nend (none):\n"
	if got := parsePacket(out); !slices.Equal(got, []string{"192.168.50.1", "192.168.50.2"}) {
		t.Errorf("parsePacket = %v", got)
	}
	if got := parsePacket("op = BOOTREPLY\n"); got != nil {
		t.Errorf("parsePacket without servers = %v", got)
	}
}

func TestSetArgs(t *testing.T) {
	if got := setArgs(sv("Wi-Fi")); !slices.Equal(got, []string{"-setdnsservers", "Wi-Fi", "Empty"}) {
		t.Errorf("none: %v", got)
	}
	if got := setArgs(sv("USB 10/100 LAN", "1.1.1.1", "8.8.8.8")); !slices.Equal(got, []string{"-setdnsservers", "USB 10/100 LAN", "1.1.1.1", "8.8.8.8"}) {
		t.Errorf("some: %v", got)
	}
}

// The first copy of what a service had is the one that stays, however
// often the placeholder is written again.
func TestKeep(t *testing.T) {
	held := sv("Wi-Fi", Placeholder...)
	first := Keep(Saved{}, Saved{Services: []Service{sv("Wi-Fi", "10.0.0.2"), sv("Ethernet")}})
	if want := (Saved{Services: []Service{sv("Wi-Fi", "10.0.0.2"), sv("Ethernet")}}); !reflect.DeepEqual(first, want) {
		t.Fatalf("first copy = %+v", first)
	}
	// Taken over since, and a service has appeared.
	again := Keep(first, Saved{Services: []Service{held, sv("Ethernet", Placeholder...), sv("Dock", "9.9.9.9")}})
	if want := (Saved{Services: []Service{sv("Wi-Fi", "10.0.0.2"), sv("Ethernet"), sv("Dock", "9.9.9.9")}}); !reflect.DeepEqual(again, want) {
		t.Errorf("second copy = %+v", again)
	}
	// The copy was lost while the placeholder was set: the placeholder is
	// never remembered as what the service had.
	if got := Keep(Saved{}, Saved{Services: []Service{held}}); len(got.Services[0].Servers) != 0 {
		t.Errorf("the placeholder was kept as the original: %+v", got)
	}
	// A service that is away keeps its entry.
	if got := Keep(first, Saved{Services: []Service{sv("Ethernet")}}); len(got.Services) != 2 || got.Services[1].Name != "Wi-Fi" {
		t.Errorf("the absent service's copy is gone: %+v", got)
	}
}

func TestTakeSteps(t *testing.T) {
	cur := Saved{Services: []Service{sv("Wi-Fi", Placeholder...), sv("Ethernet"), sv("Dock", "9.9.9.9")}}
	got := TakeSteps(cur)
	if want := []Service{sv("Ethernet", Placeholder...), sv("Dock", Placeholder...)}; !reflect.DeepEqual(got, want) {
		t.Errorf("TakeSteps = %+v", got)
	}
	if HeldAll(cur) || !HeldAll(Saved{Services: []Service{sv("Wi-Fi", Placeholder...)}}) || HeldAll(Saved{}) {
		t.Error("HeldAll is wrong")
	}
}

func TestRestoreSteps(t *testing.T) {
	saved := Saved{Services: []Service{sv("Wi-Fi", "10.0.0.2"), sv("Ethernet"), sv("Dock", "9.9.9.9"), sv("Gone", "4.4.4.4")}}
	cur := Saved{Services: []Service{
		sv("Wi-Fi", Placeholder...),    // back to what the user had
		sv("Ethernet", Placeholder...), // back to the network's own
		sv("Dock", "8.8.8.8"),          // changed by someone else since: left alone
		sv("New", Placeholder...),      // not in the copy: the network's own
	}}
	got := RestoreSteps(saved, cur)
	want := []Service{sv("Wi-Fi", "10.0.0.2"), sv("Ethernet"), sv("New")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RestoreSteps = %+v", got)
	}
	if got := RestoreSteps(saved, Saved{Services: []Service{sv("Wi-Fi", "10.0.0.2")}}); len(got) != 0 {
		t.Errorf("nothing held, but %+v", got)
	}
}

func TestLocal(t *testing.T) {
	saved := Saved{Services: []Service{
		{Name: "Wi-Fi", Device: "en1", Servers: []string{"10.0.0.2", "fe80::1%en1"}},
		{Name: "Ethernet", Device: "en0"},
	}}
	dhcp := []string{"192.168.50.1", "198.18.0.2", "127.0.0.1", "223.5.5.5"}
	if got := Local(saved, "en1", dhcp); !slices.Equal(got, []string{"10.0.0.2"}) {
		t.Errorf("the user's own servers: %v", got)
	}
	if got := Local(saved, "en0", dhcp); !slices.Equal(got, []string{"192.168.50.1"}) {
		t.Errorf("the network's servers: %v", got)
	}
	if got := Local(Saved{}, "", nil); got != nil {
		t.Errorf("nothing known: %v", got)
	}
}
