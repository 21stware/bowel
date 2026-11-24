# bowel docs

bowel is a [mihomo](https://github.com/MetaCubeX/mihomo) (Clash Meta) client for macOS, Windows, Linux and Android. These pages say how to use it; the [README](../../README.md) is the short version, and each of its sections links here for the rest.

They are also on the website, at [bowel.dev/en/docs/](https://bowel.dev/en/docs/). 中文版见 [README.zh-CN.md](README.zh-CN.md)。

## Guide

- [Install and get started](install.md): installing on each system, adding a provider, turning on takeover, and what each platform has
- [Route groups and nodes](routes.md): the built-in groups, choosing a node by hand or by speed, test sites, the download test, rules
- [Takeover, TUN and the exit lock](takeover.md): the system proxy and TUN, what TUN asks for on each system, locking a group's exit
- [Questions](faq.md): moving from another Clash client, protocols, data, the license

## Reference

- [Command line](../cli.md): the full-screen `bowel tui`, `bowel bar`, a port per route group, `--json` and exit codes, `bowel daemon`
- [Gateway mode](../gateway.md): one computer as the gateway of the LAN's TVs, consoles and phones, for people and agents

## For developers

These stay in the repository:

- [How it works and where the code is](../architecture.md)
- [What each system has, and how far each has been tried](../platform-parity.md)
- [What stays off the proxy: the LAN, Tailscale, VPNs](../dev-network.md)
- [Changelog](../../CHANGELOG.md), in Chinese

A page here changes with the feature it describes, in the same pull request. To fix one, follow the link at its foot to the file on GitHub.
