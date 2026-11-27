package cli

// Loosening the policy for agents (mcpPolicy) from the command line. A
// command is the user's unless its environment names an agent (Agent), and
// that environment stays behind on the way through `docker exec` and `ssh`:
// an agent's `ssh host bowel settings set mcpPolicy allow` reads as the
// user's (in the Docker image, a command with no person's terminal is an
// agent's: app_cli.go). So letting agents do more takes the person at a
// terminal, typing the policy's name; tightening it takes nothing. The
// running bowel holds to it too (app_mcp.go's policyChange): without the
// confirmation it refuses.

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/oboo/moho/internal/i18n"
)

// ErrNoTerminal is Ask's answer where there is no terminal to ask at.
var ErrNoTerminal = errors.New("no terminal")

// policyRank orders the policies by what they let agents do, as
// settings.PolicyLooser does.
var policyRank = map[string]int{"deny": 0, "ask": 1, "": 1, "allow": 2}

// looser says whether going from the policy from to to lets agents do more.
func looser(from, to string) bool {
	f, ok1 := policyRank[from]
	t, ok2 := policyRank[to]
	return ok1 && ok2 && t > f
}

// policyMeans is what agents may do under the policy p, in a sentence.
func policyMeans(p string) string {
	if p == "allow" {
		return i18n.T("agent（Claude Code、Codex、Cursor 等）修改 bowel 将不再询问你", "Agents (Claude Code, Codex, Cursor…) will change bowel without asking you")
	}
	return i18n.T("agent 可以请求修改 bowel：应用每次先问你，daemon 会拒绝", "Agents may ask to change bowel: the app asks you each time, and a daemon refuses")
}

// askTimeout is how long confirmPolicy waits for an answer: a person's
// terminal that nobody types at (`docker exec -t` with no -i, from one)
// ends with no answer.
var askTimeout = time.Minute

// confirmPolicy asks the person at the terminal to type to, before the
// policy goes from from to to.
func (c *Ctx) confirmPolicy(from, to string) error {
	ask := c.env.Ask
	if ask == nil {
		ask = askLine
	}
	answer, err := ask(fmt.Sprintf(i18n.T("mcpPolicy：%s → %s。%s。\n只有你能允许：如果这条命令是 agent 运行的，直接回车取消。\n输入 %s 确认：",
		"mcpPolicy: %s → %s. %s.\nOnly you can allow this: if an agent ran this command, press Enter to cancel.\nType %s to confirm: "), from, to, policyMeans(to), to))
	switch {
	case errors.Is(err, ErrNoTerminal):
		return fmt.Errorf(i18n.T("只有用户能放宽 agent 的权限（mcpPolicy %s → %s），而且要在终端里确认：在用户自己的终端里执行 `bowel settings set mcpPolicy %s`（Docker 用 `docker exec -it`，SSH 用 `ssh -t`），或用 `bowel tui`，或在应用的 设置 > MCP 里修改。",
			"Only the user can let agents do more (mcpPolicy %s → %s), and confirms it at a terminal: `bowel settings set mcpPolicy %s` in a terminal of their own (`docker exec -it` in Docker, `ssh -t` over SSH), `bowel tui`, or Settings > MCP in the app."), from, to, to)
	case err != nil:
		return err
	case !strings.EqualFold(strings.TrimSpace(answer), to):
		return fmt.Errorf(i18n.T("已取消：mcpPolicy 仍是 %s", "Cancelled: mcpPolicy stays %s"), from)
	}
	return nil
}

// askLine is Ask at this process's terminal: the question on stderr, the
// answer a line of stdin, a person's terminal both (atTerminal: not the
// 0×0 one an agent's `docker exec -t` gets).
func askLine(prompt string) (string, error) {
	if !atTerminal(os.Stdin) || !IsTerminal(os.Stderr) {
		return "", ErrNoTerminal
	}
	fmt.Fprint(os.Stderr, prompt)
	line := make(chan string, 1)
	go func() {
		s, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		line <- s
	}()
	select {
	case s := <-line:
		return s, nil
	case <-time.After(askTimeout):
		fmt.Fprintln(os.Stderr)
		return "", nil // no answer is no
	}
}
