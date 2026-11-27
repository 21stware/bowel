# One agent, several bowels

One agent can look after the bowels on several machines: the laptop it runs on, a NAS that is the home LAN's gateway, a VPS, the office's side router. Each bowel serves MCP over HTTP, and each is its own MCP server in the agent's configuration, named for its machine. This page covers turning the endpoint on, TLS, naming the servers, and what an agent can and cannot do through them.

## Turn it on

Every shape of bowel serves MCP over HTTP when `BOWEL_MCP_HTTP` is set as it starts (`:7900` listens on every interface; `127.0.0.1:7900` or a VPN address on one):

| Where bowel runs | How |
| --- | --- |
| The Docker image | on already, on `:7900`; publish the port ([docker.md](docker.md#agents-mcp)) |
| `bowel daemon` under systemd | `Environment=BOWEL_MCP_HTTP=:7900` in the unit, then `systemctl restart bowel` |
| The macOS app | `launchctl setenv BOWEL_MCP_HTTP :7900`, then quit and reopen bowel (repeat after a restart of the Mac) |
| The Windows app | `setx BOWEL_MCP_HTTP :7900`, then quit and reopen bowel |
| The Linux app | `BOWEL_MCP_HTTP=:7900` in `~/.config/environment.d/bowel.conf` (systemd-based desktops read it), then log out and in |

Every request brings a bearer token. It is made on the first start and kept in the data folder (`mcp-token`), or set with `BOWEL_MCP_TOKEN` (16 characters or more). On the machine bowel runs on, `bowel mcp token` prints it with the lines that add the server to Claude Code and other clients. It reads what the running bowel serves from its data folder (`mcp-http.json`), so it is right from a shell that lacks the service's environment too; a daemon with a data folder of its own needs that folder named (`BOWEL_DATA_DIR`).

`get_status` tells an agent what a bowel serves: `.mcpHttp` has the URL, and with TLS its certificate's fingerprint.

## TLS

The endpoint is plain HTTP unless TLS is turned on, and then the token and every answer cross the network in the clear. On `127.0.0.1`, or over a VPN that encrypts already (Tailscale, WireGuard), that is fine. Anywhere else, use one of these:

### A certificate bowel makes

`BOWEL_MCP_TLS=on`. The first start makes a self-signed certificate and keeps it in the data folder as `mcp-tls.pem`, with its key, readable by bowel's user only. It lasts ten years and is made for `localhost`, the machine's name and its addresses, and for what `BOWEL_MCP_TLS_HOSTS` adds (names and addresses, separated by commas or spaces). In a container the machine's addresses are the container's, so name the host's:

```bash
docker run -d --name bowel --restart unless-stopped \
  -p 7899:7899 -p 7900:7900 -v bowel:/data \
  -e BOWEL_MCP_TLS=on -e BOWEL_MCP_TLS_HOSTS=nas.lan,192.168.1.5 \
  ghcr.io/21stware/bowel
```

The certificate stays the same across restarts. A name added to `BOWEL_MCP_TLS_HOSTS` later makes a new one, with a new fingerprint, which the clients then trust again.

`bowel mcp token` prints the certificate's SHA-256 fingerprint, and `bowel mcp cert` prints the certificate itself, without the key. The agent's machine trusts each bowel's certificate. One file can hold them all:

```bash
mkdir -p ~/.config/bowel
ssh nas 'docker exec bowel bowel mcp cert' > ~/.config/bowel/bowel-home.pem
ssh vps 'sudo BOWEL_DATA_DIR=/var/lib/bowel /opt/bowel/bowel mcp cert' > ~/.config/bowel/bowel-vps.pem
cat ~/.config/bowel/bowel-*.pem > ~/.config/bowel/trust.pem
```

Copied over SSH, the certificate is the machine's (SSH checked the host). Copied any other way, check it before the first connection: this prints the fingerprint the endpoint shows, which must be the one `bowel mcp token` printed on that machine.

```bash
openssl s_client -connect nas.lan:7900 </dev/null 2>/dev/null | openssl x509 -noout -fingerprint -sha256
```

Then start the client with the file:

| Client | How it trusts the file |
| --- | --- |
| Claude Code, and other clients on Node.js | `NODE_EXTRA_CA_CERTS=~/.config/bowel/trust.pem claude`, or `export NODE_EXTRA_CA_CERTS=…` in the shell's profile. It is read once, at the start, and adds to the system's roots. |
| curl | `--cacert ~/.config/bowel/trust.pem` |
| Python (httpx, requests) | `verify=` with the file's path in the client; `SSL_CERT_FILE` also works, and replaces the default roots |

`curl --cacert ~/.config/bowel/trust.pem https://nas.lan:7900/mcp` answering 401 with a JSON error means TLS works and only the token is missing. Never turn checking off instead (`NODE_TLS_REJECT_UNAUTHORIZED=0`, `-k`): that is every connection the agent makes left unchecked, not only bowel's.

### A certificate of your own

`BOWEL_MCP_TLS_CERT` and `BOWEL_MCP_TLS_KEY` name a certificate and its key, PEM files: Let's Encrypt's `fullchain.pem` and `privkey.pem`, what `tailscale cert` writes, a company CA's. A client that trusts the issuer needs nothing more. bowel reads the files again when they change, so a renewal needs no restart; in a container, mount the folder that holds them rather than the files, since a renewal replaces them.

### A proxy in front

A TLS proxy (Caddy, nginx, a tunnel) in front of a plain endpoint works too, as long as it keeps the `Host` header: the endpoint refuses a request whose `Origin` names another host, which is what keeps a web page from reaching it through the browser.

## Name each bowel

`bowel mcp token --name bowel-home` prints the lines with that name in place of `bowel`. Add each machine under its own name, at the user scope so that every project sees them:

```bash
claude mcp add --scope user --transport http bowel-home https://nas.lan:7900/mcp --header "Authorization: Bearer <token>"
claude mcp add --scope user --transport http bowel-vps https://vps.example.com:7900/mcp --header "Authorization: Bearer <token>"
claude mcp add --scope user bowel -- /Applications/bowel.app/Contents/MacOS/bowel mcp   # this Mac, over its socket
```

Other clients take the same in their JSON:

```json
{
  "mcpServers": {
    "bowel-home": { "type": "http", "url": "https://nas.lan:7900/mcp", "headers": { "Authorization": "Bearer <token>" } },
    "bowel-vps": { "type": "http", "url": "https://vps.example.com:7900/mcp", "headers": { "Authorization": "Bearer <token>" } }
  }
}
```

The agent sees each server's tools under its name (`mcp__bowel-home__get_status`), and a call changes only the machine of the server it is made on. Say which machine is which where the agent reads it, a project's `CLAUDE.md` for one: "bowel-home is the NAS, the LAN's gateway; bowel-vps is the proxy for the CI runners".

## What an agent can do on each

The tools are those of the socket, with the same policy: each bowel's own `mcpPolicy` (ask, allow or deny), which only the user sets, on that machine. A daemon or a container has no window to ask in, so under the default, ask, an agent reads everything there and changes nothing until the user allows changes on that machine, at a terminal of their own: `bowel settings set mcpPolicy allow` asks them to type `allow` (`docker exec -it`, `ssh -t`), as [cli.md](cli.md#agents) explains.

The guard rails below hold for agents on MCP. A command an agent runs through `docker exec` or `ssh` arrives without the agent's environment and reads as the user's, so on that path they don't apply: give an agent MCP rather than a shell on the machine.

Besides the reading tools (`get_status`, `list_routes`, `list_connections`, `get_logs` and the rest), the gateway and the providers can be driven without a shell on the machine:

| Tool | The command it is | What |
| --- | --- | --- |
| `get_gateway` | `bowel status --json`, `.takeover.gateway` | the gateway's state, and `next`: what comes next and whose step it is (`user` or `agent`) |
| `check_dhcp` | `bowel gateway check` | who hands out addresses on the LAN |
| `set_gateway` | `bowel gateway on` / `off` | the gateway's switch |
| `set_gateway_auto` | `bowel gateway auto` / `manual` | bowel as the LAN's DHCP server, or back to manual |
| `list_devices` | `bowel gateway devices` | the devices that go out through the gateway |
| `set_device_route` | `bowel gateway device <device> <route>` | one device's traffic one way, whatever the rules say |
| `provider_add`, `provider_remove`, `update_providers` | `bowel providers add` / `remove` / `update` | the providers |

The [gateway's runbook](gateway.md#for-agents) is built into the tools:

- `set_gateway_auto` with `enabled: true` is refused while another DHCP server answers on the LAN and while the gateway's own address is leased. That holds for everyone.
- An agent's `set_gateway` with `enabled: false` is refused while bowel hands out the LAN's addresses, and while devices are online through the gateway. Its `set_gateway_auto` with `enabled: false` first checks the LAN and is refused while no other DHCP server answers. Each refusal says why, and what the user does first. `force: true` goes ahead, for when the user has heard that and still wants it. The user's own commands are never refused.
- What only the user can do (authorize the gateway once in a dialog, point a device at it, give the gateway a fixed address, turn off the router's DHCP) is marked as theirs in the tools' descriptions and in `get_gateway`'s `next`.
- `provider_add` over HTTP takes a subscription URL or a node link, never a path: the files of the machine bowel runs on are not the remote agent's to read. A subscription's URL holds its token, so the answer, an error, the dialog that asks the user and the list of agents' changes name it by its host.

## Keep the token safe

The token is the key to that bowel: whoever has it reads its configuration, connections and logs, and changes what the policy allows. Keep it in the agent's configuration, not in a repository. To change it, delete `mcp-token` in the data folder (or set another `BOWEL_MCP_TOKEN`), restart bowel, and update the clients. Publish the port only where the clients are: `127.0.0.1`, the VPN's address, the LAN.
