package main

// MCP over HTTP, for agents on other machines or in other containers (the
// Docker image turns it on): BOWEL_MCP_HTTP is the address to listen on,
// and every request brings a bearer token, BOWEL_MCP_TOKEN or the one kept
// in the data folder. The tools and the user's policy are the same as on
// the socket. The app, `bowel daemon` and the image all serve it when the
// variable is set; with TLS (app_mcp_tls.go) it is HTTPS.

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"regexp"
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

// mcpRunFile is the endpoint as the running bowel serves it, so that
// `bowel mcp token` can tell a client about it from a shell that lacks the
// service's environment. It goes when the endpoint does.
func mcpRunFile(dataDir string) string { return filepath.Join(dataDir, "mcp-http.json") }

// mcpRun is what mcpRunFile holds.
type mcpRun struct {
	Listen string `json:"listen"`
	TLS    bool   `json:"tls,omitempty"`
	// Cert is the file the certificate is in: the data folder's, or the
	// user's own.
	Cert        string `json:"cert,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

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
	// Fingerprint is the SHA-256 of the TLS certificate, "" without TLS.
	Fingerprint string `json:"fingerprint,omitempty"`
}

var mcpHTTP atomic.Pointer[mcpHTTPState]

// mcpHTTPView is the endpoint for get_status.
func mcpHTTPView() map[string]any {
	st := mcpHTTP.Load()
	if st == nil {
		return map[string]any{"on": false, "enable": mcpHTTPEnv + "=:" + mcpHTTPPort}
	}
	v := map[string]any{"on": st.Error == "", "url": st.URL, "error": st.Error, "tls": st.Fingerprint != ""}
	if st.Fingerprint != "" {
		v["fingerprint"] = st.Fingerprint
	}
	return v
}

// remoteKey marks a session that came over HTTP: an agent on another
// machine, for whom this one's files are not its own.
const remoteKey = "remote"

func isRemote(s *mcp.Session) bool { return s != nil && s.Get(remoteKey) == true }

// serveMCPHTTP serves the tools over HTTP until ctx is done, when
// BOWEL_MCP_HTTP asks for it.
func (a *App) serveMCPHTTP(ctx context.Context) {
	// What a run that crashed left says nothing of this one.
	_ = os.Remove(mcpRunFile(a.dataDir))
	addr := mcpHTTPAddr(os.Getenv)
	if addr == "" {
		mcpHTTP.Store(nil)
		return
	}
	scheme := "http"
	fail := func(err error) {
		log.Println("mcp http:", err)
		mcpHTTP.Store(&mcpHTTPState{URL: scheme + "://" + addr + mcp.HTTPPath, Error: err.Error()})
	}
	if on, _, _ := mcpTLSWanted(os.Getenv); on {
		scheme = "https"
	}
	token, from, err := mcpHTTPToken(a.dataDir, os.Getenv, true)
	if err != nil {
		fail(fmt.Errorf("not serving: %w", err))
		return
	}
	tl, err := mcpHTTPTLS(a.dataDir, os.Getenv, true)
	if err != nil {
		fail(fmt.Errorf("not serving: %w", err))
		return
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		fail(err)
		return
	}
	run := mcpRun{Listen: l.Addr().String()}
	st := &mcpHTTPState{}
	if tl != nil {
		l = tls.NewListener(l, tl.config)
		run.TLS, run.Cert, run.Fingerprint = true, tl.from, tl.fingerprint
		st.Fingerprint = tl.fingerprint
	}
	st.URL = scheme + "://" + run.Listen + mcp.HTTPPath
	log.Printf("mcp http: listening on %s, token in %s", st.URL, from)
	if tl != nil {
		log.Printf("mcp http: TLS certificate %s, SHA-256 %s", tl.from, tl.fingerprint)
	}
	mcpHTTP.Store(st)
	writeMCPRun(a.dataDir, run)
	defer os.Remove(mcpRunFile(a.dataDir))
	if err := a.mcpHTTPServer().AcceptHTTP(ctx, l, token); err != nil && ctx.Err() == nil {
		fail(err)
	}
}

// mcpHTTPServer is mcpServer for sessions over HTTP, each marked remote.
func (a *App) mcpHTTPServer() *mcp.Server {
	srv := a.mcpServer()
	prev := srv.OnInit
	srv.OnInit = func(s *mcp.Session, params json.RawMessage) {
		s.Set(remoteKey, true)
		if prev != nil {
			prev(s, params)
		}
	}
	return srv
}

func writeMCPRun(dataDir string, run mcpRun) {
	b, _ := json.MarshalIndent(run, "", "  ")
	if err := os.WriteFile(mcpRunFile(dataDir), append(b, '\n'), 0o600); err != nil {
		log.Println("mcp http:", err)
	}
}

func readMCPRun(dataDir string) (mcpRun, bool) {
	var run mcpRun
	b, err := os.ReadFile(mcpRunFile(dataDir))
	if err != nil || json.Unmarshal(b, &run) != nil || run.Listen == "" {
		return mcpRun{}, false
	}
	return run, true
}

// mcpEndpoint is the endpoint as a client is told about it: where it
// listens, and its TLS (nil for plain HTTP).
type mcpEndpoint struct {
	listen string
	tls    *mcpTLS
}

// mcpEndpointNow is the endpoint the running bowel serves, or else the one
// this command's environment would have it serve ("" listen when that is
// none). A certificate it needs is made, as the token is, so that it is
// the one bowel will serve.
func mcpEndpointNow(dataDir string, getenv func(string) string) (mcpEndpoint, error) {
	if run, ok := readMCPRun(dataDir); ok {
		ep := mcpEndpoint{listen: run.Listen}
		if run.TLS {
			b, err := os.ReadFile(run.Cert)
			if err != nil {
				return ep, err
			}
			blk, _ := pem.Decode(certBlocks(b))
			if blk == nil {
				return ep, fmt.Errorf("%s holds no certificate", run.Cert)
			}
			ep.tls = &mcpTLS{cert: certBlocks(b), fingerprint: certFingerprint(blk.Bytes), from: run.Cert, selfSigned: run.Cert == mcpTLSFile(dataDir)}
		}
		return ep, nil
	}
	ep := mcpEndpoint{listen: mcpHTTPAddr(getenv)}
	if ep.listen == "" {
		return ep, nil
	}
	var err error
	ep.tls, err = mcpHTTPTLS(dataDir, getenv, true)
	return ep, err
}

// certBlocks are the certificates in a PEM file, without its key.
func certBlocks(b []byte) []byte {
	var out []byte
	for {
		var blk *pem.Block
		blk, b = pem.Decode(b)
		if blk == nil {
			return out
		}
		if blk.Type == "CERTIFICATE" {
			out = append(out, pem.EncodeToMemory(blk)...)
		}
	}
}

// runMCP is `bowel mcp`: the stdio bridge, or with "token", how an agent
// elsewhere reaches the HTTP endpoint, or with "cert", the certificate it
// serves.
func runMCP(args []string) error {
	if len(args) == 0 {
		return runMCPBridge()
	}
	if args[0] != "token" && args[0] != "cert" {
		return fmt.Errorf(i18n.T("未知的 mcp 子命令 %s；可以用 bowel mcp、bowel mcp token 或 bowel mcp cert", "Unknown mcp subcommand %s; use bowel mcp, bowel mcp token or bowel mcp cert"), args[0])
	}
	data, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return err
	}
	if args[0] == "cert" {
		return printMCPCert(os.Stdout, data, os.Getenv)
	}
	opts, err := mcpTokenArgs(args[1:])
	if err != nil {
		return err
	}
	return printMCPToken(os.Stdout, os.Stderr, data, os.Getenv, opts)
}

// mcpTokenOpts are `bowel mcp token`'s flags: -q prints the token alone,
// --name names the server in the snippets.
type mcpTokenOpts struct {
	quiet bool
	name  string
}

var mcpNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func mcpTokenArgs(args []string) (mcpTokenOpts, error) {
	o := mcpTokenOpts{name: "bowel"}
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "-q" || a == "--quiet":
			o.quiet = true
		case a == "--name" && i+1 < len(args):
			i++
			o.name = args[i]
		case strings.HasPrefix(a, "--name="):
			o.name = strings.TrimPrefix(a, "--name=")
		default:
			return o, fmt.Errorf(i18n.T("bowel mcp token 不认识 %s；可以用 -q 和 --name <名字>", "bowel mcp token doesn't take %s; it takes -q and --name <name>"), a)
		}
	}
	if !mcpNameRe.MatchString(o.name) {
		return o, fmt.Errorf(i18n.T("服务器名 %q 只能用字母、数字、- 和 _", "The server name %q may only have letters, digits, - and _"), o.name)
	}
	return o, nil
}

// printMCPToken prints the endpoint's token and how clients use it. It
// reads the data folder itself, so it works with the daemon stopped; the
// token it makes is the one the daemon will take.
func printMCPToken(out, errOut io.Writer, dataDir string, getenv func(string) string, o mcpTokenOpts) error {
	ep, epErr := mcpEndpointNow(dataDir, getenv)
	token, from, err := mcpHTTPToken(dataDir, getenv, ep.listen != "")
	if ep.listen == "" {
		fmt.Fprintf(errOut, i18n.T("MCP 的 HTTP 服务没有开启：启动 bowel 时设置 %s=:%s 来开启。\n", "MCP over HTTP is off: start bowel with %s=:%s to turn it on.\n"), mcpHTTPEnv, mcpHTTPPort)
		if err != nil {
			return errors.New(i18n.T("还没有令牌", "There is no token yet"))
		}
	} else if err != nil {
		return err
	} else if epErr != nil {
		return epErr
	}
	if o.quiet {
		fmt.Fprintln(out, token)
		return nil
	}
	url := mcpClientURL(ep.listen, ep.tls != nil)
	b, _ := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{o.name: map[string]any{
		"type": "http", "url": url, "headers": map[string]string{"Authorization": "Bearer " + token}}}}, "", "  ")
	fmt.Fprintf(out, "%s\n\n%s\n  %s\n\n%s\n%s\n\n",
		token,
		i18n.T("Claude Code：", "Claude Code:"),
		fmt.Sprintf(`claude mcp add --transport http %s %s --header "Authorization: Bearer %s"`, o.name, url, token),
		i18n.T("其他客户端（MCP 配置 JSON）：", "Other clients (MCP config JSON):"),
		b)
	if ep.tls == nil {
		fmt.Fprintln(out, fmt.Sprintf(i18n.T("令牌来自 %s。从别的机器连接时，把 localhost 换成这台机器的地址；令牌以明文经过网络，跨网络请放在 TLS 或 VPN 后面（%s=on 开启 TLS）。", "The token is from %s. From another machine, put this machine's address in place of localhost; the token crosses the network in the clear, so across networks keep it behind TLS or a VPN (%s=on turns TLS on)."), from, mcpTLSEnv))
		return nil
	}
	kind := i18n.T("自签名证书", "self-signed certificate")
	if !ep.tls.selfSigned {
		kind = i18n.T("证书", "certificate")
	}
	fmt.Fprintf(out, "TLS: %s %s\n  SHA-256 %s\n\n", kind, ep.tls.from, ep.tls.fingerprint)
	if ep.tls.selfSigned {
		_, port, err := net.SplitHostPort(ep.listen)
		if err != nil {
			port = mcpHTTPPort
		}
		fmt.Fprintf(out, i18n.T(
			"客户端要信任这张证书：在这台机器上执行 bowel mcp cert > %[1]s.pem，把文件拷到 agent 所在的机器，Claude Code 用 NODE_EXTRA_CA_CERTS=%[1]s.pem 启动（有几台 bowel 就把几个证书拼进同一个文件）。第一次连接前核对指纹：\n  openssl s_client -connect <地址>:%[2]s </dev/null 2>/dev/null | openssl x509 -noout -fingerprint -sha256\n",
			"Clients must trust this certificate: run bowel mcp cert > %[1]s.pem on this machine, copy the file to the agent's, and start Claude Code with NODE_EXTRA_CA_CERTS=%[1]s.pem (with several bowels, put all their certificates in that one file). Check the fingerprint before the first connection:\n  openssl s_client -connect <address>:%[2]s </dev/null 2>/dev/null | openssl x509 -noout -fingerprint -sha256\n"), o.name, port)
		fmt.Fprintf(out, i18n.T("从别的机器连接时，把 localhost 换成证书里有的名字或地址（%s 可以加）。令牌来自 %s。\n", "From another machine, put a name or address the certificate is made for in place of localhost (%s adds some). The token is from %s.\n"), mcpTLSHostsEnv, from)
		return nil
	}
	fmt.Fprintf(out, i18n.T("从别的机器连接时，把 localhost 换成证书上的名字。令牌来自 %s。\n", "From another machine, put the certificate's name in place of localhost. The token is from %s.\n"), from)
	return nil
}

// printMCPCert prints the certificate the endpoint serves, for a client to
// trust.
func printMCPCert(out io.Writer, dataDir string, getenv func(string) string) error {
	ep, err := mcpEndpointNow(dataDir, getenv)
	if err != nil {
		return err
	}
	if ep.tls == nil {
		return fmt.Errorf(i18n.T("MCP 的 HTTP 服务没有用 TLS：启动 bowel 时设置 %s=:%s 和 %s=on", "MCP over HTTP has no TLS: start bowel with %s=:%s and %s=on"), mcpHTTPEnv, mcpHTTPPort, mcpTLSEnv)
	}
	_, err = out.Write(ep.tls.cert)
	return err
}

// mcpClientURL is the URL a client on this machine (or the one publishing
// the container's port) uses for the listen address addr.
func mcpClientURL(addr string, tls bool) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		host, port = "", mcpHTTPPort
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "localhost"
	}
	scheme := "http"
	if tls {
		scheme = "https"
	}
	return scheme + "://" + net.JoinHostPort(host, port) + mcp.HTTPPath
}
