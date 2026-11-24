# Questions

## Coming from another Clash client

From Clash Verge Rev, Mihomo Party, FlClash, ClashX Meta or Clash for Windows: add the same subscription link under Providers, or import the config file. When a subscription or a file is a complete config with route groups and rules of its own, bowel asks whether to route by them or only use its nodes.

Only one client should take over the system proxy or TUN at a time, so quit the old one first.

## Which protocols does it support?

Those of the mihomo core: Shadowsocks, ShadowsocksR, VMess, VLESS (with Reality), Trojan, Hysteria and Hysteria2, TUIC, WireGuard, AnyTLS, Snell, Mieru, SOCKS5 and HTTP, from a subscription, a Clash config or node links such as `vless://`, `hy2://` and `trojan://`.

## The fastest node can't open ChatGPT or Claude

A node that answers fast can still be one the service refuses. The AI group tests against OpenAI's and Anthropic's APIs, marks a node they refuse as Denied, and leaves it out of the automatic choice; to choose by Claude rather than OpenAI, make Anthropic's site the main test site. See [Why a node shows Denied](routes.md#why-a-node-shows-denied).

## A node has a low delay but downloads slowly

Delay is how fast a node answers a small request. Use Downloads in Choose exit to measure real download speeds, then pick the node yourself: [Download test](routes.md#download-test).

## Some programs still go direct

They don't read the system proxy. Turn on TUN, or on macOS and Linux set them up under Developer tools: [System proxy or TUN](takeover.md#system-proxy-or-tun).

## Does bowel collect any data?

No. It has no telemetry, no analytics and no account. Subscription links, nodes and settings stay in bowel's data folder on your device, and nothing about them or your traffic is sent to the project. Updates are checked against this repository's releases.

## Is bowel open source?

Its source is public, and you can read it and build it. Its license, the [PolyForm Noncommercial License 1.0.0](../../LICENSE), is not an OSI open-source license, though: bowel is free for personal and other noncommercial use, and commercial use, such as work for a company, needs a [commercial license](../../README.md#license).

## Where do I ask?

In [Telegram](https://t.me/+BI3KVgBn7SE3YjVh), or [open an issue](https://github.com/21stware/bowel/issues) on GitHub.
