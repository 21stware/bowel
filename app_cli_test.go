package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/oboo/moho/internal/cli"
	"github.com/oboo/moho/internal/core"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/mcp"
	"github.com/oboo/moho/internal/routes"
	"github.com/oboo/moho/internal/settings"
)

// cliApp is mcpApp serving the command line on its socket. run runs a
// command against it as a person would, or as the agent named, with no
// terminal to ask at (as in a script, `docker exec` without -t, or `ssh`).
func cliApp(t *testing.T) (a *App, asks *int, run func(agent string, args ...string) (int, string, string)) {
	t.Helper()
	return cliAppAsking(t, nil, nil)
}

// cliAppAsking is cliApp with a terminal: ask answers what the command
// asks there, nil being no terminal, and *person says whether a person's
// terminal is behind the command (cli.Env.Person), nil being no.
func cliAppAsking(t *testing.T, ask func(prompt string) (string, error), person *bool) (a *App, asks *int, run func(agent string, args ...string) (int, string, string)) {
	t.Helper()
	if ask == nil {
		ask = func(string) (string, error) { return "", cli.ErrNoTerminal }
	}
	if person == nil {
		person = new(bool)
	}
	a, asks, _ = mcpApp(t)
	l, err := mcp.Listen(cliSocket(a.dataDir))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go a.cliServer().Accept(ctx, l)
	run = func(agent string, args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		code := cli.Run(cli.Env{
			Args: args, Stdout: &out, Stderr: &errOut, Socket: cliSocket(a.dataDir), Version: "test", Ask: ask,
			Person: func() bool { return *person },
			Getenv: func(k string) string {
				if k == "BOWEL_AGENT" {
					return agent
				}
				return ""
			},
		})
		return code, out.String(), errOut.String()
	}
	return a, asks, run
}

// TestCLIRoundTrip: commands reach the app over its socket and come back,
// reading and changing, in words and as JSON.
func TestCLIRoundTrip(t *testing.T) {
	a, asks, run := cliApp(t)
	if _, err := a.settings.Update(func(s *settings.Settings) { s.Language = "en" }); err != nil {
		t.Fatal(err)
	}
	i18n.Set("en")

	code, out, errOut := run("", "status")
	if code != 0 || !strings.Contains(out, "Not taking over") || !strings.Contains(out, "Rule") {
		t.Fatalf("status: %d\n%s\n%s", code, out, errOut)
	}
	code, out, _ = run("", "status", "--json")
	var st struct {
		State, Mode string
		Routes      []struct{ ID string }
	}
	if err := json.Unmarshal([]byte(out), &st); err != nil || code != 0 || st.State != "off" || st.Mode != "rule" || len(st.Routes) == 0 {
		t.Fatalf("status --json: %v\n%s", err, out)
	}

	// A change: the user's own command is not asked about, whatever the
	// policy for agents.
	setPolicy(t, a, "deny")
	if code, out, errOut = run("", "mode", "global"); code != 0 || a.settings.Get().Mode != "global" || !strings.Contains(out, "Global") {
		t.Fatalf("mode global: %d %q %q", code, out, errOut)
	}
	if _, out, _ = run("", "mode"); out != "global\n" {
		t.Errorf("mode: %q", out)
	}
	if code, _, _ = run("", "mode", "rule"); code != 0 || a.settings.Get().Mode != "rule" {
		t.Errorf("mode rule: %d", code)
	}

	// What the core has no part in: where an address would go, the
	// interface's language, and the policy made stricter, since it is the
	// user who asks (making it looser needs a terminal: TestCLIPolicy).
	_, out, _ = run("", "rules", "test", "api.openai.com", "--json")
	var ex struct {
		Route string
		Sure  bool
	}
	if err := json.Unmarshal([]byte(out), &ex); err != nil || ex.Route != "ai" || !ex.Sure {
		t.Errorf("rules test --json: %v %s", err, out)
	}
	if code, out, errOut = run("", "settings", "set", "language", "ja"); code != 0 || a.settings.Get().Language != "ja" || !strings.Contains(out, "language: en → ja") {
		t.Errorf("settings set: %d %q %q", code, out, errOut)
	}
	if _, out, _ = run("", "settings", "get", "language"); out != "ja\n" {
		t.Errorf("settings get: %q", out)
	}
	run("", "settings", "set", "language", "en")
	if code, _, errOut = run("", "settings", "set", "language", "xx"); code != cli.ExitFail || !strings.Contains(errOut, "Choices") {
		t.Errorf("a bad value: %d %q", code, errOut)
	}
	setPolicy(t, a, "allow")
	if code, _, errOut = run("", "settings", "set", "mcpPolicy", "ask"); code != 0 || a.settings.Get().Policy() != "ask" {
		t.Errorf("the policy: %d %q", code, errOut)
	}
	for _, args := range [][]string{{"routes"}, {"nodes"}, {"providers"}, {"conns"}, {"logs"}, {"settings"}, {"routes", "--json"}, {"nodes", "--json"}} {
		if code, _, errOut = run("", args...); code != 0 {
			t.Errorf("%v: %d %q", args, code, errOut)
		}
	}
	if *asks != 0 || len(agentChanges()) != 0 {
		t.Errorf("the user's commands asked %d times and left %d agent changes", *asks, len(agentChanges()))
	}
}

func TestCLISessionIs(t *testing.T) {
	yes, no := true, false
	for _, c := range []struct {
		agent     string
		terminal  *bool
		container bool
		want      string
	}{
		{"Robot", &yes, false, "agent"}, {"Robot", &yes, true, "agent"},
		{"", nil, false, "user"}, {"", &no, false, "user"}, {"", &yes, false, "user"},
		{"", &yes, true, "user"}, {"", &no, true, "unattended"}, {"", nil, true, "unattended"},
	} {
		if got := cliSessionIs(c.agent, c.terminal, c.container); got != c.want {
			t.Errorf("%q, terminal %v, container %v: %s", c.agent, c.terminal != nil && *c.terminal, c.container, got)
		}
	}
}

// TestCLIPolicy: the user's own command makes the policy stricter at once,
// and looser only once the person at the terminal types the policy's name.
// Being on cli.sock says no more than that the environment named no agent,
// and an agent's environment stays behind on `docker exec` and `ssh`.
func TestCLIPolicy(t *testing.T) {
	var prompts []string
	answer := ""
	a, asks, run := cliAppAsking(t, func(p string) (string, error) {
		prompts = append(prompts, p)
		if answer == "no terminal" {
			return "", cli.ErrNoTerminal
		}
		return answer, nil
	}, nil)
	if _, err := a.settings.Update(func(s *settings.Settings) { s.Language = "en" }); err != nil {
		t.Fatal(err)
	}
	i18n.Set("en")
	policy := func() string { return a.settings.Get().Policy() }

	// No terminal, as for `docker exec bowel bowel …` or `ssh host bowel …`
	// run by an agent: refused, saying who can and where.
	answer = "no terminal"
	for _, c := range [][2]string{{"ask", "allow"}, {"deny", "allow"}, {"deny", "ask"}} {
		setPolicy(t, a, c[0])
		code, _, errOut := run("", "settings", "set", "mcpPolicy", c[1])
		if code != cli.ExitFail || policy() != c[0] || !strings.Contains(errOut, "Only the user can let agents do more (mcpPolicy "+c[0]+" → "+c[1]+")") || !strings.Contains(errOut, "docker exec -it") {
			t.Errorf("%s → %s with no terminal: %d, policy %s, %q", c[0], c[1], code, policy(), errOut)
		}
	}
	// Stricter, or as it is: nothing to ask, terminal or not.
	setPolicy(t, a, "allow")
	n := len(prompts)
	for _, to := range []string{"ask", "deny", "deny", `"deny"`} {
		if code, _, errOut := run("", "settings", "set", "mcpPolicy", to); code != 0 || policy() != strings.Trim(to, `"`) {
			t.Errorf("→ %s: %d, policy %s, %q", to, code, policy(), errOut)
		}
	}
	if len(prompts) != n {
		t.Errorf("making the policy stricter asked: %q", prompts[n:])
	}

	// At a terminal: anything but the policy's name, an empty line too,
	// cancels; the name lets it through.
	for _, ans := range []string{"y", "", "yes allow"} {
		answer = ans
		if code, _, errOut := run("", "settings", "set", "mcpPolicy", "allow"); code != cli.ExitFail || policy() != "deny" || !strings.Contains(errOut, "Cancelled: mcpPolicy stays deny") {
			t.Errorf("answered %q: %d, policy %s, %q", ans, code, policy(), errOut)
		}
	}
	p := prompts[len(prompts)-1]
	for _, want := range []string{"deny → allow", "without asking you", "Only you can allow this", "if an agent ran this command, press Enter", "Type allow to confirm"} {
		if !strings.Contains(p, want) {
			t.Errorf("the question lacks %q:\n%s", want, p)
		}
	}
	answer = " Allow\n"
	if code, out, errOut := run("", "settings", "set", "mcpPolicy", "allow"); code != 0 || policy() != "allow" || !strings.Contains(out, "mcpPolicy: deny → allow") {
		t.Errorf("confirmed: %d, policy %s, %q %q", code, policy(), out, errOut)
	}

	// An agent's command, known by its environment, is not asked: the
	// policy isn't its to change, typed confirmation or not.
	setPolicy(t, a, "ask")
	n = len(prompts)
	if code, _, _ := run("Robot", "settings", "set", "mcpPolicy", "allow"); code != cli.ExitFail || policy() != "ask" || len(prompts) != n {
		t.Errorf("an agent's command: %d, policy %s, %d questions", code, policy(), len(prompts)-n)
	}

	// The running bowel holds to it, whoever the client: the user's own
	// session needs confirmed to loosen it, and an agent's can't with it.
	user := &mcp.Session{}
	user.Set("local", true)
	loosen := map[string]any{"changes": map[string]any{"mcpPolicy": "allow"}}
	if _, err := callTool(t, a.cliServer(), user, "update_settings", loosen); err == nil || policy() != "ask" || !strings.Contains(err.Error(), "Only the user can let agents do more") {
		t.Errorf("loosened unconfirmed: %v, policy %s", err, policy())
	}
	loosen["confirmed"] = true
	for name, srv := range map[string]*mcp.Server{"mcp.sock": a.mcpServer(), "cli.sock": a.cliServer()} {
		if _, err := callTool(t, srv, &mcp.Session{}, "update_settings", loosen); err == nil || policy() != "ask" {
			t.Errorf("an agent's session on %s loosened it: %v", name, err)
		}
	}
	if _, err := callTool(t, a.cliServer(), user, "update_settings", loosen); err != nil || policy() != "allow" {
		t.Errorf("confirmed: %v, policy %s", err, policy())
	}
	if *asks != 0 {
		t.Errorf("the policy's changes asked the app %d times", *asks)
	}
}

// TestCLIAgent: a command an agent runs follows the policy for agents, as
// on the agents' socket, and the agents' socket has none of the command
// line's own tools.
func TestCLIAgent(t *testing.T) {
	a, asks, run := cliApp(t)
	i18n.Set("en")

	setPolicy(t, a, "deny")
	if code, out, _ := run("Robot", "status"); code != 0 || out == "" {
		t.Errorf("an agent may read: %d", code)
	}
	code, _, errOut := run("Robot", "mode", "global")
	if code != cli.ExitFail || a.settings.Get().Mode != "rule" || !strings.Contains(errOut, "not to let agents make changes") {
		t.Errorf("deny let an agent's command through: %d %q", code, errOut)
	}
	if code, _, _ := run("Robot", "stop"); code != cli.ExitFail {
		t.Errorf("deny let an agent stop bowel")
	}
	// The policy is not an agent's to change, by this way either.
	if code, _, _ := run("Robot", "settings", "set", "mcpPolicy", "allow"); code != cli.ExitFail || a.settings.Get().Policy() != "deny" {
		t.Errorf("an agent changed the policy")
	}
	if ch := agentChanges(); len(ch) == 0 || ch[0].Client != "Robot (bowel CLI) test" || ch[0].Outcome != "denied" {
		t.Errorf("the refusal is not listed: %+v", ch)
	}

	// Asked: the user's answer decides.
	setPolicy(t, a, "ask")
	if code, _, _ := run("Robot", "mode", "global"); code != cli.ExitFail || *asks != 1 || a.settings.Get().Mode != "rule" {
		t.Errorf("a refused change: %d, asks %d", code, *asks)
	}
	setPolicy(t, a, "allow")
	if code, _, errOut := run("Robot", "mode", "global"); code != 0 || a.settings.Get().Mode != "global" {
		t.Errorf("allow: %d %q", code, errOut)
	}

	// A session that never says who it is is nobody's: the policy holds.
	setPolicy(t, a, "deny")
	c, err := net.DialTimeout("unix", cliSocket(a.dataDir), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, _ = c.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"set_mode","arguments":{"mode":"direct"}}}` + "\n"))
	buf := make([]byte, 4096)
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, _ := c.Read(buf)
	if !strings.Contains(string(buf[:n]), `"isError":true`) || a.settings.Get().Mode != "global" {
		t.Errorf("a session without initialize changed the mode: %s", buf[:n])
	}

	// The agents' server has only the agents' tools.
	for _, tool := range a.mcpServer().Tools {
		for _, own := range a.cliTools() {
			if tool.Name == own.Name {
				t.Errorf("the agents' server offers %s", tool.Name)
			}
		}
	}
}

// TestCLIWithCore drives the commands that reload the core against a real
// one, on a port of its own, and follows its log.
func TestCLIWithCore(t *testing.T) {
	if testing.Short() {
		t.Skip("starts the core")
	}
	a, _, run := cliApp(t)
	if !exists(bundledCore()) {
		t.Skip("no core; run scripts/fetch-core.sh")
	}
	port, err := core.FreePort()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.settings.Update(func(s *settings.Settings) { s.MixedPort, s.Language = port, "en" }); err != nil {
		t.Fatal(err)
	}
	i18n.Set("en")
	a.copyGeo()
	if err := a.applyConfig(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer a.shutdown()

	// Rules: add, list, explain, remove.
	code, out, errOut := run("", "rules", "add", "example.org", "direct")
	if code != 0 || !strings.Contains(out, "example.org now goes to direct") {
		t.Fatalf("rules add: %d %q %q", code, out, errOut)
	}
	if rs := a.settings.Get().Rules; len(rs) != 1 || rs[0].Route != "direct" || rs[0].Kind != routes.KindSuffix {
		t.Fatalf("rules: %+v", rs)
	}
	if _, out, _ := run("", "rules"); !strings.Contains(out, "example.org") || !strings.Contains(out, "direct") {
		t.Errorf("rules list:\n%s", out)
	}
	if _, out, _ := run("", "rules", "test", "https://www.example.org/x"); !strings.Contains(out, "www.example.org → direct") || !strings.Contains(out, "your rule") {
		t.Errorf("rules test:\n%s", out)
	}
	if code, _, errOut := run("", "rules", "remove", "example.org"); code != 0 || len(a.settings.Get().Rules) != 0 {
		t.Errorf("rules remove: %d %q", code, errOut)
	}
	if code, _, errOut := run("", "rules", "add", "example.org", "nowhere"); code != cli.ExitFail || !strings.Contains(errOut, "No such route") {
		t.Errorf("a bad route: %d %q", code, errOut)
	}

	if code, out, errOut := run("", "settings", "set", "logLevel", "debug"); code != 0 || a.settings.Get().LogLevel != "debug" || !strings.Contains(out, "logLevel: info → debug") {
		t.Errorf("settings set: %d %q %q", code, out, errOut)
	}
	if code, out, errOut := run("", "status"); code != 0 || !strings.Contains(out, "mihomo v") {
		t.Errorf("status: %d %q %q", code, out, errOut)
	}
	if code, _, errOut := run("", "dns", "flush"); code != 0 {
		t.Errorf("dns flush: %d %q", code, errOut)
	}
}
