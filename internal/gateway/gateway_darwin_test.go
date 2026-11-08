package gateway

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oboo/moho/internal/tun"
)

func TestParseGlobal(t *testing.T) {
	iface, router := parseGlobal(`<dictionary> {
  PrimaryInterface : en0
  PrimaryService : E867B5BE-4DFA-4A95-980E-04FBDB7222DA
  Router : 192.168.31.1
}`)
	if iface != "en0" || router != "192.168.31.1" {
		t.Fatalf("parseGlobal = %q, %q", iface, router)
	}
}

func TestParsePorts(t *testing.T) {
	m := parsePorts(`
Hardware Port: Ethernet
Device: en5
Ethernet Address: 16:52:07:1f:aa:12

Hardware Port: Wi-Fi
Device: en0
Ethernet Address: 16:52:07:1f:aa:13

Hardware Port: Thunderbolt Bridge
Device: bridge0
`)
	if !m["en0"] || m["en5"] || m["bridge0"] || len(m) != 1 {
		t.Fatalf("parsePorts = %v", m)
	}
}

// The rule allows the two commands SetForwarding runs and nothing else,
// and sudo's own checker accepts it. Nothing is installed: visudo only
// reads a file of the test's.
func TestRule(t *testing.T) {
	r := rule(501)
	want := "#501 ALL = (root) NOPASSWD: /usr/sbin/sysctl -w net.inet.ip.forwarding=0, /usr/sbin/sysctl -w net.inet.ip.forwarding=1\n"
	if !strings.HasSuffix(r, want) {
		t.Fatalf("rule:\n%s", r)
	}
	if strings.Count(r, "NOPASSWD") != 1 || strings.Contains(r, "ALL = (ALL)") {
		t.Fatalf("rule allows more than it should:\n%s", r)
	}
	if strings.Join(forwardCmd(true), " ") != "/usr/sbin/sysctl -w net.inet.ip.forwarding=1" {
		t.Fatalf("forwardCmd(true) = %v", forwardCmd(true))
	}
	f := filepath.Join(t.TempDir(), "rule")
	if err := os.WriteFile(f, []byte(r), 0o440); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("/usr/sbin/visudo"); err != nil {
		t.Skip("no visudo")
	}
	if out, err := exec.Command("/usr/sbin/visudo", "-cf", f).CombinedOutput(); err != nil {
		t.Fatalf("visudo: %v: %s", err, out)
	}
}

// The rule is checked before it takes its name, and under a name sudo
// skips until then (one with a dot).
func TestPrepareSteps(t *testing.T) {
	steps := prepareSteps("/tmp/x y/rule", "/private/etc/sudoers.d/bowel-gateway-501")
	if len(steps) != 3 {
		t.Fatalf("steps: %q", steps)
	}
	stage := steps[0][3]
	if !strings.HasPrefix(filepath.Base(stage), ".") || filepath.Dir(stage) != "/private/etc/sudoers.d" {
		t.Errorf("staged at %q", stage)
	}
	if !strings.Contains(steps[1][0], "visudo -c") || steps[1][1] != stage {
		t.Errorf("not checked where it is staged: %q", steps[1])
	}
	if !strings.HasPrefix(steps[2][0], "/bin/mv") || steps[2][3] != "/private/etc/sudoers.d/bowel-gateway-501" {
		t.Errorf("last step: %q", steps[2])
	}
	for _, st := range steps {
		if len(st)%2 != 0 {
			t.Errorf("step %q doesn't end on a path", st)
		}
	}
	_ = tun.Step{}
}

func TestReadyNeedsRoot(t *testing.T) {
	f := filepath.Join(t.TempDir(), "rule")
	if err := os.WriteFile(f, []byte(rule(501)), 0o440); err != nil {
		t.Fatal(err)
	}
	if ruleInstalled(f) || ruleInstalled(filepath.Join(t.TempDir(), "missing")) {
		t.Error("a file of the user's counts as the installed rule")
	}
}
