package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/oboo/moho/internal/i18n"
)

// agentMarkers are environment variables agent harnesses set for the
// commands they run, and who that is. A command run under one says so to
// bowel, and its changes then follow the user's policy for agents.
var agentMarkers = []struct{ env, name string }{
	{"BOWEL_AGENT", ""}, // its value is the name
	{"CLAUDECODE", "Claude Code"},
	{"CLAUDE_CODE_ENTRYPOINT", "Claude Code"},
	{"CODEX_SANDBOX", "Codex"},
	{"CODEX_THREAD_ID", "Codex"},
	{"CURSOR_AGENT", "Cursor"},
	{"GEMINI_CLI", "Gemini CLI"},
	{"OPENCODE", "opencode"},
	{"AI_AGENT", ""},
	{"AGENT", ""},
}

// Agent is the agent these commands are run by, "" for a person.
func Agent(getenv func(string) string) string {
	for _, m := range agentMarkers {
		if v := getenv(m.env); v != "" {
			if m.name != "" {
				return m.name
			}
			return v
		}
	}
	return ""
}

// client is a connection to the running bowel's command socket: MCP's
// messages, a line each (internal/mcp).
type client struct {
	conn net.Conn
	br   *bufio.Reader
	id   int
}

// errNotRunning is the failure to reach bowel, said with what to do.
// inCopy: the older socket is in a copy of AppData this terminal can't
// reach (behindCopy), so bowel may be running.
type errNotRunning struct {
	err    error
	inCopy bool
}

func (e errNotRunning) Error() string {
	if e.inCopy {
		return i18n.T("这个终端连不上 bowel。打包应用（如 Claude 桌面版）里的终端看到的是它自己的一份 AppData，而 bowel 0.4.9 及以前把套接字放在那里。bowel 若在运行，重启它就会换成这个版本；否则用 `bowel daemon` 启动，或打开应用。",
			"This terminal can't reach bowel. A terminal of a packaged app, such as Claude's desktop app, sees its own copy of AppData, and bowel 0.4.9 and before keep their socket there. If bowel is running, restart it to run this version; otherwise start it with `bowel daemon`, or open the app.")
	}
	return i18n.T("bowel 没在运行：用 `bowel daemon` 启动，或打开应用。", "bowel isn't running: start it with `bowel daemon`, or open the app.")
}

func (e errNotRunning) Unwrap() error { return e.err }

// behindCopy says that a socket was there and could not be connected to
// because it is in AppData and this process is a packaged app's, which
// sees a copy of AppData: Windows then fails the connect with WSAEINVAL
// (10022), whether anything listens or the file is stale.
func behindCopy(err error) bool {
	return runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(10022))
}

func dial(env Env) (*client, error) {
	conn, err := net.DialTimeout("unix", env.Socket, 2*time.Second)
	if err != nil && env.OlderSocket != "" {
		c, oerr := net.DialTimeout("unix", env.OlderSocket, 2*time.Second)
		if oerr == nil {
			conn, err = c, nil
		} else if behindCopy(oerr) {
			return nil, errNotRunning{oerr, true}
		}
	}
	if err != nil {
		return nil, errNotRunning{err: err}
	}
	cl := &client{conn: conn, br: bufio.NewReaderSize(conn, 64<<10)}
	name, agent := "bowel CLI", Agent(env.Getenv)
	if agent != "" {
		name = agent + " (bowel CLI)"
	}
	person := env.Person
	if person == nil {
		person = func() bool { return atTerminal(os.Stdin) }
	}
	// terminal: in a container, where `docker exec` leaves an agent's
	// environment behind, it is what tells the user (app_cli.go).
	_, err = cl.request("initialize", map[string]any{
		"protocolVersion": "2025-06-18", "agent": agent, "terminal": person(),
		"clientInfo": map[string]any{"name": name, "version": env.Version},
	})
	if err != nil {
		conn.Close()
		return nil, err
	}
	return cl, nil
}

func (cl *client) close() { cl.conn.Close() }

func (cl *client) request(method string, params any) (json.RawMessage, error) {
	cl.id++
	b, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": cl.id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	if _, err := cl.conn.Write(append(b, '\n')); err != nil {
		return nil, errNotRunning{err: err}
	}
	for {
		line, err := cl.br.ReadBytes('\n')
		if err != nil {
			return nil, errors.New(i18n.T("bowel 断开了连接（它可能正在退出）。", "bowel closed the connection (it may be quitting)."))
		}
		var resp struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(line, &resp) != nil || resp.ID != cl.id {
			continue
		}
		if resp.Error != nil {
			return nil, errors.New(resp.Error.Message)
		}
		return resp.Result, nil
	}
}

// call runs a tool and returns what it answered: an object's JSON, or a
// JSON string when the tool answered in words.
func (cl *client) call(tool string, args any) (json.RawMessage, error) {
	if args == nil {
		args = map[string]any{}
	}
	raw, err := cl.request("tools/call", map[string]any{"name": tool, "arguments": args})
	if err != nil {
		if strings.HasPrefix(err.Error(), "unknown tool") {
			return nil, fmt.Errorf(i18n.T("运行中的 bowel 版本较旧，不支持这条命令（%s）。请升级后重启它。", "The running bowel is older and doesn't know this command (%s). Update and restart it."), tool)
		}
		return nil, err
	}
	var res struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Structured json.RawMessage `json:"structuredContent"`
		IsError    bool            `json:"isError"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	text := ""
	if len(res.Content) > 0 {
		text = res.Content[0].Text
	}
	if res.IsError {
		return nil, errors.New(text)
	}
	if len(res.Structured) > 0 {
		return res.Structured, nil
	}
	b, _ := json.Marshal(text)
	return b, nil
}

// call runs a tool of the running bowel, connecting first.
func (c *Ctx) call(tool string, args any) (json.RawMessage, error) {
	if c.cl == nil {
		cl, err := dial(c.env)
		if err != nil {
			return nil, err
		}
		c.cl = cl
	}
	return c.cl.call(tool, args)
}

// get runs a tool and decodes its answer into v.
func (c *Ctx) get(tool string, args, v any) (json.RawMessage, error) {
	raw, err := c.call(tool, args)
	if err != nil {
		return nil, err
	}
	if v != nil && len(raw) > 0 && raw[0] == '{' {
		if err := json.Unmarshal(raw, v); err != nil {
			return nil, err
		}
	}
	return raw, nil
}
