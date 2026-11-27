package main

// Adding and removing a provider, for agents on the socket or over HTTP as
// for the command line (`bowel providers add|remove` calls them on
// cli.sock). A subscription's URL holds its token, so it is never shown,
// logged or answered whole: the dialog that asks the user, an error from
// its download and the provider in the answer all name it by its host.

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/oboo/moho/internal/i18n"
	"github.com/oboo/moho/internal/mcp"
	"github.com/oboo/moho/internal/provider"
)

// providerTools are the tools that add and remove providers.
func (a *App) providerTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name: "provider_add", Title: "Add a provider",
			Description: "Adds a provider: a subscription URL or a node link (source), or a file on the machine bowel runs on (file, an absolute path; not over HTTP, where that machine's files are not yours). A subscription link is the user's: ask them for it and never invent one. Its URL holds a token: the answer and any error name it by its host only, and so should you.",
			Input:       obj(map[string]any{"source": str("A subscription URL or a node link."), "file": str("An absolute path.")}),
			Run: func(ctx context.Context, s *mcp.Session, raw json.RawMessage) (any, error) {
				var p struct{ Source, File string }
				if err := mcp.Args(raw, &p); err != nil {
					return nil, err
				}
				if (p.Source == "") == (p.File == "") {
					return nil, errors.New("give source or file")
				}
				if p.File != "" && isRemote(s) {
					return nil, errors.New(i18n.T("通过 HTTP 连接时不能添加 bowel 所在机器上的文件：请用订阅链接或节点链接（source）", "Over HTTP, a file on the machine bowel runs on can't be added: give a subscription URL or a node link (source)"))
				}
				detail := p.File
				if p.Source != "" {
					detail = sourceHost(p.Source)
				}
				var v ProviderView
				add := func() error {
					done := a.begin("add")
					defer done()
					var pv provider.Provider
					var err error
					if p.File != "" {
						pv, err = a.providers.AddFile(p.File)
					} else {
						pv, err = a.providers.Add(ctx, a.http, p.Source)
					}
					if err != nil {
						return err
					}
					v, err = a.afterProviderAdded(pv)
					return err
				}
				err := a.change(ctx, s, words{"添加 Provider", "Add Provider"}, detail, func() error {
					// Redacted before the list of agents' changes keeps it.
					if err := add(); err != nil && p.Source != "" {
						return errors.New(redact(err, p.Source))
					} else if err != nil {
						return err
					}
					return nil
				})
				if err != nil {
					return nil, err
				}
				return map[string]any{"provider": v}, nil
			},
		},
		{
			Name: "provider_remove", Title: "Remove a provider", Destructive: true,
			Description: "Removes a provider, by id or name (list_providers), and its nodes with it: a route that used one of them moves to another node, and with no provider left there is no node to use.",
			Input:       obj(map[string]any{"id": str("A provider's id or name.")}, "id"),
			Run: func(ctx context.Context, s *mcp.Session, raw json.RawMessage) (any, error) {
				var p struct{ ID string }
				if err := mcp.Args(raw, &p); err != nil {
					return nil, err
				}
				pv, err := a.findProvider(p.ID)
				if err != nil {
					return nil, err
				}
				err = a.change(ctx, s, words{"删除 Provider", "Remove Provider"}, pv.Name, func() error { return a.deleteProvider(pv.ID) })
				if err != nil {
					return nil, err
				}
				return map[string]any{"removed": a.providerView(pv)}, nil
			},
		},
	}
}
