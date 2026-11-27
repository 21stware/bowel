# Command line

The same program is a command line for the running bowel. How to put it in your `PATH`, and the commands most people need, are in the [README](../README.md#command-line); this page has the rest.

## Full screen

`bowel tui` is the same on a whole terminal: pages for the overview, connections, logs, rules, Providers and settings, kept current. The overview has the switches (takeover, system proxy, TUN, gateway mode, allow LAN) and the routes: Enter or Space turns a switch, Enter on a route chooses its node (`t` tests the delays), `m` switches the mode and `a` adds a subscription. The settings page changes any setting by key. `/` filters connections and `?` lists the keys. A change asks y/n first. It works over SSH: `ssh server -t bowel tui`.

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

## A port per route group

Two programs on this computer can reach the same site by different exits when each is given a proxy port of its own: `bowel settings set extraPorts '[{"port":7901,"route":"ai"},{"port":7902,"route":"media"}]'` opens mixed ports on `127.0.0.1` whose traffic all goes to that route group (the IDs `bowel routes` lists), whatever the rules and the mode say. While an imported config's own routing is in use, `route` is the name of one of that config's groups instead, which `bowel routes` lists below the routes; a port whose group is not there stays closed. One name can be both a bowel route ID and a group of the config: such an entry is open under either routing, and its traffic goes to whichever group has that name there. They are never open to the LAN, are not part of an exported configuration, and have no place in the window; `'[]'` closes them.

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

That environment stays behind on `docker exec` and `ssh`, so an agent's command run through them looks like yours and is not asked about. What it can't do is let agents do more: loosening the policy (ask to allow, deny to ask) needs you at a terminal. `bowel settings set mcpPolicy allow` asks you to type `allow`, and is refused where there is no terminal to ask at: in a script, in `docker exec` without `-it`, in `ssh` without `-t`. `bowel tui` asks the same way, on its settings page; the app's Settings > MCP needs nothing more. Making the policy stricter is never asked about.
