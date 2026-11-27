# Command line

The same program is a command line for the running bowel. How to put it in your `PATH`, and the commands most people need, are in the [README](../README.md#command-line); this page has the rest.

## Full screen

`bowel tui` is the same on a whole terminal: pages for the overview, connections, logs, rules, Providers and settings, kept current. The overview has the switches (takeover, system proxy, TUN, gateway mode, allow LAN) and the routes: Enter or Space turns a switch, Enter on a route chooses its node (`t` tests the delays), `m` switches the mode and `a` adds a subscription; on a route, the keys of [route groups](#route-groups) set it up. The settings page changes any setting by key. `/` filters connections and `?` lists the keys. A change asks y/n first. It works over SSH: `ssh server -t bowel tui`.

## In a status bar

`bowel bar` prints a line a second for waybar (Omarchy's top bar, and Hyprland's and Sway's usual one): the speeds now as the text, the state, mode, takeover, each route's node and today's traffic in the tooltip, and the classes `on`, `off`, `paused` or `stopped`, plus `degraded` or `error`, to style it by. It is started once and goes on while bowel restarts or isn't running. Running the program again without a command brings the window up, so a click can open it:

```jsonc
// ~/.config/waybar/config.jsonc: add "custom/bowel" to a modules list too
"custom/bowel": {
  "exec": "/opt/bowel/bowel bar",
  "return-type": "json",
  "on-click": "/opt/bowel/bowel"
}
```

`#custom-bowel.off`, `#custom-bowel.stopped` and the rest style it in `style.css`. For polybar, i3blocks and other bars that read plain text, `bowel bar --format text`; for one that runs a command on an interval, add `--once`. Under `bowel daemon` there is no window to open.

## Route groups

Everything the window's route group list does is here too, so a bowel under `bowel daemon` or in Docker can be set up without it. `bowel route show` lists the groups in the order they apply, each with its strategy and main test site; `bowel route show <group>` has one whole: its strategy, range, backup order, the node in use, test sites, services and your rules. A group is named by its ID or its name.

```bash
bowel route new Netflix --before media            # a group of your own: the fastest node, no rules yet
bowel rules add netflix.com Netflix               # send traffic to it
bowel route move Netflix --at 1                   # tried from the top; or --before / --after another group
bowel route strategy Netflix fixed --node "🇯🇵 Tokyo 02"
bowel route order Netflix "🇯🇵 Tokyo 02" "🇸🇬 Singapore 01"   # the backup order, first preferred
bowel route strategy Netflix auto                 # back to the fastest; each strategy keeps its settings
bowel route scope Netflix --regions JP,SG --providers a1b2c3 --except   # the fastest's range; all for every one
bowel route strategy Netflix --by download --every 12   # choose by download speed, tested every 6, 12 or 24 hours
bowel route sites Netflix https://www.netflix.com fast.com   # test sites, the main one first; --reset for the defaults
bowel route direct Netflix on                     # out without a node; the strategy is kept for when it's off
bowel route rename Netflix Video
bowel route remove Video --to media               # its services and rules go to media (default unless given)
```

The range of the fastest is its regions (country codes), its Providers (only those, or with `--except` all but those) and `--nodes` picked by hand, all at once; a list takes commas, or a JSON list (`'["a, b","c"]'`) for names with commas in them. A new main test site is first tried through the node the group uses, and refused if it fails there. Other, Direct and Block always exist and keep their names, and Other stays last; Direct and Block never use a node, so they have no strategy or test sites.

While a Provider's own config routes the traffic, the groups are that config's: these commands refuse, `bowel route use` still picks a member of one of its select groups, and `bowel routing bowel` goes back to bowel's own groups, which are kept meanwhile. `bowel routing <provider>` uses that config's routing, and `bowel routing` says which is in use.

In `bowel tui`, on a route's row of the overview, `s` sets its strategy, `J` and `K` move it down and up, `e` edits its test sites, `R` renames and `d` deletes it, and `n` makes a new group; in its node list, `o` puts a node in the backup order or takes it out. Agents have the same as MCP tools (`list_route_groups`, `create_route_group`, `move_route_group`, `set_route_strategy`, `set_route_sites`, `rename_route_group`, `delete_route_group`, `set_routing_source`), under the same policy as their other changes.

## A port per route group

Two programs on this computer can reach the same site by different exits when each is given a proxy port of its own: `bowel settings set extraPorts '[{"port":7901,"route":"ai"},{"port":7902,"route":"media"}]'` opens mixed ports on `127.0.0.1` whose traffic all goes to that route group (the IDs `bowel routes` lists), whatever the rules and the mode say. While an imported config's own routing is in use, `route` is the name of one of that config's groups instead, which `bowel routes` lists below the routes; a port whose group is not there stays closed. One name can be both a bowel route ID and a group of the config: such an entry is open under either routing, and its traffic goes to whichever group has that name there. Each port takes HTTP and SOCKS5, UDP included, as the main port does. With the group's exit locked (`bowel route lock ai`), everything that comes in by its port leaves by a node or is refused, never directly.

A port is this computer's alone unless its entry says `"lan": true`: `bowel settings set extraPorts '[{"port":7901,"route":"ai","lan":true}]'` opens it to the LAN as the main port is, only while `allowLan` is on, to the networks of `lanAllowedIps`, and behind `authentication` when that is set (this computer needs no password). In a container (`BOWEL_CONTAINER`) every extra port is open that way, since the container's loopback is no one else's, and `BOWEL_EXTRA_PORTS` sets them from the environment: see [docker.md](docker.md#egress-for-other-containers). Extra ports are not part of an exported configuration and have no place in the window; `'[]'` closes them.

## In scripts

Every command has `--help` with examples. Reading commands take `--json`, and `logs -f` and `conns --watch` print one JSON object per line with it. Colors and progress go away when the output is not a terminal or `NO_COLOR` is set. Exit codes are 0 for done, 1 when it failed (for example bowel isn't running) and 2 for a wrong command line. `bowel completion bash|zsh|fish|powershell` prints a completion script.

## Without a window

`bowel daemon` runs bowel with no window, menu bar icon or Dock icon: the core, the monitors and the command line's socket only. It stays in the foreground and stops on SIGINT, SIGTERM or `bowel stop`, putting the system proxy back as the app does, so systemd, launchd or a service wrapper can keep it. `BOWEL_DATA_DIR` (or `--data-dir`) gives it a data folder of its own. The daemon does not update itself. Opening the app while the daemon runs does nothing: stop the daemon first.

`bowel health` exits 0 when the daemon answers, the core runs and its port accepts connections, and 1 otherwise, for a service manager's or Docker's health check. In a container (`BOWEL_CONTAINER` set), the first start opens the proxy to private addresses and leaves the system proxy alone; the environment variables it reads, and running it in Docker, are in [docker.md](docker.md).

```ini
# /etc/systemd/system/bowel.service
[Service]
ExecStart=/opt/bowel/bowel daemon
Environment=BOWEL_DATA_DIR=/var/lib/bowel
Restart=on-failure
```

## Agents

With `BOWEL_MCP_HTTP` set (`:7900`, say), bowel also serves MCP over HTTP at `/mcp`, for agents on other machines, behind a bearer token kept in the data folder; `bowel mcp token` prints it and the lines that add bowel to Claude Code and other clients. `BOWEL_MCP_TOKEN` gives a token of your own.

Commands you type are not asked about. A command run by an agent (Claude Code, Codex, Cursor and others are recognised by their environment) follows the same policy as MCP: ask, allow or deny. The daemon has no window to ask in, so under "ask" it refuses an agent's change; allow them with `bowel settings set mcpPolicy allow`, which only you can run.

That environment stays behind on `docker exec` and `ssh`. In the Docker image, where every command comes through `docker exec`, a command counts as yours only with your terminal behind it (`docker exec -it`); without one it counts as an agent's, and its changes follow the policy. Over `ssh`, an agent's command still looks like yours and is not asked about. What neither can do is let agents do more: loosening the policy (ask to allow, deny to ask) needs you at a terminal. `bowel settings set mcpPolicy allow` asks you to type `allow`, and is refused where there is no terminal to ask at: in a script, in `docker exec` without `-it`, in `ssh` without `-t`. `bowel tui` asks the same way, on its settings page; the app's Settings > MCP needs nothing more. Making the policy stricter is never asked about.
