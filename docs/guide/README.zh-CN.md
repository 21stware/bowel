# bowel 文档

bowel 是 macOS、Windows、Linux 和 Android 上的 [mihomo](https://github.com/MetaCubeX/mihomo)（Clash Meta）客户端。这里写的是怎么用它；[README](../../README.zh-CN.md) 是简短的版本，它的每一节都链接到这里看详细的说明。

这些页面也在网站上：[bowel.dev/docs/](https://bowel.dev/docs/)。English: [README.md](README.md).

## 使用指南

- [安装与开始使用](install.zh-CN.md)：各系统的安装方法、添加提供商、打开接管，以及各平台有哪些功能
- [路由组与节点](routes.zh-CN.md)：内置的路由组、手动或按速度选节点、测速站点、测下载、规则
- [接管、TUN 与出口锁定](takeover.zh-CN.md)：系统代理和 TUN、各系统上 TUN 要的授权、锁定一个路由组的出口
- [常见问题](faq.zh-CN.md)：从别的 Clash 客户端换过来、支持的协议、数据、许可证

## 参考

- [命令行](../cli.zh-CN.md)：全屏的 `bowel tui`、`bowel bar`、每个路由组一个端口、`--json` 和退出码、`bowel daemon`
- [网关模式](../gateway.md)（英文）：让一台电脑做局域网里电视、游戏机和手机的网关，写给人也写给 Agent

## 写给开发者

这些留在仓库里：

- [工作原理和代码在哪](../architecture.md)（英文）
- [各系统有什么、各自试到了什么程度](../platform-parity.md)（英文）
- [哪些不走代理：局域网、Tailscale、VPN](../dev-network.md)（英文）
- [更新日志](../../CHANGELOG.md)

文档跟着它写的功能一起改，放在同一个 PR 里。要修正哪一页，点页面底部的链接到 GitHub 上的源文件。
