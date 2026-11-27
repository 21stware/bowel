package main

// The command line's side in the app. `bowel status`, `bowel mode global`
// and the rest (internal/cli) are a thin client of the running bowel, the
// app or `bowel daemon`: they call the same tools agents do (app_mcp.go's
// mcpServer), and a few only the command line has, over a second socket
// in the data folder, cli.sock, in the protocol of the first.
//
// Who is asking is what the two sockets tell apart. On mcp.sock it is an
// agent, and a change follows the user's policy. On cli.sock it is the
// user, at a shell, and nothing is asked: unless the command says an
// agent ran it (internal/cli sees the agent's environment), and then the
// policy holds as on mcp.sock. That is a guard rail, not a wall: a program
// of this user that wants round it can also write settings.json.
//
// The environment does not always come along: `docker exec` runs a command
// with the container's, and `ssh` with the server's, so an agent that
// reaches bowel that way reads as the user. The policy is what keeps that
// from mattering most, so loosening it takes more than a command on
// cli.sock: the person typing the confirmation at a terminal, which the
// command says with update_settings' confirmed (policyChange).
//
// In a container every command comes through `docker exec`, so there the
// user is who has a terminal: a command whose stdin is no person's
// terminal (internal/cli's atTerminal: none, or the 0×0 one of an agent's
// `docker exec -t`) is unattended, and its changes follow the policy as an
// agent's do. The user's `docker exec -it`, and so `bowel tui`, are still
// the user's.

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"github.com/oboo/moho/internal/backup"
	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/mcp"
	"github.com/oboo/moho/internal/provider"
	"github.com/oboo/moho/internal/routes"
)

// cliSocket is where the app serves the command line, beside mcp.sock
// (app_socket.go).
func cliSocket(dataDir string) string { return cliSocketIn(socketDir(dataDir), dataDir) }

// olderCLISocket is where bowel 0.4.9 and before served it, "" when that
// is cliSocket.
func olderCLISocket(dataDir string) string {
	return olderSocket(cliSocket(dataDir), cliSocketIn(dataDir, dataDir))
}

// cliSocketIn is cli.sock in the folder dir, or, when that path is too
// long for a unix socket, in the temporary folder.
func cliSocketIn(dir, dataDir string) string {
	p := filepath.Join(dir, "cli.sock")
	if len(p) < 100 {
		return p
	}
	// Told apart by the folder, so that two long ones don't share a socket.
	sum := sha256.Sum256([]byte(dataDir))
	return filepath.Join(os.TempDir(), fmt.Sprintf("bowel-%d-%x-cli.sock", os.Getuid(), sum[:4]))
}

// isLocal says the session is the user's own command, not an agent's.
func isLocal(s *mcp.Session) bool { return s != nil && s.Get("local") == true }

// cliSessionIs is who a session on cli.sock is, from what its initialize
// says: an agent by its environment, unattended (a command in a container
// with no person's terminal, which may be an agent's), or the user. A
// command older than terminal says none, and in a container is unattended.
func cliSessionIs(agent string, terminal *bool, container bool) string {
	switch {
	case agent != "":
		return "agent"
	case container && (terminal == nil || !*terminal):
		return "unattended"
	}
	return "user"
}

// unattendedNote is err with why an unattended command (cliSessionIs) is
// refused what the user's would not be.
func unattendedNote(s *mcp.Session, err error) error {
	if err == nil || s == nil || s.Get("unattended") != true {
		return err
	}
	return fmt.Errorf("%s %w", i18n.T("没有终端的 `docker exec` 命令算作 agent 的命令，因为 bowel 分不清是谁运行的。用户可以在自己的终端里用 `docker exec -it` 运行；agent 不行。",
		"A command through `docker exec` with no terminal counts as an agent's, since bowel can't tell who ran it. The user can run it at their own terminal with `docker exec -it`; an agent can't."), err)
}

// cliServer is mcpServer with the command line's own tools, for sessions
// that are the user's unless they say an agent runs them.
func (a *App) cliServer() *mcp.Server {
	srv := a.mcpServer()
	srv.Name, srv.Title, srv.Instructions = "bowel-cli", "bowel CLI", ""
	for i, t := range srv.Tools {
		if t.Name == "list_connections" || t.Name == "get_logs" {
			run := t.Run
			srv.Tools[i].Run = func(ctx context.Context, s *mcp.Session, raw json.RawMessage) (any, error) {
				a.watched()
				return run(ctx, s, raw)
			}
		}
	}
	srv.Tools = append(srv.Tools, a.cliTools()...)
	srv.Tools = append(srv.Tools, a.healthTool()) // app_container.go
	srv.OnInit = func(s *mcp.Session, params json.RawMessage) {
		var p struct {
			Agent    string `json:"agent"`
			Terminal *bool  `json:"terminal"`
		}
		_ = json.Unmarshal(params, &p)
		switch cliSessionIs(p.Agent, p.Terminal, inContainer()) {
		case "user":
			s.Set("local", true)
		case "unattended":
			s.Set("unattended", true)
		}
	}
	return srv
}

// watched notes that a command reads the connections or the logs now, so
// that they are polled every second while it does (pollEvery).
func (a *App) watched() {
	a.watchedAt.Store(time.Now().Unix())
	a.mon.Wake()
}

// findProvider is the provider s names: its id, or its name when one has it.
func (a *App) findProvider(s string) (provider.Provider, error) {
	if p, ok := a.providers.Get(s); ok {
		return p, nil
	}
	var hits []provider.Provider
	for _, p := range a.providers.List() {
		if strings.EqualFold(p.Name, s) {
			hits = append(hits, p)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return provider.Provider{}, fmt.Errorf(i18n.T("没有这个 Provider: %s", "No such Provider: %s"), s)
	}
	return provider.Provider{}, fmt.Errorf(i18n.T("有多个 Provider 叫 %s，请用它的 id", "Several Providers are called %s; use the id"), s)
}

// cliTools are the tools only the command line has. Their changes go
// through change, as the agents' do.
func (a *App) cliTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name: "provider_enable", Title: "Turn a provider on or off",
			Description: "Turns a provider on or off, by id or name: off, its nodes are not used.",
			Input:       obj(map[string]any{"id": str("A provider's id or name."), "enabled": map[string]any{"type": "boolean"}}, "id", "enabled"),
			Run: func(ctx context.Context, s *mcp.Session, raw json.RawMessage) (any, error) {
				var p struct {
					ID      string
					Enabled bool
				}
				if err := mcp.Args(raw, &p); err != nil {
					return nil, err
				}
				pv, err := a.findProvider(p.ID)
				if err != nil {
					return nil, err
				}
				what := words{"停用 Provider", "Turn a Provider off"}
				if p.Enabled {
					what = words{"启用 Provider", "Turn a Provider on"}
				}
				if err := a.change(ctx, s, what, pv.Name, func() error { return a.setProviderEnabled(pv.ID, p.Enabled) }); err != nil {
					return nil, err
				}
				return map[string]any{"providers": a.snapshot().Providers}, nil
			},
		},
		{
			Name: "list_nodes", Title: "Nodes on a route",
			Description: "The nodes with their delays on a route's test target, the node the route uses, and the providers' names. With test, the delays are measured first. refusedBySite marks a failed test the site answered by turning the node away.",
			Input:       obj(map[string]any{"route": str("A route's id."), "test": map[string]any{"type": "boolean"}}),
			Run: func(ctx context.Context, s *mcp.Session, raw json.RawMessage) (any, error) {
				var p struct {
					Route string
					Test  bool
				}
				if err := mcp.Args(raw, &p); err != nil {
					return nil, err
				}
				if p.Route == "" {
					p.Route = routes.Default
				}
				r, err := a.findRoute(p.Route)
				if err != nil {
					return nil, err
				}
				out := map[string]any{"route": r.ID}
				if p.Test {
					if r.Direct {
						return nil, errors.New(i18n.T("这条路由不使用节点", "This route uses no node"))
					}
					if err := a.testRoute(ctx, r.ID); err != nil {
						out["error"] = err.Error()
					}
				}
				snap := a.snapshot()
				for _, rv := range snap.Routes {
					if rv.ID == r.ID {
						out["current"], out["auto"] = rv.Node, rv.Auto
					}
				}
				names := map[string]string{}
				for _, pv := range snap.Providers {
					names[pv.ID] = pv.Name
				}
				out["nodes"], out["providers"] = mcpNodes(snap, r.ID), names
				return out, nil
			},
		},
		{
			Name: "tail_logs", Title: "Follow the core's logs", ReadOnly: true,
			Description: "The core's log lines after the one numbered after, oldest first, and the number to ask with next. With after 0, the last limit lines.",
			Input: obj(map[string]any{
				"after": map[string]any{"type": "integer", "minimum": 0},
				"level": str("Only this level and worse.", "debug", "info", "warning", "error"),
				"limit": map[string]any{"type": "integer", "minimum": 0, "maximum": 1000},
			}),
			Run: func(ctx context.Context, s *mcp.Session, raw json.RawMessage) (any, error) {
				var p struct {
					After int64
					Level string
					Limit int
				}
				if err := mcp.Args(raw, &p); err != nil {
					return nil, err
				}
				a.watched()
				all := a.logs(p.After)
				last := p.After
				if n := len(all); n > 0 {
					last = all[n-1].Seq
				}
				rank := map[string]int{"debug": 0, "info": 1, "warning": 2, "error": 3}
				out := []LogLine{}
				for _, l := range all {
					if p.Level == "" || rank[l.Level] >= rank[p.Level] {
						out = append(out, l)
					}
				}
				if p.After == 0 && len(out) > p.Limit {
					out = out[len(out)-p.Limit:]
				}
				return map[string]any{"logs": out, "last": last}, nil
			},
		},
		{
			Name: "config_export", Title: "Export the configuration",
			Description: "Writes the settings, providers and rule sets to a file (path, absolute), sealed with password if one is given. Only for the user's own command.",
			Input:       obj(map[string]any{"path": str("An absolute path."), "password": str("Seals the file; empty for none.")}),
			Run: func(ctx context.Context, s *mcp.Session, raw json.RawMessage) (any, error) {
				var p struct{ Path, Password string }
				if err := mcp.Args(raw, &p); err != nil {
					return nil, err
				}
				if err := userOnly(s, p.Path); err != nil {
					return nil, err
				}
				if err := a.exportConfig(p.Path, p.Password); err != nil {
					return nil, err
				}
				return map[string]any{"path": p.Path, "encrypted": p.Password != "", "providers": len(a.providers.List())}, nil
			},
		},
		{
			Name: "config_import", Title: "Import a configuration", Destructive: true,
			Description: "Replaces the settings, providers and rule sets with those of an exported file (path, absolute), keeping this machine's own settings; or, with undo, puts back what the last import replaced. Only for the user's own command.",
			Input:       obj(map[string]any{"path": str("An absolute path."), "password": str("The file's password, if it has one."), "undo": map[string]any{"type": "boolean", "description": "Undo the last import instead."}}),
			Run: func(ctx context.Context, s *mcp.Session, raw json.RawMessage) (any, error) {
				var p struct {
					Path, Password string
					Undo           bool
				}
				if err := mcp.Args(raw, &p); err != nil {
					return nil, err
				}
				if p.Undo {
					p.Path = a.undoImportPath()
				}
				if err := userOnly(s, p.Path); err != nil {
					return nil, err
				}
				var err error
				if p.Undo {
					err = a.undoImport()
				} else {
					err = a.importConfig(p.Path, p.Password)
				}
				if backup.IsPassword(err) {
					// Said apart from other failures: the command asks and
					// tries again.
					return map[string]any{"imported": false, "needPassword": true, "message": err.Error()}, nil
				}
				if err != nil {
					return nil, err
				}
				return map[string]any{"imported": true, "undone": p.Undo, "providers": len(a.providers.List())}, nil
			},
		},
		{
			Name: "stop", Title: "Quit bowel", Destructive: true,
			Description: "Quits bowel: the system proxy and the gateway go back as they were, and the core ends.",
			Run: func(ctx context.Context, s *mcp.Session, _ json.RawMessage) (any, error) {
				err := a.change(ctx, s, words{"退出 bowel", "Quit bowel"}, "bowel", func() error {
					// After the answer has gone: quitting closes this socket.
					time.AfterFunc(200*time.Millisecond, a.quit)
					return nil
				})
				if err != nil {
					return nil, err
				}
				return map[string]any{"stopping": true, "headless": a.headless}, nil
			},
		},
	}
}

// userOnly refuses what only the user may do with a file: an export holds
// the subscription links, and an import replaces everything, so neither is
// an agent's to run, whatever the policy.
func userOnly(s *mcp.Session, path string) error {
	if !isLocal(s) {
		return unattendedNote(s, errors.New(i18n.T("导出和导入配置只能由用户自己操作", "Only the user can export or import the configuration")))
	}
	if !filepath.IsAbs(path) {
		return errors.New("path must be absolute")
	}
	return nil
}

// quit ends bowel as its menu's 退出 does, or ends a run without a window.
func (a *App) quit() {
	if !a.headless {
		mygo.App.Quit()
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stop != nil {
		select {
		case <-a.stop:
		default:
			close(a.stop)
		}
	}
}

var cliStop context.CancelFunc

// startCLI serves the command line for the app's life; stopCLI ends that.
func (a *App) startCLI() {
	ctx, cancel := context.WithCancel(context.Background())
	cliStop = cancel
	dropOlderSocket(olderCLISocket(a.dataDir))
	l, err := mcp.Listen(cliSocket(a.dataDir))
	if err != nil {
		log.Println("cli:", err)
		return
	}
	go a.cliServer().Accept(ctx, l)
}

func stopCLI() {
	if cliStop != nil {
		cliStop()
	}
}
