# Takeover, TUN and the exit lock

Takeover is how traffic gets into bowel. On the desktop there are two ways, switched in the takeover card on the home page (Use system proxy, Use TUN), and from the command line with `bowel on` and `bowel tun on`. On Android, takeover is the system VPN.

## System proxy or TUN

- **System proxy**: bowel points the system's proxy settings at itself. Browsers and most apps follow it. Command-line tools, SDKs, Docker and many games don't.
- **TUN**: a virtual network device that catches traffic from programs that ignore the system proxy. Use it when something still goes direct with the system proxy on.

On macOS and Linux, Developer tools also sets up Docker, the terminal and Git one by one: bowel lists which aren't connected, writes their own config in one click, and removes it cleanly when you undo.

### What TUN asks for

TUN asks for an administrator once, the first time you turn it on: macOS in its own dialog, Windows in a User Account Control prompt, Linux through polkit (`pkexec`).

On Linux with systemd-resolved, that authorization also leaves a polkit rule letting your user point the resolver at the TUN device, which the core does every time TUN starts. Without it (polkit older than 0.106, which reads no such rules) the desktop asks for a password three times at each start.

### Linux

The system proxy on Linux is GNOME's, which Cinnamon, MATE, Budgie and others share, or KDE's. On another desktop use TUN, or point your applications at the mixed port, 7899 by default.

## Exit lock

Some traffic should never leave from your own address, even for a moment: an AI service that refuses your region, for instance. Lock a route group's exit from its page on the board, and its traffic only goes through a node. When it can't, its connections are cut rather than sent over this machine's network:

- **With takeover off**: everything else goes direct as usual, and only the locked group is refused. The system proxy stays pointed at bowel for that.
- **When no node can be reached**: the group is refused, not sent direct. The group says so, with the way out: pick a node that works, or unlock it.
- **In global or direct mode**: the locked group still goes by its own node.

Once bowel quits, nothing is refused.

From the command line:

```bash
bowel route lock ai
```

### What the lock can hold

Without TUN, the exit lock only holds programs that follow the system proxy; command-line tools, SDKs and apps that ignore it still go direct. To hold everything, keep TUN on.

With only TUN on and TUN not working, the system proxy points at bowel for the time being, so the locked group is still held. If that fails too, the lock turns red and says it is not holding now, and what to do.

In the Docker image there is no system proxy. There the lock holds what comes in by bowel's ports and by TUN, whether takeover is on or off, and the rest isn't sent direct. A port of a locked group holds everything that uses it: see [Egress for other containers](../docker.md#egress-for-other-containers).

On Android the switch is on the group's page too. With the VPN off the phone can't refuse anything, so the first lock points you to Android's own VPN settings: Always-on VPN and Block connections without VPN, for bowel. That holds every app on the phone, not only the locked group, whenever the VPN is down.

## Gateway mode

On macOS and Linux, one computer can also take over for the TVs, consoles and phones on the LAN: [Gateway mode](../gateway.md).
