# bowel

Formerly Moho. The Go module is still `github.com/oboo/moho` and the bundle identifier `dev.moho.app`, which keeps macOS's camera and screen-recording permissions. On first launch the data in `~/Library/Application Support/Moho` moves to `…/bowel`.

A macOS client for the [mihomo](https://github.com/MetaCubeX/mihomo) proxy core, built with [MyGo](https://github.com/egoist/mygo)'s native UI: Go only, drawn on the GPU, no webview.

The product logic is in [docs/product-ia.md](docs/product-ia.md); the approved interaction prototype is [prototype/index.html](prototype/index.html).

## Run

Needs Go 1.27.3+ (`GOTOOLCHAIN=auto` fetches it). No Node or Bun.

```bash
./scripts/fetch-core.sh     # mihomo + GeoSite/GeoIP data into resources/
go tool mygo dev            # the app, rebuilt on every change
go tool mygo build          # bowel.app and a .dmg
CGO_ENABLED=0 go test ./... # includes tests that start the real core
MOHO_SHOTS=/tmp/shots CGO_ENABLED=0 go test -short -run 'Home|Activity|Provider|Panel' .
                            # renders the interface off screen to PNGs
```

## How it works

- **Providers only supply nodes.** A subscription URL, pasted node links (`vmess://`, `trojan://`, `hysteria2://`, …) or a YAML file each become a mihomo `type: file` proxy-provider. bowel downloads subscriptions itself, to read their usage and expiry. A hidden per-provider suffix keeps node names unique across providers.
- **Routes come from bowel's template** (`internal/routes`): 默认, AI 服务, 流媒体, 开发 and 直连, and the rules that recognise each. Every node can serve every route. Each proxied route has two groups: a selector, and an automatic url-test group the selector can point at ("自动" = fastest + failover).
- **The core runs as a sidecar process** (`internal/core`), controlled over a unix socket. bowel falls back to TCP with a secret when the socket path is too long. Config changes reload the core without restarting it.
- **The interface is native.** The window and the menu bar panel are MyGo `ui` views over two values: a `Snapshot`, rebuilt whenever anything changes, and a `Tick`, every second. The 今日 board animates its cells' boxes: the expanded cell grows in place and the others are squeezed into summaries chosen to fit. Every view is laid out at its cell's final size, so nothing reflows on the way. Views open and fold like drawers, and shared pieces fly between views (`Match`, with text morphing).
- **Status is measured, not assumed** (§2 of the IA). bowel checks every 2 s that the system proxy still points at it (`scutil --proxy`). The core's state, failing nodes and automatic switches feed the one light: 正常 / 降级 / 异常 / 未接管.
- **Exit locations** (`internal/locate`, `internal/geo`). A node's place is guessed from its name. When it matters (the node a route uses, or one you pick), bowel measures it instead: it requests Cloudflare's `/cdn-cgi/trace` through the node, via a mihomo listener bound to a probe group. The UI labels each place 实测 / 按名称 / 未知.
- **Monitoring** (`internal/monitor`). bowel polls connections every second while the window or the menu bar panel is open (every 5 s otherwise) to get rates per route and per app, new connections, and closed ones. It parses failed dials from the core's log, and keeps today's and this month's totals. On macOS 27 the kernel hides its socket list from a process whose parent is an ad-hoc signed app, so the core, bowel's child, cannot tell which app opened a connection. bowel reads the list itself (`internal/procinfo`) and names those connections. Under `mygo dev` bowel is itself such a child, so the names show as 未知 in development; an app opened from Finder has them.
- **TUN.** The first time you turn it on, macOS asks for an administrator's authorisation. bowel then copies the core to `~/Library/Application Support/bowel/bin/` and makes it setuid root (`internal/tun`), the approach early Clash Verge used. A launchd helper would be the cleaner long-term design.

## Layout

```
main.go, app.go           wiring: lifecycle, actions, the snapshot and tick
model.go                   the view model: Snapshot, Tick, Conn, …
ui.go                      the window: palette, title bar, pages, shared parts
ui_home.go, ui_map.go      今日: the morphing board and the exit map
ui_activity.go             活动: connections and logs
ui_providers.go            Provider: add, list, nodes
ui_settings.go             设置: the core's options, in a window of their own (sidebar + pane)
ui_panel.go, tray.go       the menu bar icon, its panel and its menu
debug.go                   a script hook for development builds (MOHO_DEBUG_DIR)
internal/core              mihomo process, runtime config, the core's options (Options)
internal/mihomo            controller client
internal/provider          providers: detect, download, store
internal/routes            the routing template
internal/monitor           connections, rates, events, logs, totals
internal/procinfo          which app owns a local socket (macOS)
internal/locate, geo       exit locations
internal/sysproxy          set and check the macOS system proxy
internal/tun               setuid install for TUN
internal/settings          settings.json
prototype/                 the interaction prototype and its history
```

mihomo is GPL-3.0; bowel runs it as a separate program and does not link it.
