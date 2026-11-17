package netsetup

import "testing"

func TestRefused(t *testing.T) {
	for out, want := range map[string]bool{
		"Command requires admin privileges.":           true,
		"Command requires admin privileges.\n":         true,
		"** Error: Command requires admin privileges.": true,
		"": false,
		"Wi-Fi is not a recognized network service.":           false,
		"There aren't any DNS Servers set on Wi-Fi.":           false,
		"Error: The parameters were not valid.\n** Error: -1.": false,
	} {
		if got := Refused(out); got != want {
			t.Errorf("Refused(%q) = %v, want %v", out, got, want)
		}
	}
}
