package sysproxy

import (
	"reflect"
	"testing"
)

func TestParseGet(t *testing.T) {
	got := parseGet("Enabled: Yes\nServer: 127.0.0.1\nPort: 7899\nAuthenticated Proxy Enabled: 0\n")
	if want := (Endpoint{true, "127.0.0.1", 7899}); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if got := parseGet("Enabled: No\nServer: \nPort: 0\nAuthenticated Proxy Enabled: 0\n"); got != (Endpoint{}) {
		t.Errorf("unset: got %+v", got)
	}
}

func TestParseBypass(t *testing.T) {
	if got := parseBypass("There aren't any bypass domains set on Wi-Fi.\n"); got != nil {
		t.Errorf("none: got %q", got)
	}
	got := parseBypass("*.local\n169.254/16\n")
	if want := []string{"*.local", "169.254/16"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestRestoreSteps: only what differs is written, and proxies go off
// before anything else. Pure: nothing here runs networksetup.
func TestRestoreSteps(t *testing.T) {
	moho := Endpoint{On: true, Host: "127.0.0.1", Port: 7899}
	ours := Service{Web: moho, Secure: moho, Socks: moho, Bypass: []string{"*.local"}}
	named := func(name string, s Service) Service { s.Name = name; return s }
	corp := Endpoint{On: true, Host: "proxy.corp", Port: 8080}
	idle := Endpoint{Host: "old.corp", Port: 3128}

	tests := []struct {
		name  string
		list  []string
		saved []Service
		cur   []Service
		want  [][]string
	}{
		{
			name:  "nothing was set: only states, and no bypass that is the same",
			list:  []string{"Wi-Fi"},
			saved: []Service{{Name: "Wi-Fi", Bypass: []string{"*.local"}}},
			cur:   []Service{named("Wi-Fi", ours)},
			want: [][]string{
				{"-setwebproxystate", "Wi-Fi", "off"},
				{"-setsecurewebproxystate", "Wi-Fi", "off"},
				{"-setsocksfirewallproxystate", "Wi-Fi", "off"},
			},
		},
		{
			name:  "already as saved: nothing to do",
			list:  []string{"Wi-Fi"},
			saved: []Service{{Name: "Wi-Fi", Web: corp, Secure: idle, Bypass: []string{"a"}}},
			cur:   []Service{{Name: "Wi-Fi", Web: corp, Secure: idle, Bypass: []string{"a"}}},
		},
		{
			name:  "off comes before on and bypass, across services",
			list:  []string{"A", "B"},
			saved: []Service{{Name: "A", Web: corp, Bypass: []string{"*.local"}}, {Name: "B", Web: idle, Bypass: nil}},
			cur: []Service{
				{Name: "A", Web: moho, Secure: Endpoint{}, Socks: Endpoint{}, Bypass: []string{"x"}},
				{Name: "B", Web: moho},
			},
			want: [][]string{
				{"-setwebproxy", "B", "old.corp", "3128"},
				{"-setwebproxystate", "B", "off"},
				{"-setwebproxy", "A", "proxy.corp", "8080"},
				{"-setproxybypassdomains", "A", "*.local"},
			},
		},
		{
			name:  "a service that could not be read gets every call",
			list:  []string{"A"},
			saved: []Service{{Name: "A", Web: corp}},
			want: [][]string{
				{"-setsecurewebproxystate", "A", "off"},
				{"-setsocksfirewallproxystate", "A", "off"},
				{"-setwebproxy", "A", "proxy.corp", "8080"},
				{"-setproxybypassdomains", "A", "Empty"},
			},
		},
		{
			name: "a service not saved only goes off",
			list: []string{"New"},
			cur:  []Service{named("New", ours)},
			want: [][]string{
				{"-setwebproxystate", "New", "off"},
				{"-setsecurewebproxystate", "New", "off"},
				{"-setsocksfirewallproxystate", "New", "off"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cur := map[string]Service{}
			for _, s := range tt.cur {
				cur[s.Name] = s
			}
			got := restoreSteps(tt.list, Saved{Services: tt.saved}, cur)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}
