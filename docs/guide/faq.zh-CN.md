# 常见问题

## 从别的 Clash 客户端换过来

从 Clash Verge Rev、Mihomo Party、FlClash、ClashX Meta 或 Clash for Windows 换过来：在「提供商」里添加同一个订阅链接，或者直接导入配置文件。订阅或文件是一份自带路由组和规则的完整配置时，bowel 会问你是按它的路由分流，还是只用它的节点。

同一时间只应有一个客户端接管系统代理或 TUN，所以先退出原来的客户端。

## 支持哪些协议？

和 mihomo 内核一致：Shadowsocks、ShadowsocksR、VMess、VLESS（含 Reality）、Trojan、Hysteria 和 Hysteria2、TUIC、WireGuard、AnyTLS、Snell、Mieru、SOCKS5 和 HTTP，可以来自订阅、Clash 配置，或 `vless://`、`hy2://`、`trojan://` 这样的节点链接。

## 延迟最低的节点打不开 ChatGPT 或 Claude

回应快的节点，也可能正是这些服务拒绝的出口。「AI 服务」路由组按 OpenAI 和 Anthropic 的 API 测速，被拒绝的节点标为「被拒绝」，自动选择不会选它；想按 Claude 而不是 OpenAI 来选，把 Anthropic 的站点设为主目标。见[节点为什么显示「被拒绝」](routes.zh-CN.md#节点为什么显示被拒绝)。

## 节点延迟很低，下载却很慢

延迟只是节点回应小请求有多快。在「选择出口」里用「测下载」测真实的下载速度，再自己选节点：见[测下载](routes.zh-CN.md#测下载)。

## 有些程序还是直连

它们不读系统代理。打开 TUN，或者在 macOS 和 Linux 上用「开发工具」给它们设置好：见[系统代理还是 TUN](takeover.zh-CN.md#系统代理还是-tun)。

## bowel 会收集数据吗？

不会。没有遥测、没有统计，也不需要账号。订阅链接、节点和设置都保存在本机 bowel 的数据目录里，它们和你的流量都不会发给项目方。检查更新时访问的是本仓库的 Releases。

## bowel 是开源的吗？

源码公开，可以阅读，也可以自己编译。但它的许可证 [PolyForm Noncommercial License 1.0.0](../../LICENSE) 不是 OSI 认可的开源许可证：个人和其他非商业用途免费，商业用途（比如在公司里工作时使用）需要[商业授权](../../README.zh-CN.md#许可证)。

## 在哪里提问？

在 [Telegram](https://t.me/+BI3KVgBn7SE3YjVh) 群里，或者在 GitHub 上[提 issue](https://github.com/21stware/bowel/issues)。
