# Tailscale and Headscale

bowel works alongside a tailnet: the tailnet's addresses and names stay off the proxy nodes, whether it runs on Tailscale's own servers or on a self-hosted [Headscale](https://github.com/juanfont/headscale).

- **macOS, Windows, Linux**: bowel uses the Tailscale client installed on the system. Point the client at Headscale and there is nothing to set in bowel.
- **Android**: the system allows one VPN at a time, so bowel carries a Tailscale node of its own and reaches the tailnet without the Tailscale app. For Headscale, give it the control server.

## On a computer

Log the Tailscale client in to Headscale, for example:

```sh
tailscale up --login-server https://headscale.example.com
```

bowel reads the client's state every 20 seconds and sends these direct, in rule, global and direct mode alike:

- addresses in `100.64.0.0/10` and `fd7a:115c:a1e0::/48`
- MagicDNS names: the suffix comes from the client's state, so Headscale's `base_domain` is treated as `ts.net` is, with nothing to fill in
- short host names, as in `ssh mybox`
- subnet routes, and the Split DNS domains an admin set

An exit node competes with TUN for the default route; bowel doesn't work around it, and the settings page warns about it. How all this is done is in [developer networks](../dev-network.md).

## On Android

Open Settings › Tailscale.

1. **Control server**: Headscale's address, such as `https://headscale.example.com`. http works too, for a Headscale without TLS. Empty is Tailscale's own server.
2. **MagicDNS domain**: shown once a control server is set. Fill in the `base_domain` of Headscale's config, such as `tail.example.com`. bowel can't ask the node for it, so it has to be written here; without it the tailnet's devices are reachable only by their `100.x` addresses. It can't be the control server's domain or one above it (not `example.com` for `headscale.example.com`), or the login page would be sent into the node before it has logged in. Headscale refuses such a `base_domain` too.
3. **Device name**: the name on the tailnet; empty is `bowel-android`.
4. Turn on Built-in Tailscale. bowel's VPN has to be on, and the Tailscale app off.
5. Log in, either way:
   - **In a browser**: Log in › Open the login link, and register the device on the server as Headscale's page says.
   - **With an auth key**: under "Optional: log in without a browser", fill in a key made on the server with `headscale preauthkeys create`. Hex keys and `hskey-auth-…` keys both work. The key is used only for the first login, after which bowel keeps the login, so a one-off key is best.

Once logged in, `100.64.0.0/10`, `*.ts.net` and the names under the MagicDNS domain go through the built-in node in every mode, and everything else as before. bowel fetches subscriptions that live on the tailnet through it too.

### Changing the control server

A node belongs to one control server, so changing it logs out: the node gets a new state folder and the old one is deleted, and the auth key is cleared unless a new one is given with it. Log in again on the new server.

Log out only makes this device forget its login; the control server still lists it, so delete it there if need be (`headscale nodes delete`).

## The other way: an imported `type: tailscale` node

Before the built-in node, Headscale took an imported mihomo node of `type: tailscale` with `control-url` and `auth-key`, a route group fixed to it, and rules of your own for the tailnet's ranges and domains. That works in rule mode only and isn't needed any more; setups made that way keep working.

Such a node leads to the tailnet's devices, and without an exit node it can't reach a test site, so a failed test shows "—" rather than calling it broken, and Auto fastest never picks it for the internet.
