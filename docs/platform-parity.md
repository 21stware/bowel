# Platform parity

What the desktop app does on each system, taking macOS as the baseline: every behaviour a `_darwin.go` file gives has a row. A row says either how the other systems do the same thing, why they have no need of it, or that it is left for later and why.

How far each implementation has been tried is its own column, because "it compiles" and "it has run" are different claims:

- **macOS**: developed and run on.
- **Linux**: the package tests run on Ubuntu 24.04 (arm64, kernel 6.8), unprivileged and with no desktop session. Anything that needs a desktop (polkit's dialog, GNOME's or KDE's settings, the tray, a screenshot tool) or root on a real network (the TUN device itself, forwarding) has **not** been run.
- **Windows**: cross-compiled and vetted, with the logic that needs no Windows API tested on other systems. **Nothing has run on Windows on our side**; what is known from a real machine comes from users' reports, noted where it applies.

## The table

| Feature | macOS | Linux | Windows |
| --- | --- | --- | --- |
| TUN: one authorisation | `osascript` with administrator privileges | `pkexec` (polkit) | UAC prompt (`ShellExecuteEx`, `runas`) |
| TUN: what it grants | setuid-root copy of the core in the data directory | root-owned copy with file capabilities `cap_net_admin,cap_net_bind_service,cap_net_raw`; setuid root where `setcap` is missing | copy under `%ProgramData%\bowel\tun\<SID>\` writable by administrators only, and a scheduled task that runs it elevated |
| TUN: running the core | child process | child process | the task, started and ended with `schtasks`; log read from the controller |
| TUN: files the core's config names (providers, rule sets) | beside the core's directory, allowed through `SAFE_PATHS` in the child's environment | same | inside the core's directory (`core\providers`, `core\rulesets`): the task's core has no environment of the app's |
| TUN: its core does not start | the bundled core runs as a child in its place; the TUN entry says why, and turning TUN on again tries again | same | same; a task that was deleted counts as not installed, and is asked for again |
| TUN: stale copy after an update | offered, one authorisation | same | same |
| System proxy: set, restore | `networksetup`, every service | GNOME (`gsettings`, shared by Cinnamon, MATE, Budgie…) and KDE (`kwriteconfig`, `kioslaverc`) | WinINet LAN settings |
| System proxy: measured every 2 s | `scutil --proxy` | the session's own desktop's settings | WinINet |
| System proxy: other desktops | — | not available; the app says so and points at TUN | — |
| Developer networks: routes, tunnels | `netstat -rn`, interface names | `ip route show table all`, sysfs for what an interface is | `GetIpForwardTable2`, adapter types |
| Developer networks: split DNS | `scutil --dns` | `resolvectl` (systemd-resolved) | name resolution policy table, tunnel adapters' suffixes |
| Developer networks: Tailscale | CLI, or interface and resolver | same | same |
| Bind past TUN (IPv6 probe) | `IPV6_BOUND_IF` | `SO_BINDTODEVICE` | `IPV6_UNICAST_IF` / `IP_UNICAST_IF` |
| Ping a node's server | `/sbin/ping -b iface` | `ping -I iface` | `PING.EXE -S address` |
| Primary interface | `scutil` | `/proc/net/route` | route table |
| Gateway mode | sudoers rule for two `sysctl` commands, `caffeinate` | sudoers rule for two `sysctl` commands, `systemd-inhibit` | **later** |
| Gateway: network facts (Wi-Fi, DHCP, firewall) | `networksetup`, `ipconfig`, socketfilterfw | sysfs, `ip addr`, ufw / firewalld | adapters (Wi-Fi, DHCP); no firewall check |
| Which app owns a connection | bowel reads the kernel's socket list | **not needed** | **not needed** |
| Developer tools: terminal | zsh (`.zshenv`) | zsh or bash (`.bashrc`), by `$SHELL` | **later** |
| Developer tools: Git HTTPS, Git SSH | yes | yes; SSH with OpenBSD `nc` or `ncat` | **later** |
| Developer tools: Docker daemon | Colima | Colima only; the system's dockerd **later** | **later** |
| Download sources (npm, pip, Go, Cargo) | yes | yes | yes |
| QR: picture file | yes (HEIC and others through `sips`) | formats Go decodes | formats Go decodes |
| QR: screen | `screencapture`, per display | first of grim, spectacle, gnome-screenshot, maim, scrot, ImageMagick | PowerShell, .NET `CopyFromScreen` |
| QR: camera | AVFoundation | **later** | **later** |
| Dock icon, flag emoji drawing, quit pump | yes | **not applicable** | **not applicable** |
| Tray | menu bar | system tray | system tray |

## Not needed, and why

- **Which app owns a connection** (`internal/procinfo`). mihomo names the process behind a connection itself, on every system. `procinfo` exists only because macOS 27 hides the kernel's socket list from a process whose parent is an ad-hoc signed app, which the core is; bowel, opened like any app, reads the list for it. Linux (`sock_diag` and `/proc`) and Windows (`GetExtendedTcpTable`) put no such limit on the core, so the monitor's fallback is never asked.
- **Dock icon** (`syncDock`), **flag drawing** (`flagdraw_darwin.go`), **quit pump** (`quit_pump_darwin.go`). The first is the Dock's, which the other systems do not have. The second works around how macOS's text system colours emoji flags; elsewhere the plain drawing is right. The third keeps AppKit's run loop turning during a quit.
- **Screen Recording permission**. Only macOS keeps the screen behind a permission of its own. Elsewhere a capture that fails says so.

## Left for later, and why

- **Gateway mode on Windows.** Three things are missing, and each needs trying on Windows before it is written. Windows forwards per adapter: both the LAN's and TUN's must be switched, and TUN's is made anew each time. Nothing like a sudoers rule lets a user's process do that unelevated, so it would take a second task or a service. And the firewall drops what arrives to be forwarded, and the resolver's port 53, until rules are added. The interface does not offer the gateway on Windows.
- **Developer tools on Windows.** The model is another one: terminals are PowerShell and cmd, whose environment is the user's registry (`HKCU\Environment`) and a profile script; OpenSSH for Windows has no `nc` to call. Nothing is read or written there, and the section is not shown. Download sources work.
- **The system's Docker daemon on Linux.** Its proxy is `/etc/docker/daemon.json` or a systemd drop-in, both root's, and takes a restart of the daemon. Colima's, which bowel does connect, is a file of the user's.
- **Camera on Linux and Windows.** Video4Linux and Media Foundation are each a capture pipeline of their own; the screen and picture files cover the same use meanwhile.
- **System proxy on other Linux desktops** (sway, Hyprland, Xfce with no GSettings schema). There is no setting to write; programs there follow environment variables, which the terminal's connection under Developer tools sets.
- **A polkit action of bowel's own.** `pkexec` shows its generic text ("run /bin/sh as the super user") since bowel installs no policy file. Packaging one in the `.deb` and `.rpm` would let the dialog say what it is for.

## Privileged paths

The rules are the same on every system: the authorisation is asked once, it grants the narrowest thing that works, and what bowel changed is put back, also after a crash.

| | What root (or elevation) does, once | What it leaves | Undo |
| --- | --- | --- | --- |
| macOS TUN | `chown root:admin`, `chmod 4755`, `mv` | setuid core in the data directory | delete the file |
| Linux TUN | `chown root:root`, `chmod 0755`, `setcap`, `mv` | core with three network capabilities, running as the user | delete the file |
| Windows TUN | `mkdir`, `icacls`, `copy`, `schtasks /Create` | protected copy, task `bowel-tun-<SID>` | delete the task and the directory |
| macOS, Linux gateway | `install`, `visudo -c`, `mv` | `/etc/sudoers.d/bowel-gateway-<uid>`: two `sysctl` commands | delete the file |

On Windows the mark beside the copy is what `Installed` reads, so deleting the task alone leaves it: the app finds the task missing the next time it starts the core (`schtasks /Query`), runs the bundled core, and asks for the authorisation again when TUN is next turned on. The task's description names both things to delete.

At run time: forwarding as it was is kept in `gateway-before.json` and put back when the gateway goes off or on the next start; the system proxy as it was is kept in `sysproxy-before.json` and restored on quit; TUN's routes go with the core's process. On Windows a core that a crash of the app left running is ended (`schtasks /End`) before the next one starts.

## Not yet run on a real machine

Linux:

1. The polkit dialog and the whole `pkexec` installation, on a desktop session. (The commands' quoting ran in `sh`, and reading capabilities was checked against a file `setcap` really marked.)
2. The core opening a TUN device with file capabilities alone, with `auto-route`, on a real network; and the same on a home directory mounted `nosuid`, which should be refused.
3. System proxy on real GNOME and KDE sessions (tested against stand-ins for `gsettings` and `kwriteconfig6`), including whether running programs pick the change up.
4. Gateway mode end to end: the sudoers rule through real `sudo` (the rule passes real `visudo`), forwarding with TUN's rules in place, and the resolver on `0.0.0.0:53` beside systemd-resolved's stub on `127.0.0.53`.
5. Split DNS from `resolvectl` on a machine that runs systemd-resolved, and Tailscale's routes in table 52 (parsed from samples).
6. Screen scanning with each screenshot tool, on Wayland and X11; the tray.
7. `ping` (the test machine has none installed).

Windows, all of it on our side. A user's report on Windows 11 (issue 59, bowel 0.3.0) covers the first three for TUN: the UAC installation, the copy, its mark and the task were made as written; with the core able to read its files, the task started from the unelevated `schtasks /Run`, the adapter and routes came up, and `/End` took them down. That report is also where the fixes after 0.3.0 come from (files inside the core's directory, the bundled core as a fallback), and those have **not** been run on Windows. In the order they would fail:

1. The UAC prompt and the elevated command: `ShellExecuteExW`'s structure, cmd's quoting, `icacls` and `schtasks /Create /XML` with a UTF-16 definition.
2. Whether a task with an S4U principal and `HighestAvailable` starts from the user's unelevated `schtasks /Run`, runs elevated with no window, and ends with `/End`.
3. The core finding Wintun and opening the adapter from session 0.
4. Waiting on the elevated process (`OpenProcess(SYNCHRONIZE)`, else the process list) and the log through the controller.
5. Routes, adapters and the policy table read through the IP helper API and the registry; `IPV6_UNICAST_IF`; `PING.EXE -S`.
6. Screen capture through PowerShell, at display scales other than 100%.
7. A user who is not an administrator: the task's `HighestAvailable` is then no elevation, and TUN will not open.
