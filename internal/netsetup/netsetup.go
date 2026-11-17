// Package netsetup knows how macOS's networksetup says no for want of an
// administrator, for the system proxy and the system's DNS servers alike.
//
// A Mac with "Require an administrator password to access system-wide
// settings" turned on (System Settings › Privacy & Security › Advanced…)
// lets only root change network settings. networksetup run as the user
// then prints "Command requires admin privileges." and changes nothing
// (issue 189).
package netsetup

import (
	"errors"
	"strings"
)

// ErrAdmin is networksetup refusing a change because the Mac asks for an
// administrator password for system-wide settings.
var ErrAdmin = errors.New("requires admin privileges: this Mac asks for an administrator password to change system-wide settings")

// Refused says whether out, what networksetup printed, is that refusal.
func Refused(out string) bool {
	return strings.Contains(strings.ToLower(out), "requires admin privileges")
}
