package sysproxy

import (
	"reflect"
	"testing"
)

func TestForget(t *testing.T) {
	saved := Saved{Services: []Service{
		{Name: "Wi-Fi", Web: Endpoint{true, "127.0.0.1", 7899}, Secure: Endpoint{true, "127.0.0.1", 7899}, Socks: Endpoint{true, "localhost", 7899}, Bypass: []string{"10.0.0.0/8"}},
		{Name: "Ethernet", Web: Endpoint{true, "proxy.corp", 8080}, Secure: Endpoint{false, "127.0.0.1", 7000}, Bypass: []string{"corp"}},
		{Name: "Bridge"},
	}}
	saved.Forget(7899, 7000)
	want := Saved{Services: []Service{
		{Name: "Wi-Fi", Bypass: DefaultBypass},
		{Name: "Ethernet", Web: Endpoint{true, "proxy.corp", 8080}, Bypass: []string{"corp"}},
		{Name: "Bridge"},
	}}
	if !reflect.DeepEqual(saved, want) {
		t.Errorf("got %+v\nwant %+v", saved, want)
	}
}
