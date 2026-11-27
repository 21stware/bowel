# Docker

The image is `bowel daemon` with the mihomo core beside it, for servers, NAS boxes, routers and development machines with no screen. It has no window and no web page: `bowel tui` in the container is the interface, the command line does the rest, and agents reach it over MCP. It is built for amd64, arm64 and armv7, is about 40 MB to download, and has no shell or package manager inside.

```bash
docker run -d --name bowel --restart unless-stopped \
  -p 7899:7899 -v bowel:/data \
  -e BOWEL_SUBSCRIPTION='https://example.com/your-subscription' \
  ghcr.io/21stware/bowel
```

Then point programs at `http://<host>:7899` (HTTP and SOCKS5 share the port), and open the interface:

```bash
docker exec -it bowel bowel tui
```

`docker exec bowel bowel status`, `bowel nodes`, `bowel route use ai <node>`, `bowel logs -f` and every other command work the same way: see [cli.md](cli.md). A shell alias saves typing: `alias bowel='docker exec -it bowel bowel'`.

## Images

| Tag | What |
| --- | --- |
| `ghcr.io/21stware/bowel:latest` | the latest release |
| `ghcr.io/21stware/bowel:0.4` | the latest 0.4.x |
| `ghcr.io/21stware/bowel:0.4.8` | one release |

The same image is on Docker Hub as `21stware/bowel` (`docker pull 21stware/bowel`), which is where Synology's Container Manager, Unraid and other NAS interfaces search; every command here takes either name.

The amd64 image carries mihomo's `v1` build, which runs on every x86-64 CPU, including the Atoms and Celerons of many NAS boxes. Each release pushes the image after the release itself is published; the image does not update itself, so pull a new tag to update (`docker compose pull && docker compose up -d`). The data volume carries over.

## Compose

[docker/compose.yaml](docker/compose.yaml) is the usual setup, a proxy for the LAN:

```yaml
services:
  bowel:
    image: ghcr.io/21stware/bowel
    container_name: bowel
    restart: unless-stopped
    ports:
      - "7899:7899"
      - "127.0.0.1:7900:7900" # MCP for agents on this machine; drop 127.0.0.1: to open it to the LAN
    volumes:
      - bowel:/data
    environment:
      BOWEL_SUBSCRIPTION: https://example.com/your-subscription
      TZ: Asia/Shanghai
volumes:
  bowel:
```

[docker/compose.tun.yaml](docker/compose.tun.yaml) runs TUN on the host's network, for the host's own traffic or for gateway mode (below).

## Settings

The container is configured by its environment. The first start seeds the settings for a container: the proxy open to private addresses (10/8, 172.16/12, 192.168/16, 127/8, fc00::/7), no system proxy, no process lookup, and the language of `LANG` (English without one). A `settings.json` already in `/data` is never touched by that. The variables below apply at every start and are saved:

| Variable | What |
| --- | --- |
| `BOWEL_SUBSCRIPTION` | Subscription URLs or node links, one per line or separated by commas. Each that no Provider has yet is added, and retried a few times if the download fails. The log names the Provider's ID and host, never the URL. |
| `BOWEL_MIXED_PORT` | The proxy port inside the container (7899). |
| `BOWEL_ALLOW_LAN` | `false` to serve only the container itself, as with TUN on the host's network. |
| `BOWEL_LAN_ALLOWED_IPS` | Who may use the port, as CIDRs separated by commas or spaces. With `-p`, Docker hands the clients' real addresses to the container, so on a server with a public address, clients on the internet are refused until they are listed here; set `authentication` too (`bowel settings set authentication '["user:password"]'`). |
| `BOWEL_MCP_HTTP` | Where MCP over HTTP listens (`:7900`); empty turns it off. See [Agents](#agents-mcp). |
| `BOWEL_MCP_TOKEN` | The MCP token, instead of the one made on the first start. |
| `TZ` | The time zone of the logs and the statistics' days. |

Everything else is a setting, changed with `bowel settings set <key> <value>` (`bowel settings list` shows them), in the TUI's settings page, or through an agent: `bowel settings set language zh` for Chinese, for one. Settings live in `/data/settings.json`, beside the Providers, the rules and the statistics; keep `/data` on a volume. The container runs as root; `--user` runs it as another user, who needs to own `/data`.

## TUN and gateway mode

The proxy port needs nothing special. TUN, which takes all traffic of the network it is on, needs the TUN device and the right to configure the network:

```bash
docker run -d --name bowel --restart unless-stopped \
  --network host --cap-add NET_ADMIN --device /dev/net/tun \
  -v bowel:/data ghcr.io/21stware/bowel
docker exec bowel bowel tun on
```

On the host's network, TUN takes the host's own traffic, and `bowel gateway on` then makes the host a router for the LAN: devices that use its address as their gateway and DNS go through bowel. Gateway mode needs the host's network (it serves the LAN on a physical interface, not on Docker's bridge) and IP forwarding, which Docker turns on on the host already; where it is off, turn it on on the host (`sysctl -w net.ipv4.ip_forward=1`), since `/proc/sys` is read-only in a container. On Docker's bridge network instead, TUN takes the traffic of the container and of containers that share its network (`--network container:bowel`), which is a way to send one service through bowel. Without the device or the capability, `bowel tun on` says which one is missing. Docker Desktop's host network is its VM's, so on macOS and Windows use the app instead.

## Agents (MCP)

The image serves MCP over HTTP on port 7900, behind a token that is made on the first start and kept in `/data/mcp-token`. Print it, with the lines that add bowel to an agent:

```bash
docker exec bowel bowel mcp token
```

```bash
claude mcp add --transport http bowel http://<host>:7900/mcp --header "Authorization: Bearer <token>"
```

Other clients take the same URL and header (`"type": "http"`, `"url"`, `"headers"`). `BOWEL_MCP_TOKEN` sets a token of your own (16 characters or more) instead; an empty `BOWEL_MCP_HTTP` turns the endpoint off. It is plain HTTP and the token travels in the clear: publish 7900 on `127.0.0.1`, a VPN or a private network only, or put it behind a TLS proxy that keeps the `Host` header.

There is no window to ask in, so under the default policy, "ask", an agent can read everything and change nothing. Let agents change things with `docker exec bowel bowel settings set mcpPolicy allow`. Agents on the same host can also run `docker exec -i bowel bowel mcp`, the stdio server, which needs no token.

## Health and logs

The image's health check is `bowel health`: healthy while the daemon answers, the core runs and its port accepts connections. `docker logs bowel` has the daemon's log; the core's is `docker exec bowel bowel logs -f`. `docker stop` stops it cleanly.

## Egress for other containers

Other containers, and machines on the LAN, can go out through bowel and pick their exit. There are three ways in:

- **The proxy port, 7899.** Traffic goes where the rules send it, as for any client.
- **A port per route group.** `BOWEL_EXTRA_PORTS: 7901=ai 7902=media` opens 7901 and 7902, which take HTTP and SOCKS5, UDP included. Everything that comes in by one of them leaves by its group, whatever the rules and the mode say. In a container every extra port is open as 7899 is: to the networks of `BOWEL_LAN_ALLOWED_IPS` (the private ones at first), and behind `authentication` when that is set. So `-p 7901:7901` reaches it from the LAN, and other containers reach it at `bowel:7901`.
- **TUN, for a container that shares bowel's network** (`network_mode: service:bowel`). Every connection it makes goes through bowel, including those of programs that ignore proxy variables, and the rules route them.

The exit lock is the kill switch. A locked group leaves by one of its nodes or not at all. It never goes direct: not when no node answers, not while the subscription is still downloading, and not when its nodes are gone. What it holds depends on the way in. Through a port of a locked group, it holds everything that comes in by that port. Under TUN, it holds the traffic the rules send to the locked groups. The default route can't be locked, and the direct route goes direct by design (domestic sites and the LAN). A container has no system proxy, so the lock holds the same way whether takeover is on or off.

The environment sets all of this at every start, so a compose file is enough:

| Variable | What |
| --- | --- |
| `BOWEL_EXTRA_PORTS` | The extra ports, as `7901=ai 7902=media` (port=route ID, separated by spaces or commas; `bowel routes` lists the IDs) or as the setting's JSON. `none` closes them. |
| `BOWEL_LOCK` | The route groups whose exit is locked, by ID (`ai dev`); the others are unlocked. If an entry names no group that can be locked (`default`, `direct` and `reject` can't be), the valid ones are still locked and nothing is unlocked, so a typo never loosens the lock. `none` unlocks them all. |
| `BOWEL_TUN` | `true` turns TUN on, as `bowel tun on` does. It needs `NET_ADMIN` and `/dev/net/tun`. |
| `BOWEL_SETTINGS` | Any other settings, as a JSON object by their keys (`{"sniff": true}`), checked as `bowel settings set` checks them. It is applied first, so the variables above win over it. It can't set `mcpPolicy`. |

Each is applied and saved on its own, so one the settings refuse (a port that is taken, say) is logged and leaves the others in place.

### An agent behind bowel

[docker/compose.agent.yaml](docker/compose.agent.yaml) runs an agent, a devcontainer or a CI runner on bowel's network, with TUN on and `ai` and `dev` locked:

```yaml
services:
  bowel:
    image: ghcr.io/21stware/bowel
    cap_add: [NET_ADMIN]
    devices: [/dev/net/tun:/dev/net/tun]
    dns: [1.1.1.1]
    volumes: [bowel:/data]
    environment:
      BOWEL_SUBSCRIPTION: https://example.com/your-subscription
      BOWEL_TUN: "true"
      BOWEL_LOCK: ai dev
      BOWEL_SETTINGS: '{"sniff": true}'
  agent:
    image: node:22-bookworm
    network_mode: service:bowel
    depends_on:
      bowel: {condition: service_healthy}
```

The agent's API calls (`ai`) and its GitHub, npm, PyPI and Docker Hub traffic (`dev`) leave by a node or are refused. `docker exec bowel bowel logs` shows each connection with the group and node it took.

The lock goes by the rules, so the rules have to see the names, and two lines see to that. `dns` makes Docker's resolver ask a server through TUN, where bowel answers. Without it the host's resolver answers, and bowel sees only addresses: an API call then matches no rule of `ai` and takes the default route. `sniff` reads the name from the traffic, for programs that resolve names by themselves. Traffic no rule matches takes the default route, which leaves by its node, or directly when it has none. For a container whose every connection must leave by a node, use a port of a locked group on an internal network, as in the next recipe.

A container on bowel's network shares its ports too: the agent reaches bowel's MCP at `http://127.0.0.1:7900/mcp`, with the token `docker exec bowel bowel mcp token` prints, and any extra port at `127.0.0.1:<port>`.

### A download client with no way out but bowel

[docker/compose.downloads.yaml](docker/compose.downloads.yaml) puts qBittorrent on an internal network whose only other member is bowel, so it has no route out and no DNS for outside names. Its proxy is bowel's port 7902, which leaves by `media`, locked:

```yaml
services:
  bowel:
    networks: [default, downloads]
    environment:
      BOWEL_EXTRA_PORTS: 7902=media
      BOWEL_LOCK: media
  qbittorrent:
    image: lscr.io/linuxserver/qbittorrent
    networks: [downloads]
  webui:
    image: caddy:2-alpine
    networks: [default, downloads]
    ports: ["8080:8080"]
    command: caddy reverse-proxy --from :8080 --to qbittorrent:8080
networks:
  downloads:
    internal: true
```

In qBittorrent at `http://<host>:8080` (the first password is in `docker logs qbittorrent`), set Tools → Options → Connection → Proxy server to SOCKS5, host `bowel`, port `7902`, with peer connections and hostname lookups through the proxy. Whatever qBittorrent sends around the proxy has nowhere to go. What it sends through the port leaves by `media`'s node or is refused, so a node that goes down, or a subscription that fails, stops the downloads instead of sending them from your own address.

An internal network can't publish ports, which is what the reverse proxy is for. qBittorrent wants the same port on both sides: to move it, change `WEBUI_PORT`, the mapping and caddy's two ports together. Any group works in place of `media`, and a group of its own, with its own node, is best. Many providers forbid BitTorrent, so check yours first.

### A port per container

[docker/compose.ports.yaml](docker/compose.ports.yaml) gives two containers two exits by the usual proxy variables: `assistant` uses `bowel:7901` (`ai`) and `downloader` uses `bowel:7902` (`media`). Set the variables in both cases, since curl reads `http_proxy` only in lower case; a program that reads neither goes out by itself, and the TUN recipe catches those. The ports are published too, so a machine on the LAN picks its exit the same way, with `http://<host>:7901`. With `BOWEL_LOCK: ai media`, a port whose group has no node refuses instead of going direct.

On a Mac or PC the app does the same: `bowel settings set extraPorts '[{"port":7901,"route":"ai","lan":true}]'` with Allow LAN on (see [cli.md](cli.md#a-port-per-route-group)).

## Building it yourself

```bash
docker build -t bowel .
docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v7 -t you/bowel --push .
```

Every stage that runs a command runs on the build machine and cross-compiles, so a multi-platform build needs no emulation.
