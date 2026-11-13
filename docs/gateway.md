# Gateway mode

One computer running bowel becomes the way out for the rest of the LAN. A TV, a console, a phone or a whole network sets its **router (gateway)** and its **DNS** to that computer and follows bowel's routes, with nothing installed on the device and nothing changed on the real router's firmware. This is what is often called a side router (旁路由) or a transparent gateway.

Works on **macOS** and **Linux**. Windows has no gateway mode yet. This page is written to be followed by a person or by an agent; the [last section](#for-agents) is the same thing as a runbook.

## How it works

```
device ──(router = G, DNS = G)──▶ bowel on G ──▶ node ──▶ internet
                                      └──────▶ direct (LAN, 直连 rules)
real router R: still hands out addresses and is G's own way out
```

- The device asks G's port 53 for a name. bowel answers with a **fake IP** (`198.18.x.x`) and remembers the name.
- The device connects to that address through G. The kernel forwards the packet (IPv4 forwarding), TUN's routes hand it to the core, and the core sends it by bowel's rules: a node, direct, or rejected.
- A device with a resolver of its own written in (`8.8.8.8`) is answered by bowel all the same: port 53 is taken on its way through.
- Addresses on the LAN are not forwarded at all: devices reach each other directly, as before.

G keeps using the real router R as its own gateway.

The gateway has three steps, each on top of the one before:

| Step | What it is | What changes on the network |
| --- | --- | --- |
| off | no gateway | nothing |
| **manual** | the gateway answers; each device that should use it is pointed at it by hand | only the devices you set up |
| **automatic** | bowel also hands out the LAN's addresses (DHCP), so every device uses it by itself | the router's DHCP must be off; bowel's takes its place |

Turning the gateway on is always manual. Automatic is one more decision, offered only when no other DHCP server answers on the LAN.

## What you need

- A computer that stays on, on the same LAN as the devices, by cable if possible. It should keep its address: give it a fixed one, or a DHCP reservation on the router. A changed address leaves every device pointing at nothing.
- bowel with at least one Provider, and administrator rights once (macOS, Linux desktop) or root (Linux server).
- IPv4 on the LAN. See [Limits](#limits) for IPv6.

## Turn it on

### macOS, or Linux with a desktop

In the app: **设置 → 网关模式**, switch it on. The first time, the system asks for an administrator's authorization once, for TUN and for IP forwarding together. The card then shows the two values to type on devices, and four measured checks: TUN, IP forwarding, DNS on port 53, kept awake.

Or from a terminal, with the app running:

```bash
bowel gateway on
bowel status
```

### Linux server, no desktop

Run bowel as a service, as root. Root needs no authorization dialog, and nothing asks for a desktop.

```ini
# /etc/systemd/system/bowel.service
[Unit]
Description=bowel
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/opt/bowel/bowel daemon
Environment=BOWEL_DATA_DIR=/var/lib/bowel
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now bowel
```

Every command must name the same data folder as the service. An alias saves typing:

```bash
alias bowel='sudo BOWEL_DATA_DIR=/var/lib/bowel /opt/bowel/bowel'
bowel providers add 'https://example.com/your-subscription'
bowel gateway on
bowel status
```

`bowel gateway on` turns TUN and the takeover on with it, and is remembered: after a restart of the service or of the machine the gateway comes back by itself.

The Linux build needs glibc (Debian, Ubuntu, Fedora, Arch). It does not run on musl systems such as Alpine or OpenWrt.

## Point devices at it

Read the address to use from bowel, not from memory:

```bash
bowel status --json   # .takeover.gateway.ip is G; .takeover.gateway.router is R
```

### One device

In the device's network settings switch IPv4 from automatic to manual and fill in:

| Field | Value |
| --- | --- |
| IP address | the one it has now (keep it) |
| Subnet mask | as it is, usually `255.255.255.0` |
| Router / Gateway | **G** |
| DNS | **G**, and no second one |

A second DNS server is a way around bowel: the device may ask it instead and get real addresses, which then go by IP rules only.

### The whole LAN: automatic

bowel becomes the LAN's DHCP server and gives every device this computer as its router and DNS. Nothing is typed on any device, and a new device is covered the moment it joins.

Two DHCP servers on one LAN fight over every device, so the order is fixed:

1. Check who hands out addresses now: the app shows it under the two addresses (*内网代理 · DHCP 服务器 192.168.1.1*), and `bowel gateway check` prints it. The check asks as a device that just joined would, and takes no address.
2. Turn the DHCP service off in that router's admin page. Only DHCP: the router stays the way out.
3. Check again (*重新检测*, or `bowel gateway check`). With no server left the app offers *确认开启*; from a terminal it is `bowel gateway auto`. bowel refuses while another server still answers.

```bash
bowel gateway check    # who hands out addresses on the LAN
bowel gateway auto     # bowel does, from now on; refused while another server answers
bowel gateway devices  # who uses the gateway: address, name, automatic or manual, online
bowel gateway manual   # back to manual
```

What to know before the switch:

- **Nobody is cut off by it.** A device keeps the address it has and asks bowel to confirm it when its old lease runs down; bowel confirms any address of the LAN that nobody else holds. Devices move over one by one, within the router's lease time (reconnect a device's Wi-Fi to move it at once). New devices get addresses from the upper part of the LAN (`.100` to `.249` of a /24).
- **G needs a fixed address**, set on G itself with R as its router. G does not take an address from its own server.
- **While G is down, the LAN has no DHCP.** Devices keep their addresses for the rest of a lease (20 minutes) but have no way out, since their router is G. This is the price of automatic; a machine that stays on, with bowel as a service, is what makes it worth paying.
- **Going back:** turn the router's DHCP on again, then `bowel gateway manual` (or *关闭* on the same line in the app). Devices return to the router within ten minutes. In the other order they are offline in between.
- bowel keeps checking: if the router's DHCP comes back while bowel serves, the gateway's line turns red and says which server it is.

On Linux automatic needs root (the daemon as a system service has it); on macOS the app can do it as it is.

### The whole LAN, through the router's own DHCP

If the router lets you choose what its DHCP server hands out, set **default gateway = G** and **DNS server = G** there and every device follows at its next lease, with nothing typed on any of them. On Asuswrt and Asuswrt-Merlin this is *LAN → DHCP Server → Default Gateway* and *DNS Server 1*; OpenWrt has DHCP options 3 and 6 on the LAN interface.

G itself must not follow this: give it a fixed address with **R** as its gateway, or it would be its own way out.

When G is off, every device that got G from DHCP is offline until G is back or the router's setting is undone. That is the price of this way; pointing single devices keeps the others independent.

## Check that it works

On G:

```bash
bowel status          # the light, and "网关 开" in the takeover line
bowel status --json   # .takeover.gateway: status "on", forwarding, dns, tun all true
bowel conns --watch   # connections appear as the device is used
```

The same object has `.auto`, the automatic step: `wanted`, `status` (`off`, `on`, `error` with `detail`), `servers` (the other DHCP servers the last check found), `pool` and `leases`.

`.takeover.gateway.status` is one of:

| Status | Meaning | What to do |
| --- | --- | --- |
| `on` | TUN runs, forwarding is on, port 53 answers on the LAN address | nothing |
| `checking` | not measured yet | wait a few seconds |
| `paused` | the takeover or TUN is off, so the gateway is too | `bowel gateway on` |
| `install` | the one-time authorization is missing | turn it on in the app, or run as root |
| `error` | `detail` says which part does not work | see below |
| `off` | not wanted | `bowel gateway on` |

On a device (or any computer on the LAN, without changing its settings):

```bash
nslookup example.com G      # answers 198.18.x.x: bowel's resolver is reached
```

and, once the device points at G, any page that shows the visitor's address shows the node's, not the home's.

## What bowel keeps, and puts back

`scripts/gateway-e2e.sh` runs this for real: it builds bowel, puts a gateway, a device and a node in three containers, and checks every line below (Linux, as root; macOS is not covered by the script):

| Event | The gateway | Devices |
| --- | --- | --- |
| `bowel gateway off` | forwarding goes back to what it was before, port 53 closes; automatic ends too, and the next time starts at manual | offline until pointed back at R |
| bowel quits (`bowel stop`, the service stops) | the same, and the core ends | offline while it is down |
| bowel starts again | comes back by itself, automatic included, with the leases it had | online again |
| bowel is killed or crashes | gone until bowel starts again (`Restart=always` in the unit does that in seconds) | offline for that long |
| the machine restarts | comes back when the service starts | online again |
| the core is killed or crashes | bowel starts it again, after 1 s, then 2, 5, 15, 30, at most 60 | a gap of a few seconds |

The value forwarding had before is kept in `gateway-before.json` in the data folder while the gateway is on, so that a crash is still undone later.

## When it does not work

| What you see | Likely cause | Fix |
| --- | --- | --- |
| `detail`: 53 端口没有应答 DNS | another resolver has port 53 (`systemd-resolved`'s stub, `dnsmasq`, Pi-hole), or a firewall drops it | free the port (`DNSStubListener=no` for systemd-resolved) or allow UDP and TCP 53 from the LAN |
| `detail`: IP 转发没有打开 | something turned it off again | `bowel gateway off`, then `on` |
| `detail`: TUN 没有运行 | the core could not open TUN | `bowel logs --level warning` |
| `"firewall": true` in the gateway object and devices get nothing | ufw or firewalld drops what is forwarded | allow forwarding from the LAN and port 53 on G |
| the device has a network but no page loads | its DNS is not G, or it has a second DNS | set DNS to G alone |
| some apps on the device skip the gateway | the device has IPv6 and uses it | see [Limits](#limits) |
| it worked, and stopped after a day | G's address changed (`"dhcp": true`), or G went to sleep | fixed address or a reservation; on a laptop keep it on power |
| G is on Wi-Fi (`"wifi": true`) and devices are slow | every packet crosses the air twice | use a cable |
| everything is direct | no Provider, or the mode is direct | `bowel providers list`, `bowel mode rule` |
| `bowel gateway auto` says a server still hands out addresses | the router's DHCP is on, or a second router or mesh node has one | turn it off there, `bowel gateway check` until none answers |
| automatic: "needs to run as root" | Linux, bowel as an ordinary user | run the daemon as a system service |
| automatic is on and a device has no address | it was off the network when the router's DHCP went, or its lease ended while G was down | reconnect it; check G is up |
| automatic: the line is red, "… hands out addresses too" | the router's DHCP was switched back on (a firmware reset does that) | turn it off again, or `bowel gateway manual` |

## Limits

- **IPv4 only.** bowel forwards and answers for IPv4. A device that also has IPv6 from the router can reach IPv6 sites past the gateway. If that matters, turn IPv6 off on the device, or on the router's LAN side.
- **No rules by device.** Every device follows the same routes. The devices are listed (日志 → 设备 in the app, `bowel gateway devices`) with what each carries and when it came and went, but one device cannot be given a route of its own, or be left out of automatic.
- **A device is known by its address.** One pointed by hand shows without a name; one that got its address from bowel shows the name it gave.
- **Not on Windows**, and not on musl systems (OpenWrt, Alpine).
- Connections show the app as unknown for other devices: bowel can only name programs of the computer it runs on.

## Turn it off

```bash
bowel gateway off
```

Forwarding goes back to what it was, port 53 closes, TUN stays as you had it. Then point the devices back: automatic (DHCP) on each, or the router's DHCP options back to R. Nothing else was changed on the system; the one-time authorization is the files listed under *What an authorization leaves behind* in [platform-parity.md](platform-parity.md).

## For agents

Everything above is reachable from the command line; every reading command takes `--json`. Exit codes: 0 done, 1 failed (for example bowel is not running), 2 the command was wrong.

**Permission.** bowel tells an agent's commands from the user's by the agent's environment, and by default asks the user before a change. With the app open, a dialog appears: tell the user to expect it. A daemon has no window and refuses instead; the user (not you) can run `bowel settings set mcpPolicy allow`. Reading never asks.

**Steps.**

1. Is it running, and what is there? `bowel status --json`. Exit code 1: start it (open the app, or `systemctl start bowel`). On a server, use the service's data folder for every command (`BOWEL_DATA_DIR`).
2. No Provider (`"providers": 0`)? Ask the user for a subscription link, then `bowel providers add '<link>'`. Never invent one.
3. `bowel gateway on`. Then poll `bowel status --json` until `.takeover.gateway.status` is `on` (a few seconds). On `install`, the user must authorize once in the app, or the daemon must run as root. On `error`, read `.takeover.gateway.detail` and the table above.
4. Read `.takeover.gateway.ip` (G) and `.router` (R). Warn if `.dhcp` is true (G's address may change), `.wifi` is true (slow), or `.firewall` is true (may block).
5. Tell the user what to set on the device: router = G, DNS = G only. You cannot do this step from G.
6. Verify from G: `bowel gateway devices --json` lists the device (`"via": "manual"`, `"online": true`) once it has used the gateway.
7. Only if the user wants every device covered: `bowel gateway check --json`. A non-empty `.servers` names the DHCP server to turn off; that is the user's to do in the router's admin page, and you say so and stop. With `.servers` empty, `bowel gateway auto`; then `.takeover.gateway.auto.status` is `on`. `error` comes with `.detail`.

**To undo:** `bowel gateway off`, then the devices back to automatic.

**Do not** stop bowel, turn the gateway off or run `bowel gateway manual` to "test" while `.takeover.gateway.auto.status` is `on`, or on a LAN whose router hands out G: every device goes offline. Do not run `bowel gateway auto` unless the user asked for the whole LAN, and never change a router's settings yourself.

The MCP server (`bowel mcp`) has the same reads (`get_status`, `list_connections`, `get_logs`) but not the gateway's switch: use the command line for that.
