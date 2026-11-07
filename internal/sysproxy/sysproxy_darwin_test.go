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
