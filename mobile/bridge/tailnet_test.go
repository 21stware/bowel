package bridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oboo/moho/internal/settings"
)

// What the core said of its node, as the host keeps it.
func TestReadTailnetLog(t *testing.T) {
	line := func(level, msg string) string {
		return `time="2026-11-13T01:34:10-05:00" level=` + level + ` msg="[Tailscale](Tailscale-net) ` + msg + `"`
	}
	cases := []struct {
		name  string
		lines []string
		want  tailnetData
	}{
		{"nothing yet", nil, tailnetData{State: "idle"}},
		{"starting", []string{line("info", `tsnet starting with hostname \"bowel-android\", varRoot \"/data/x/tailscale\"`)}, tailnetData{State: "starting"}},
		{"needs login", []string{
			line("info", `tsnet starting with hostname \"x\", varRoot \"y\"`),
			line("info", "LocalBackend state is NeedsLogin; running StartLoginInteractive..."),
			line("info", "To start this tsnet server, restart with TS_AUTHKEY set, or go to: https://login.tailscale.com/a/g6238831c96531e2d"),
		}, tailnetData{State: "login", LoginURL: "https://login.tailscale.com/a/g6238831c96531e2d"}},
		{"logged in", []string{
			line("info", "To start this tsnet server, restart with TS_AUTHKEY set, or go to: https://login.tailscale.com/a/x"),
			line("info", "AuthLoop: state is Starting; done"),
		}, tailnetData{State: "on"}},
		{"restarted after login", []string{
			line("info", "AuthLoop: state is Running; done"),
			line("info", `tsnet starting with hostname \"x\", varRoot \"y\"`),
		}, tailnetData{State: "starting"}},
		{"other lines", []string{
			`time="x" level=info msg="[TCP] 198.18.0.1:1 --> 100.85.4.93:80 match IPCIDR(100.64.0.0/10) using Tailscale-net"`,
		}, tailnetData{State: "idle"}},
	}
	for _, c := range cases {
		path := filepath.Join(t.TempDir(), tailnetLog)
		if err := os.WriteFile(path, []byte(strings.Join(c.lines, "\n")), 0o600); err != nil {
			t.Fatal(err)
		}
		d := tailnetData{State: "idle"}
		readTailnetLog(path, &d)
		if d != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, d, c.want)
		}
	}
}

// Off, or where it can't run, the setting adds nothing to the config.
func TestTailnetOverlay(t *testing.T) {
	if !tailnetOverlay(settings.Tailnet{}).Empty() {
		t.Error("off: not empty")
	}
	o := tailnetOverlay(settings.Tailnet{On: true, StateID: "ab"})
	if canTailnet == o.Empty() {
		t.Errorf("on: empty = %v on this system", o.Empty())
	}
	if canTailnet {
		tn := o.Outbounds[0].Tsnet
		if tn.Hostname != defaultHostname || tn.StateDir != "tailscale/ab" {
			t.Errorf("%+v", tn)
		}
	}
}
