package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/oboo/moho/internal/cli"
	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/provider"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
	"gopkg.in/yaml.v3"
)

func TestContainerMode(t *testing.T) {
	for v, want := range map[string]bool{"": false, " ": false, "0": false, "false": false, "No": false, "off": false,
		"1": true, "true": true, "yes": true, "docker": true} {
		if got := isContainer(v); got != want {
			t.Errorf("%q: container %v", v, got)
		}
	}
}

// TestContainerSeed: a container's first settings serve the proxy to the
// private networks, Docker's bridges among them, and to no one else.
func TestContainerSeed(t *testing.T) {
	dir := t.TempDir()
	st, err := settings.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s, err := st.Update(func(s *settings.Settings) { containerSeed(s, "en") })
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if !s.AllowLan || s.SystemProxy || s.Language != "en" || s.FindProcess != "off" {
		t.Errorf("seed: allowLan %v systemProxy %v language %s findProcess %s", s.AllowLan, s.SystemProxy, s.Language, s.FindProcess)
	}
	for _, want := range []string{"172.16.0.0/12", "10.0.0.0/8", "192.168.0.0/16", "127.0.0.0/8", "fc00::/7", "::1/128"} {
		if !slices.Contains(s.LanAllowedIPs, want) {
			t.Errorf("lanAllowedIps lacks %s: %v", want, s.LanAllowedIPs)
		}
	}
	if slices.Contains(s.LanAllowedIPs, "0.0.0.0/0") || slices.Contains(s.LanAllowedIPs, "::/0") {
		t.Errorf("lanAllowedIps lets everyone in: %v", s.LanAllowedIPs)
	}
	// Saved, and read back as saved: Fill doesn't put the default back.
	again, err := settings.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.Get(); !got.AllowLan || !slices.Equal(got.LanAllowedIPs, s.LanAllowedIPs) {
		t.Errorf("read back: allowLan %v %v", got.AllowLan, got.LanAllowedIPs)
	}
	// The core is told: the port on every address, from those networks.
	b, err := core.BuildConfig(core.Input{Routes: routes.Template(), Mode: s.Mode, MixedPort: s.MixedPort, LogLevel: s.LogLevel,
		AllowLan: s.AllowLan, TestURL: s.TestURL, ProbePort: 17900, Options: s.Options})
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		AllowLan bool     `yaml:"allow-lan"`
		Bind     string   `yaml:"bind-address"`
		IPs      []string `yaml:"lan-allowed-ips"`
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.AllowLan || (cfg.Bind != "" && cfg.Bind != "*") || !slices.Equal(cfg.IPs, s.LanAllowedIPs) {
		t.Errorf("config: allow-lan %v bind %q ips %v", cfg.AllowLan, cfg.Bind, cfg.IPs)
	}
}

func TestEnvSettings(t *testing.T) {
	env := map[string]string{"BOWEL_MIXED_PORT": "7890", "BOWEL_ALLOW_LAN": "false", "BOWEL_LAN_ALLOWED_IPS": "10.0.0.0/8, 192.168.1.0/24"}
	changes, lines := envSettings(func(k string) string { return env[k] })
	s := settings.Defaults.Clone()
	for _, c := range changes {
		c(&s)
	}
	if s.MixedPort != 7890 || s.AllowLan || !slices.Equal(s.LanAllowedIPs, []string{"10.0.0.0/8", "192.168.1.0/24"}) || len(lines) != 3 {
		t.Errorf("port %d allowLan %v ips %v lines %q", s.MixedPort, s.AllowLan, s.LanAllowedIPs, lines)
	}
	env = map[string]string{"BOWEL_MIXED_PORT": "70000", "BOWEL_ALLOW_LAN": "maybe", "BOWEL_LAN_ALLOWED_IPS": "10.0.0.1"}
	changes, lines = envSettings(func(k string) string { return env[k] })
	if len(changes) != 0 || len(lines) != 3 {
		t.Errorf("bad values: %d changes, lines %q", len(changes), lines)
	}
	if changes, lines = envSettings(func(string) string { return "" }); len(changes)+len(lines) != 0 {
		t.Errorf("no environment: %d changes, lines %q", len(changes), lines)
	}
}

func TestEnvSources(t *testing.T) {
	v := "https://a.example/sub?token=x&list=a,b, https://b.example/s\n\n  vless://id@h:443?type=ws#Tokyo, 02 ,trojan://pw@h:443#c\n"
	want := []string{"https://a.example/sub?token=x&list=a,b", "https://b.example/s", "vless://id@h:443?type=ws#Tokyo, 02", "trojan://pw@h:443#c"}
	if got := envSources(v); !slices.Equal(got, want) {
		t.Errorf("envSources:\n got %q\nwant %q", got, want)
	}
	if got := envSources(" \n "); len(got) != 0 {
		t.Errorf("blank: %q", got)
	}
}

// TestEnvSourceKnown: a source a Provider has already is not added again,
// a subscription by its URL and a link by its file.
func TestEnvSourceKnown(t *testing.T) {
	sub, link := "https://a.example/sub?token=secret", "trojan://pw@h.example:443#a"
	ps := []provider.Provider{
		{ID: "s1", Kind: provider.Subscription, Source: sub},
		{ID: "l1", Kind: provider.Link},
	}
	content := func(id string) string {
		if id == "l1" {
			return link + "\n"
		}
		return ""
	}
	if p, ok := hasSource(ps, sub, content); !ok || p.ID != "s1" {
		t.Errorf("subscription: %v %v", p.ID, ok)
	}
	if p, ok := hasSource(ps, link, content); !ok || p.ID != "l1" {
		t.Errorf("link: %v %v", p.ID, ok)
	}
	if _, ok := hasSource(ps, "https://a.example/sub?token=other", content); ok {
		t.Error("another URL counts as known")
	}
	if _, ok := hasSource(ps, "trojan://pw@other:443#a", content); ok {
		t.Error("another link counts as known")
	}

	// And through the app: the provider stays one.
	a, _, _ := mcpApp(t)
	if _, err := a.providers.Add(context.Background(), nil, link); err != nil {
		t.Fatal(err)
	}
	if !a.addEnvProvider(link) || len(a.providers.List()) != 1 {
		t.Errorf("the link was added again: %d providers", len(a.providers.List()))
	}
	if !a.addEnvProvider("not a source") {
		t.Error("what is no source is tried again")
	}
}

// TestEnvRedact: the log names a subscription by its host, and its
// failures without the URL, whose path or query holds the token.
func TestEnvRedact(t *testing.T) {
	src := "https://sub.example/api/v1/client/subscribe?token=s3cr3t"
	if h := sourceHost(src); h != "sub.example" {
		t.Errorf("host: %q", h)
	}
	if h := sourceHost("vless://uuid@h:443#n"); h != "vless link" || strings.Contains(h, "uuid") {
		t.Errorf("link: %q", h)
	}
	err := fmt.Errorf("download: %w", &url.Error{Op: "Get", URL: src, Err: errors.New("dial tcp: i/o timeout")})
	if got := redact(err, src); strings.Contains(got, "s3cr3t") || !strings.Contains(got, "timeout") {
		t.Errorf("url.Error: %q", got)
	}
	err = errors.New("HTTP 403 for " + src)
	if got := redact(err, src); strings.Contains(got, "s3cr3t") {
		t.Errorf("plain: %q", got)
	}
}

func TestHealthText(t *testing.T) {
	i18n.Set("en")
	defer i18n.Set("zh")
	if ok, text := healthText(core.Status{State: core.Running, Version: "v1.19"}, 7899, true); !ok || !strings.Contains(text, "7899") {
		t.Errorf("running: %v %q", ok, text)
	}
	if ok, text := healthText(core.Status{State: core.Running, Error: "listen tcp :7899: address in use\nmore"}, 7899, false); ok || !strings.Contains(text, "address in use") || strings.Contains(text, "more") {
		t.Errorf("port: %v %q", ok, text)
	}
	if ok, text := healthText(core.Status{State: core.Failed, Error: "boom"}, 7899, false); ok || !strings.Contains(text, "boom") {
		t.Errorf("failed: %v %q", ok, text)
	}
}

// TestContainerExecPolicy: `docker exec` runs the command with the
// container's environment, not the agent's, so the agent's call reads as
// the user's. It still can't let agents do more: with no -t there is no
// terminal, with -t and no -i nobody answers (askLine's wait ends with no
// answer), and only the person on `docker exec -it` types the policy in.
func TestContainerExecPolicy(t *testing.T) {
	t.Setenv(containerEnv, "1")
	var answer func() (string, error)
	a, _, run := cliAppAsking(t, func(string) (string, error) { return answer() })
	if _, err := a.settings.Update(func(s *settings.Settings) { s.Language = "en" }); err != nil {
		t.Fatal(err)
	}
	i18n.Set("en")
	for name, ans := range map[string]func() (string, error){
		"docker exec":    func() (string, error) { return "", cli.ErrNoTerminal },
		"docker exec -t": func() (string, error) { return "", nil },
	} {
		answer = ans
		if code, _, errOut := run("", "settings", "set", "mcpPolicy", "allow"); code != cli.ExitFail || a.settings.Get().Policy() != "ask" {
			t.Errorf("%s let agents do more: %d %q", name, code, errOut)
		}
	}
	// Tightening it needs no terminal: an agent may well do that.
	if code, _, errOut := run("", "settings", "set", "mcpPolicy", "deny"); code != 0 || a.settings.Get().Policy() != "deny" {
		t.Errorf("docker exec could not tighten it: %d %q", code, errOut)
	}
	answer = func() (string, error) { return "allow\n", nil }
	if code, _, errOut := run("", "settings", "set", "mcpPolicy", "allow"); code != 0 || a.settings.Get().Policy() != "allow" {
		t.Errorf("docker exec -it, allow typed: %d %q", code, errOut)
	}
}

// TestHealthCommand: `bowel health` exits 1 while the core isn't running,
// and 1 when no bowel answers.
func TestHealthCommand(t *testing.T) {
	a, _, run := cliApp(t)
	i18n.Set("en") // mcpApp sets zh back
	if code, out, errOut := run("", "health"); code != cli.ExitFail || out != "" || !strings.Contains(errOut, "core isn't running") {
		t.Errorf("no core: %d %q %q", code, out, errOut)
	}
	code := cli.Run(cli.Env{Args: []string{"health"}, Stdout: new(strings.Builder), Stderr: new(strings.Builder), Socket: cliSocket(a.dataDir) + ".none"})
	if code != cli.ExitFail {
		t.Errorf("no bowel: exit %d", code)
	}
}

func TestStatZombie(t *testing.T) {
	if z, ppid, ok := statZombie("42 (my (odd) name) Z 1 42 42 0 -1"); !ok || !z || ppid != 1 {
		t.Errorf("zombie: %v %d %v", z, ppid, ok)
	}
	if z, ppid, ok := statZombie("7 (mihomo) S 1 7 7 0"); !ok || z || ppid != 1 {
		t.Errorf("running: %v %d %v", z, ppid, ok)
	}
	if _, _, ok := statZombie("garbage"); ok {
		t.Error("garbage read")
	}
}
