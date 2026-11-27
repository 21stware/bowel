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

- A computer that stays on, on the same LAN as the devices, by cable if possible. It should keep its address: give it a fixed one, or a DHCP reservation on the router. A changed address leaves every device pointing at nothing. For [automatic](#the-whole-lan-automatic) only a fixed address set on the computer itself will do: the router's DHCP is off then, and a reservation with it.
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
- **G needs a fixed address**, set on G itself (manually, in the system's network settings) with R as its router. A reservation on the router is not enough: the router's DHCP is about to be off, nobody would renew G's lease, and when it ran out the LAN would lose its router, DNS and DHCP at once. So bowel refuses to turn automatic on while G's address comes from DHCP (`"dhcp": true` in the gateway object) and says so; if the address becomes a leased one later, bowel stops handing out addresses, the line turns red with the reason, and it goes on by itself once the address is fixed again. G does not take an address from its own server: it answers neither its own DHCP client nor a request for its own address.
- **While G is down, the LAN has no DHCP.** Devices keep their addresses for the rest of a lease (20 minutes) but have no way out, since their router is G. This is the price of automatic; a machine that stays on, with bowel as a service, is what makes it worth paying.
- **Going back:** turn the router's DHCP on again, then `bowel gateway manual` (or *关闭* on the same line in the app). Devices return to the router within ten minutes. In the other order they are offline in between.
- **Only on the LAN you turned it on for.** bowel remembers that LAN (its network, its router and the router's hardware address). On any other network, a laptop taken to an office say, it hands out nothing and says so; back on its own LAN it goes on. To use automatic on another LAN, turn it off and on again there.
- **It asks again before every start.** At launch, after a pause, and when this computer's address changed, bowel first checks the LAN once more (a few seconds). While another server answers it stays out, says which one, and starts by itself once that one is off.
- bowel keeps checking while it serves: if the router's DHCP comes back, the gateway's line turns red and says which server it is.

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

The same object has `.auto`, the automatic step: `wanted`, `status` (`off`, `on`, `starting` while the LAN is checked before a start, `error` with `detail`), `servers` (the other DHCP servers the last check found), `pool` and `leases`.

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

## A route for one device

Every device follows the same route groups, unless it is given a route of its own: then **all** of its traffic goes one way, whatever the groups' rules would do with it. A TV to 流媒体, so that every app on it leaves by the streaming node; a camera or a smart plug to 直连, so that it never uses a node; a guest's tablet to 拦截, so that it gets nothing through G; or a device left out of the gateway altogether.

In the app: **日志 → 设备**, open the device, and choose under *路由*. From a terminal:

```bash
bowel gateway devices                         # the Route column says what each has
bowel gateway device 192.168.1.50 media       # all of the TV's traffic to 流媒体 (a group's id or name)
bowel gateway device 192.168.1.60 direct      # the camera never uses a node
bowel gateway device tablet reject            # by its name, as the list shows it: blocked
bowel gateway device 192.168.1.70 bypass      # left out of the gateway
bowel gateway device 192.168.1.50 rules       # back to the rules every device follows
```

A device is named by its address, its hardware address or its name as `bowel gateway devices` shows them. An address on the LAN that no device has used yet is taken too, for a device that is still to be pointed at G. The device's open connections are closed, so that what it opens next goes the new way.

**Where it comes in the order.** Route groups match top to bottom ([routes.md](guide/routes.md)). A device's route comes before every group, so that it takes everything, but for two kinds of rules that hold for every device:

- **拦截 (Block)**: what is blocked stays blocked on the TV too.
- **Groups with the exit lock**: their traffic leaves by their node or not at all, from a device as from G. A camera sent direct still sends a locked AI group's sites through that group's node.

The [developer networks](dev-network.md) (LAN, Tailscale, VPN) come first as always. A device sent to 拦截 has nothing before it: it gets nothing through G at all, though G's DNS still answers it. Like every rule, a device's route holds in rule mode, not in global or direct mode, and not while a provider's own routing is in use (its groups are not bowel's).

**Left out of the gateway** (`bypass`) is for automatic: at its next renewal, at most 10 minutes later (at once if its Wi-Fi reconnects), bowel's DHCP gives the device the real router R as its router and DNS, so its traffic no longer reaches G. It keeps its address, which bowel goes on handing out. A device pointed at G by hand cannot be turned away by bowel: its traffic is sent direct, and to take it out altogether set its router and DNS back to R.

**How a device is known.** The core sees a device by its address only, so that is what a route goes by. bowel keeps the address the device's:

- In automatic, bowel keeps the address for the device's hardware address: it gets that address at every lease, and no other device does.
- When the device turns up at another address anyway (the router's DHCP gave it a new one, or it was set by hand), its route follows its hardware address there, from bowel's leases or the ARP table.
- When its address is seen with another device and the device itself is not seen, the route waits for it (`"away": true` in `bowel gateway devices --json`) rather than take that other device.
- A device whose hardware address is not known yet (`"mac": ""`) is known by its address alone; bowel learns the hardware address once the device has used the gateway.

The routes are kept in `settings.json` as `gatewayDevices`, and stay on this computer: an exported configuration does not carry them.

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
| `bowel gateway auto` says this computer gets its address from DHCP | G's address is leased (`"dhcp": true`); with the router's DHCP off it would run out | set a fixed address on G itself (same address, R as router and DNS), then `bowel gateway auto` |
| automatic: "this LAN's router isn't known" | G has no default gateway set | set R as the router in G's network settings |
| automatic: "needs to run as root" | Linux, bowel as an ordinary user | run the daemon as a system service |
| automatic is on and a device has no address | it was off the network when the router's DHCP went, or its lease ended while G was down | reconnect it; check G is up |
| automatic: "this isn't the LAN it was turned on for" | the computer is on another network, or the router was replaced | nothing to do elsewhere; on a new router, `bowel gateway manual` then `bowel gateway auto` |
| automatic: the line is red, "… hands out addresses too" | the router's DHCP was switched back on (a firmware reset does that) | turn it off again, or `bowel gateway manual` |

## Limits

- **IPv4 only.** bowel forwards and answers for IPv4. A device that also has IPv6 from the router can reach IPv6 sites past the gateway. If that matters, turn IPv6 off on the device, or on the router's LAN side.
- **A device's route is all or nothing.** A device can be given [a route of its own](#a-route-for-one-device), which takes all of its traffic, or follow the groups as every device does; it cannot have rules of its own beside the groups'. Its route holds in rule mode only, and not under a provider's own routing.
- **A device is known by its address.** That is all the core sees of it, and a route goes by it. One pointed by hand shows without a name; one that got its address from bowel shows the name it gave. Its route follows its hardware address to a new address only once the device has used the gateway or renewed a lease there; until then it waits.
- **Left out means direct for a device pointed by hand.** bowel can only take a device out of the gateway in automatic, by its DHCP answer.
- **Not on Windows**, and not on musl systems (OpenWrt, Alpine).
- Connections show the app as unknown for other devices: bowel can only name programs of the computer it runs on.

## Turn it off

```bash
bowel gateway off
```

Forwarding goes back to what it was, port 53 closes, TUN stays as you had it. Then point the devices back: automatic (DHCP) on each, or the router's DHCP options back to R. Nothing else was changed on the system; the one-time authorization is the files listed under *What an authorization leaves behind* in [platform-parity.md](platform-parity.md).

## For agents

Everything above is reachable from the command line, and over MCP (below); every reading command takes `--json`. Exit codes: 0 done, 1 failed (for example bowel is not running), 2 the command was wrong.

**Permission.** bowel tells an agent's commands from the user's by the agent's environment, and by default asks the user before a change. With the app open, a dialog appears: tell the user to expect it. A daemon has no window and refuses instead; the user (not you) can allow agents' changes at their own terminal, where `bowel settings set mcpPolicy allow` asks them to type `allow`. Run without a terminal, as through your `docker exec` or `ssh`, it is refused: don't answer that question, and don't add `-t` to make it ask. In the Docker image your `docker exec` is an agent's whatever it runs, so its changes follow the same policy. Reading never asks.

**Steps.**

1. Is it running, and what is there? `bowel status --json`. Exit code 1: start it (open the app, or `systemctl start bowel`). On a server, use the service's data folder for every command (`BOWEL_DATA_DIR`).
2. No Provider (`"providers": 0`)? Ask the user for a subscription link, then `bowel providers add '<link>'`. Never invent one.
3. `bowel gateway on`. Then poll `bowel status --json` until `.takeover.gateway.status` is `on` (a few seconds). On `install`, the user must authorize once in the app, or the daemon must run as root. On `error`, read `.takeover.gateway.detail` and the table above.
4. Read `.takeover.gateway.ip` (G) and `.router` (R). Warn if `.dhcp` is true (G's address may change), `.wifi` is true (slow), or `.firewall` is true (may block).
5. Tell the user what to set on the device: router = G, DNS = G only. You cannot do this step from G.
6. Verify from G: `bowel gateway devices --json` lists the device (`"via": "manual"`, `"online": true`) once it has used the gateway.
7. Only if the user wants every device covered, and `.dhcp` is false (with a leased address `bowel gateway auto` refuses: a fixed address on G is the user's to set, so say so and stop): `bowel gateway check --json`. A non-empty `.servers` names the DHCP server to turn off; that is the user's to do in the router's admin page, and you say so and stop. With `.servers` empty, `bowel gateway auto`; then `.takeover.gateway.auto.status` is `on`. `error` comes with `.detail`.
8. Only if the user asks for one device to go another way: `bowel gateway device <device> <route> --json`, the device by its `.ip`, `.mac` or `.name` from `bowel gateway devices --json`, the route a group's id from `bowel routes --json` (`direct` sends it direct, `reject` blocks everything it does), `bypass` (left out of the gateway), or `rules` (back to the groups). The answer's `.device` has `.route` or `.bypass`, and `.note` says what still stands between the change and the device (for example the renewal a device left out waits for). Before `reject` or `bypass`, say what the device will lose. Over MCP the same is `set_device_route` (`device`, `route`), following the same permission.

**To undo:** `bowel gateway off`, then the devices back to automatic.

**Do not** stop bowel, turn the gateway off or run `bowel gateway manual` to "test" while `.takeover.gateway.auto.status` is `on`, or on a LAN whose router hands out G: every device goes offline. Do not run `bowel gateway auto` unless the user asked for the whole LAN, and never change a router's settings yourself.

**Over MCP.** The same steps work without a shell on G, through bowel's MCP server: on G's socket (`bowel mcp`), or over HTTP from another machine ([mcp-http.md](mcp-http.md)). `get_gateway` is the reading of steps 1 and 4, with `next` saying what comes next and whether it is the user's step or yours; `provider_add` is step 2, `set_gateway` step 3 and the undo, `list_devices` step 6, `check_dhcp` and `set_gateway_auto` step 7, `set_device_route` step 8. The guard rails above are in the tools: `set_gateway_auto` refuses automatic while another DHCP server answers or while G's address is leased, and an agent's `set_gateway` off or `set_gateway_auto` back to manual is refused, with the reason, while it would take devices offline. Pass `force: true` only after the user has heard that reason and still wants it.
