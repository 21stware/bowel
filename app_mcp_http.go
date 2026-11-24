package main

// MCP over HTTP, for agents on other machines or in other containers (the
// Docker image turns it on): BOWEL_MCP_HTTP is the address to listen on,
// and every request brings a bearer token, BOWEL_MCP_TOKEN or the one kept
// in the data folder. The tools and the user's policy are the same as on
// the socket.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/egoist/mygo"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/mcp"
)

const (
	mcpHTTPEnv  = "BOWEL_MCP_HTTP"
	mcpTokenEnv = "BOWEL_MCP_TOKEN"
	// mcpHTTPPort is the port the snippets name when the address has none.
	mcpHTTPPort = "7900"
)

// mcpTokenFile keeps the endpoint's token when BOWEL_MCP_TOKEN is not set.
func mcpTokenFile(dataDir string) string { return filepath.Join(dataDir, "mcp-token") }

// mcpHTTPAddr is the address BOWEL_MCP_HTTP asks for, "" when it is off. A
// bare port listens on every interface.
func mcpHTTPAddr(getenv func(string) string) string {
	v := strings.TrimSpace(getenv(mcpHTTPEnv))
	if v != "" && !strings.Contains(v, ":") {
		v = ":" + v
	}
	return v
}

// mcpHTTPToken is the token the endpoint takes and where it comes from:
// BOWEL_MCP_TOKEN, or the file, made the first time when create is true.
func mcpHTTPToken(dataDir string, getenv func(string) string, create bool) (token, from string, err error) {
	if t := strings.TrimSpace(getenv(mcpTokenEnv)); t != "" {
		if len(t) < 16 {
			return "", mcpTokenEnv, fmt.Errorf("%s is shorter than 16 characters", mcpTokenEnv)
		}
		return t, mcpTokenEnv, nil
	}
	path := mcpTokenFile(dataDir)
	if create {
		token, err = mcp.Token(path)
	} else {
		token, err = mcp.ReadToken(path)
	}
	return token, path, err
}

// mcpHTTPState is the endpoint as get_status and 设置 > MCP show it; nil
// when it is off.
type mcpHTTPState struct {
	URL   string `json:"url,omitempty"`
	Error string `json:"error,omitempty"`
}

var mcpHTTP atomic.Pointer[mcpHTTPState]

// mcpHTTPView is the endpoint for get_status.
func mcpHTTPView() map[string]any {
	st := mcpHTTP.Load()
	if st == nil {
		return map[string]any{"on": false, "enable": mcpHTTPEnv + "=:" + mcpHTTPPort}
	}
	return map[string]any{"on": st.Error == "", "url": st.URL, "error": st.Error}
}

// serveMCPHTTP serves the tools over HTTP until ctx is done, when
// BOWEL_MCP_HTTP asks for it.
func (a *App) serveMCPHTTP(ctx context.Context) {
	addr := mcpHTTPAddr(os.Getenv)
	if addr == "" {
		mcpHTTP.Store(nil)
		return
	}
	fail := func(err error) {
		log.Println("mcp http:", err)
		mcpHTTP.Store(&mcpHTTPState{URL: "http://" + addr + mcp.HTTPPath, Error: err.Error()})
	}
	token, from, err := mcpHTTPToken(a.dataDir, os.Getenv, true)
	if err != nil {
		fail(fmt.Errorf("not serving: %w", err))
		return
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		fail(err)
		return
	}
	url := "http://" + l.Addr().String() + mcp.HTTPPath
	log.Printf("mcp http: listening on %s, token in %s", url, from)
	mcpHTTP.Store(&mcpHTTPState{URL: url})
	if err := a.mcpServer().AcceptHTTP(ctx, l, token); err != nil && ctx.Err() == nil {
		fail(err)
	}
}

// runMCP is `bowel mcp`: the stdio bridge, or with "token", how an agent
// elsewhere reaches the HTTP endpoint.
func runMCP(args []string) error {
	if len(args) == 0 {
		return runMCPBridge()
	}
	if args[0] != "token" {
		return fmt.Errorf(i18n.T("未知的 mcp 子命令 %s；可以用 bowel mcp 或 bowel mcp token", "Unknown mcp subcommand %s; use bowel mcp or bowel mcp token"), args[0])
	}
	quiet := len(args) > 1 && (args[1] == "-q" || args[1] == "--quiet")
	data, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return err
	}
	return printMCPToken(os.Stdout, os.Stderr, data, os.Getenv, quiet)
}

// printMCPToken prints the endpoint's token and how clients use it. It
// reads the data folder itself, so it works with the daemon stopped; the
// token it makes is the one the daemon will take.
func printMCPToken(out, errOut io.Writer, dataDir string, getenv func(string) string, quiet bool) error {
	addr := mcpHTTPAddr(getenv)
	token, from, err := mcpHTTPToken(dataDir, getenv, addr != "")
	if addr == "" {
		fmt.Fprintf(errOut, i18n.T("MCP 的 HTTP 服务没有开启：启动 bowel 时设置 %s=:%s 来开启。\n", "MCP over HTTP is off: start bowel with %s=:%s to turn it on.\n"), mcpHTTPEnv, mcpHTTPPort)
		if err != nil {
			return errors.New(i18n.T("还没有令牌", "There is no token yet"))
		}
	} else if err != nil {
		return err
	}
	if quiet {
		fmt.Fprintln(out, token)
		return nil
	}
	url := mcpClientURL(addr)
	b, _ := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{"bowel": map[string]any{
		"type": "http", "url": url, "headers": map[string]string{"Authorization": "Bearer " + token}}}}, "", "  ")
	fmt.Fprintf(out, "%s\n\n%s\n  %s\n\n%s\n%s\n\n%s\n",
		token,
		i18n.T("Claude Code：", "Claude Code:"),
		fmt.Sprintf(`claude mcp add --transport http bowel %s --header "Authorization: Bearer %s"`, url, token),
		i18n.T("其他客户端（MCP 配置 JSON）：", "Other clients (MCP config JSON):"),
		b,
		fmt.Sprintf(i18n.T("令牌来自 %s。从别的机器连接时，把 localhost 换成这台机器的地址；令牌以明文经过网络，跨网络请放在 TLS 或 VPN 后面。", "The token is from %s. From another machine, put this machine's address in place of localhost; the token crosses the network in the clear, so across networks keep it behind TLS or a VPN."), from))
	return nil
}

// mcpClientURL is the URL a client on this machine (or the one publishing
// the container's port) uses for the listen address addr.
func mcpClientURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		host, port = "", mcpHTTPPort
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port) + mcp.HTTPPath
}
