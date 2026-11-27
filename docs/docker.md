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
| `BOWEL_MCP_TLS` | `on` serves MCP over HTTPS with a self-signed certificate kept in `/data`. |
| `BOWEL_MCP_TLS_HOSTS` | The names and addresses clients reach it by, for that certificate. |
| `BOWEL_MCP_TLS_CERT`, `BOWEL_MCP_TLS_KEY` | A certificate and key of your own (PEM files), read again when they change. |
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

Other clients take the same URL and header (`"type": "http"`, `"url"`, `"headers"`). `BOWEL_MCP_TOKEN` sets a token of your own (16 characters or more) instead; an empty `BOWEL_MCP_HTTP` turns the endpoint off. Without TLS it is plain HTTP and the token travels in the clear: publish 7900 on `127.0.0.1`, a VPN or a private network only, or turn TLS on. `BOWEL_MCP_TLS=on` makes a self-signed certificate in `/data` (name the host clients use in `BOWEL_MCP_TLS_HOSTS`, as `nas.lan,192.168.1.5`); `docker exec bowel bowel mcp token` then prints its fingerprint and the `https://` lines, and `docker exec bowel bowel mcp cert` the certificate for the agent's machine to trust. `BOWEL_MCP_TLS_CERT` and `BOWEL_MCP_TLS_KEY` serve a certificate of your own instead. [mcp-http.md](mcp-http.md) has the clients' side, and how one agent drives several bowels.

There is no window to ask in, so under the default policy, "ask", an agent can read everything and change nothing. Let agents change things with `docker exec bowel bowel settings set mcpPolicy allow`. Agents on the same host can also run `docker exec -i bowel bowel mcp`, the stdio server, which needs no token.

## Health and logs

The image's health check is `bowel health`: healthy while the daemon answers, the core runs and its port accepts connections. `docker logs bowel` has the daemon's log; the core's is `docker exec bowel bowel logs -f`. `docker stop` stops it cleanly.

## Building it yourself

```bash
docker build -t bowel .
docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v7 -t you/bowel --push .
```

Every stage that runs a command runs on the build machine and cross-compiles, so a multi-platform build needs no emulation.
