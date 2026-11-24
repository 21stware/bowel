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

ENVIRONMENT_TABLE

Everything else is a setting, changed with `bowel settings set <key> <value>` (`bowel settings list` shows them), in the TUI's settings page, or through an agent. Settings live in `/data/settings.json`, beside the Providers, the rules and the statistics; keep `/data` on a volume. A backup made in the desktop app can be imported here, and the other way round.

The container starts in Chinese unless `BOWEL_LANG` says otherwise (`en`, `ja`, `zh-Hant`), or `bowel settings set language en` changes it for good.

## TUN and gateway mode

The proxy port needs nothing special. TUN, which takes all traffic of the network it is on, needs the TUN device and the right to configure the network:

TUN_SECTION

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

The image's health check is `bowel health`: healthy while the daemon answers and the core runs. `docker logs bowel` has the daemon's log; the core's is `docker exec bowel bowel logs -f`. `docker stop` stops it cleanly.

## Building it yourself

```bash
docker build -t bowel .
docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v7 -t you/bowel --push .
```

Every stage that runs a command runs on the build machine and cross-compiles, so a multi-platform build needs no emulation.
