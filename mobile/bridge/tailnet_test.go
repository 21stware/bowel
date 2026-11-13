package bridge

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/oboo/moho/internal/devnet"
	"github.com/oboo/moho/internal/mihomo"
	"github.com/oboo/moho/internal/settings"
	"gopkg.in/yaml.v3"
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

// The app is outside its own VPN: a subscription on a tailnet machine is
// fetched through the core's listener that leads to the node.
func TestTailnetFetch(t *testing.T) {
	was := canTailnet
	canTailnet = true
	defer func() { canTailnet = was }()
	x := testBridge(t)
	proxyOf := func(raw string) (*url.URL, error) {
		t.Helper()
		r, _ := http.NewRequest(http.MethodGet, raw, nil)
		return x.httpClient().Transport.(*http.Transport).Proxy(r)
	}
	if u, err := proxyOf("https://example.com/sub"); u != nil || err != nil {
		t.Errorf("public: %v %v", u, err)
	}
	if _, err := proxyOf("http://100.101.1.2:3002/sub"); err == nil || !strings.Contains(err.Error(), "Tailscale") {
		t.Errorf("node off: %v", err)
	}

	if _, err := x.set.Update(func(s *settings.Settings) { s.Tailnet.On = true }); err != nil {
		t.Fatal(err)
	}
	if _, err := proxyOf("http://box.tail1.ts.net/sub"); err == nil || !strings.Contains(err.Error(), "VPN") {
		t.Errorf("VPN off: %v", err)
	}

	if err := x.writeConfig(); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(x.dir, "config.yaml"))
	type listener struct {
		Name   string `yaml:"name"`
		Listen string `yaml:"listen"`
		Port   int    `yaml:"port"`
		Proxy  string `yaml:"proxy"`
		Users  []any  `yaml:"users"`
	}
	var cfg struct {
		Listeners []listener `yaml:"listeners"`
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(cfg.Listeners, func(l listener) bool {
		return l.Name == tailnetListener
	})
	if i < 0 {
		t.Fatalf("no %s in %+v", tailnetListener, cfg.Listeners)
	}
	if l := cfg.Listeners[i]; l.Listen != "127.0.0.1" || l.Port != tailnetPort || l.Proxy != devnet.TailscaleOutbound || len(l.Users) != 1 {
		t.Errorf("listener = %+v", l)
	}

	x.client = mihomo.NewUnix(filepath.Join(x.dir, "none.sock")) // the core is up
	u, err := proxyOf("http://[fd7a:115c:a1e0::1]:3002/sub")
	if err != nil {
		t.Fatal(err)
	}
	pass, _ := x.probeLogin().Password()
	if got, _ := u.User.Password(); u.Host != "127.0.0.1:"+strconv.Itoa(tailnetPort) || got != pass || pass == "" {
		t.Errorf("proxy = %v", u)
	}
	if u, _ := proxyOf("https://example.com/sub"); u != nil {
		t.Errorf("public went through the node: %v", u)
	}
}
