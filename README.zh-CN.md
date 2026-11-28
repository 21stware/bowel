<p align="center">
  <img src="site/icon.png" width="112" alt="bowel">
</p>

<h1 align="center">bowel</h1>

<p align="center">
bowel 是 macOS、Windows、Linux 和 Android 上的 <a href="https://github.com/MetaCubeX/mihomo">mihomo</a>（Clash Meta）客户端，原生应用，不基于 Electron，也不用 WebView。
</p>

<p align="center">
  <a href="https://github.com/21stware/bowel/releases/latest"><b>下载</b></a> ·
  <a href="https://bowel.dev/">官网</a> ·
  <a href="https://bowel.dev/docs/">文档</a> ·
  <a href="https://t.me/+BI3KVgBn7SE3YjVh">Telegram 群</a> ·
  <a href="README.md">English</a> ·
  <a href="README.tr.md">Türkçe</a>
</p>

<p align="center">
  <a href="https://github.com/21stware/bowel/releases/latest"><img src="https://img.shields.io/github/v/release/21stware/bowel?label=release" alt="最新版本"></a>
  <a href="https://github.com/21stware/bowel/releases"><img src="https://img.shields.io/github/downloads/21stware/bowel/total?label=downloads" alt="下载量"></a>
</p>

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="site/shots/home-zh-dark.webp">
    <img src="site/shots/home-zh.webp" width="880" alt="bowel 主窗口">
  </picture>
</p>

桌面版界面使用 Go 开发并通过 GPU 渲染，不使用 WebView。Android 版使用 Flutter 开发，卡片和皮肤与桌面版相同。

## 有什么不同

- **按用途分配节点。** AI 服务、流媒体、开发等各是一条路由，各用各的节点，可以手选，也可以按测速自动选。
- **Clash 配置直接用。** 订阅链接、节点链接、二维码，以及完整的 Clash 或 mihomo 配置都能导入，完整配置可以沿用它自带的路由组和规则。
- **为开发者的电脑做的。** macOS 和 Linux 上，Docker、终端和 Git 一键接入代理；npm、pip、Go 和 Cargo 的镜像源实测后一键切换；局域网、Tailscale 和公司 VPN 自动直连。
- **命令行、TUI 和 daemon。** `bowel` 命令操作运行中的应用，`--json` 方便写脚本；`bowel tui` 通过 SSH 也能用；`bowel daemon` 不开窗口运行。
- **给 Agent 用的 MCP 服务器。** Claude Code 等 Agent 可以读取状态、排查连接，改动之前先问你。
- **网关模式。** 一台 Mac 或 Linux 电脑给局域网里的电视和游戏机当旁路由。

## 界面与功能

以下截图来自 macOS 版。

### 路由
按用途分配不同路由，可手动指定出口节点或由自动测速选择。路由组、测速站点和测下载怎么用，见[路由组与节点](https://bowel.dev/docs/routes/)。

<img src="site/shots/routes-zh.webp" width="880" alt="路由">

### 连接
查看每个连接的应用、目标、规则、出口和速率，也可以直接修改指定域名的路由。

<img src="site/shots/activity-zh.webp" width="880" alt="连接日志">

### 提供商
支持添加订阅链接、节点链接、Clash 或 mihomo 的 YAML 配置，或扫码导入，并显示流量与到期时间。

<img src="site/shots/providers-zh.webp" width="880" alt="提供商">

### 菜单栏
支持在菜单栏（Windows 和 Linux 上是系统托盘）开关接管状态、切换运行模式和「其他流量」的出口节点。Omarchy 上要在顶栏的托盘里固定（pin）bowel 的图标，否则它会折叠在托盘的抽屉里。

<img src="site/shots/panel-zh.webp" width="300" alt="菜单栏面板">

### 主题皮肤
十六种皮肤，换的是整个界面的样子。

<table>
  <tr>
    <td><img src="site/shots/skin-broadsheet-zh.webp" alt="报刊"><br><sub>报刊</sub></td>
    <td><img src="site/shots/skin-phosphor-zh.webp" alt="荧光屏"><br><sub>荧光屏</sub></td>
    <td><img src="site/shots/skin-neon-zh.webp" alt="霓虹"><br><sub>霓虹</sub></td>
  </tr>
  <tr>
    <td><img src="site/shots/skin-blueprint-zh.webp" alt="蓝图"><br><sub>蓝图</sub></td>
    <td><img src="site/shots/skin-sakura-zh.webp" alt="樱"><br><sub>樱</sub></td>
    <td><img src="site/shots/skin-paper-zh.webp" alt="纸墨"><br><sub>纸墨（默认）</sub></td>
  </tr>
  <tr>
    <td><img src="site/shots/skin-shanshui-zh.webp" alt="山水"><br><sub>山水</sub></td>
    <td><img src="site/shots/skin-celadon-zh.webp" alt="青瓷"><br><sub>青瓷</sub></td>
    <td><img src="site/shots/skin-dunhuang-zh.webp" alt="敦煌"><br><sub>敦煌</sub></td>
  </tr>
  <tr>
    <td><img src="site/shots/skin-dusk-zh.webp" alt="暮色"><br><sub>暮色</sub></td>
    <td><img src="site/shots/skin-aurora-zh.webp" alt="极光"><br><sub>极光</sub></td>
    <td><img src="site/shots/skin-obsidian-zh.webp" alt="黑曜"><br><sub>黑曜</sub></td>
  </tr>
  <tr>
    <td><img src="site/shots/skin-riso-zh.webp" alt="孔版"><br><sub>孔版</sub></td>
    <td><img src="site/shots/skin-mrrobot-zh.webp" alt="黑客军团"><br><sub>黑客军团</sub></td>
    <td><img src="site/shots/skin-machine-zh.webp" alt="The Machine"><br><sub>The Machine</sub></td>
  </tr>
  <tr>
    <td><img src="site/shots/skin-samaritan-zh.webp" alt="Samaritan"><br><sub>Samaritan</sub></td>
  </tr>
</table>

### 其他
- **自定义规则**：支持按域名、关键词、IP 段或 GeoSite / GeoIP 列表添加分流规则并测试匹配结果，也可以直接粘贴 mihomo 写法的规则；macOS、Windows、Linux 和 Android 10 及以上还可以按应用分流：在路由组里添加一条「应用」规则，这个应用的所有连接都走这个路由组。
- **出口位置实测**：通过 Cloudflare 请求实测节点真实的出口所在地。
- **网关模式**（macOS、Linux）：支持作为局域网网关（旁路由）为电视、游戏机和手机提供代理服务。完整说明见 [docs/gateway.md](docs/gateway.md)，人和 agent 都可以照着做。
- **TUN**（macOS、Windows、Linux）：接管不遵循系统代理设置的应用程序流量。
- **开发工具**（macOS、Linux）：Docker、终端和 Git 不读系统代理，bowel 列出哪些没有接入，点一下写好它们自己的配置，撤销时原样删除。
- **下载源**：实测 npm、pip、Go 和 Cargo 的官方源与各个镜像，点一下换源；当前的源连不上时，一键换到最快的那个，或让它重新走节点。
- **内网直连**：局域网、Tailscale 和公司 VPN 的地址自动直连，全局模式下也一样。
- **出口锁定**：在首页路由组的页面上锁定一个路由组（比如 AI 服务），它的流量就只走节点，节点都走不了时直接断开，不会改走直连；全局和直连模式下、关闭接管后也一样。退出 bowel 后不再拦截。也可以用 `bowel route lock ai`。开不开 TUN 时各管得住什么、Android 上怎么用，见[接管、TUN 与出口锁定](https://bowel.dev/docs/takeover/#出口锁定)。
- **MCP**：内置 MCP 服务器，供 Claude Code 等 Agent 读取状态、排查网络问题，改设置前默认先问你；只有你能让 Agent 不经询问就修改：在 设置 > MCP 里，或在你自己的终端里执行 `bowel settings set mcpPolicy allow` 并输入 `allow` 确认。下面是 macOS 的命令，Windows 和 Linux 上请换成 bowel 程序所在的路径。
  ```bash
  claude mcp add --scope user bowel -- /Applications/bowel.app/Contents/MacOS/bowel mcp
  ```
- **设置与语言**：设置项支持搜索（⌘F），支持简体中文、繁體中文、English 与日本語，支持浅色与深色。

## 安装

macOS，使用 [Homebrew](https://brew.sh)：

```bash
brew install --cask 21stware/tap/bowel
```

Windows，在 PowerShell 中运行：

```powershell
irm https://github.com/21stware/bowel/releases/latest/download/install.ps1 | iex
```

Debian、Ubuntu 和 Fedora。这条命令会添加一个已签名的软件源，之后由 `apt` 或 `dnf` 更新 bowel：

```bash
curl -fsSL https://21stware.github.io/packages/install.sh | sh
```

没有屏幕的服务器、NAS 或软路由，用 Docker（amd64、arm64、armv7）：镜像是 `ghcr.io/21stware/bowel`，Docker Hub 上叫 `oboochin/bowel`，NAS 界面里搜得到。之后用 `docker exec -it bowel bowel tui` 管理；TUN、网关模式、compose 文件、群晖和 Unraid，以及给 agent 用的 MCP 见 [docs/docker.zh-CN.md](docs/docker.zh-CN.md)：

```bash
docker run -d --name bowel --restart unless-stopped -p 7899:7899 -v bowel:/data \
  -e BOWEL_SUBSCRIPTION='<订阅地址>' ghcr.io/21stware/bowel
```

也可以从 [Releases](https://github.com/21stware/bowel/releases/latest) 下载对应平台的安装包：

| 平台 | 文件 | 要求 |
| --- | --- | --- |
| macOS | `bowel-<版本>-arm64.dmg` | macOS 12 或更新版本，Apple 芯片 |
| Windows | `bowel-setup-<版本>-amd64.exe` 或 `-arm64.exe` | x64 或 Arm64 |
| Linux | `bowel_<版本>_<架构>.deb`、`bowel-<版本>-1.<rpm 架构>.rpm` 或 `bowel-<版本>-linux-<架构>.tar.gz` | amd64 或 arm64 |
| Android | `bowel-<版本>-android-arm64.apk`，32 位电视盒子用 `-armv7.apk` | Android 8.0 或更新版本，arm64 或 armv7 |

- **macOS**：把 bowel 拖进「应用程序」。安装包用 Developer ID 签名，并经过 Apple 公证。
- **Windows**：运行安装程序。安装包没有代码签名，运行前 Windows 可能会弹出提示。
- **Linux**：安装 `.deb` 或 `.rpm`。要安装到当前用户的 `~/.local`，从同一个 Release 下载 `install.sh` 和 `.tar.gz`，不用解压，运行 `sh install.sh bowel-<版本>-linux-<架构>.tar.gz`。这个脚本与上面添加软件源的脚本不同。
- **Android**：安装 APK，如有提示请允许浏览器安装应用。首次连接时系统会请求 VPN 权限。
- **iOS**：暂未发布。

打开后添加一个 Provider（订阅链接、节点链接或扫码），打开接管：见[安装与开始使用](https://bowel.dev/docs/install/#开始使用)。

部分功能因平台而异：

| | macOS | Windows | Linux | Android |
| --- | --- | --- | --- | --- |
| 接管方式 | 系统代理或 TUN | 系统代理或 TUN | 系统代理（GNOME、KDE）或 TUN | 系统 VPN |
| 网关模式 | 支持 | 不支持 | 支持 | 不支持 |
| 开发工具 | 支持 | 不支持 | 支持（zsh、bash） | 不支持 |
| 扫描屏幕上的二维码 | 支持 | 支持 | 支持 | 不支持 |
| 用摄像头扫描二维码 | 支持 | 不支持 | 不支持 | 支持 |
| MCP 服务器 | 支持 | 支持 | 支持 | 不支持 |
| 命令行和 `bowel daemon` | 支持 | 支持 | 支持 | 不支持 |
| 按 App 分流（规则） | 支持 | 支持，我们未在真机上运行过 | 支持 | Android 10 及以上 |
| 不走 bowel 的 App（排除列表） | 没有 | 没有 | 没有 | 支持，重新连接后生效 |

按 App 分流是路由组里的一种规则（「应用」）；Android 上的排除列表是另一回事：列表里的 App 完全不经过 VPN，规则也管不到它。两者见[路由组与节点](https://bowel.dev/docs/routes/#按-app-分流)。

第一次开启 TUN 时需要管理员授权一次。各系统上要什么授权、Linux 各桌面的系统代理，见[接管、TUN 与出口锁定](https://bowel.dev/docs/takeover/)；各系统底下具体有什么、缺什么，见 [docs/platform-parity.md](docs/platform-parity.md)。

## 常见问题

更多问题，比如延迟最低的节点为什么打不开 ChatGPT，见[文档](https://bowel.dev/docs/faq/)。

**从 Clash Verge Rev、Mihomo Party、FlClash、ClashX Meta 或 Clash for Windows 换过来？** 在 Provider 里添加同一个订阅链接，或者直接导入配置文件。订阅或文件是一份自带路由组和规则的完整配置时，bowel 会问你是按它的路由分流，还是只用它的节点。同一时间只应有一个客户端接管系统代理或 TUN，所以先退出原来的客户端。

**支持哪些协议？** 和 mihomo 内核一致：Shadowsocks、ShadowsocksR、VMess、VLESS（含 Reality）、Trojan、Hysteria 和 Hysteria2、TUIC、WireGuard、AnyTLS、Snell、Mieru、SOCKS5 和 HTTP，可以来自订阅、Clash 配置，或 `vless://`、`hy2://`、`trojan://` 这样的节点链接。

**bowel 会收集数据吗？** 不会。没有遥测、没有统计，也不需要账号。订阅链接、节点和设置都保存在本机 bowel 的数据目录里，它们和你的流量都不会发给项目方。检查更新时访问的是本仓库的 Releases。

**bowel 是开源的吗？** 源码公开，可以阅读，也可以自己编译。但它的许可证 [PolyForm Noncommercial License 1.0.0](LICENSE) 不是 OSI 认可的开源许可证：个人和其他非商业用途免费，商业用途（比如在公司里工作时使用）需要[商业授权](#许可证)。

## 命令行

同一个程序也是命令行，用来操作运行中的 bowel。macOS 上用 Homebrew 安装时，`bowel` 已经在 `PATH` 里；否则它在 `/Applications/bowel.app/Contents/MacOS/bowel`，可以链接到 `PATH` 里（`ln -s /Applications/bowel.app/Contents/MacOS/bowel /usr/local/bin/bowel`），或直接写完整路径。不带命令运行时，和以前一样打开应用。

```bash
bowel status                      # 状态灯、模式、接管、各路由的节点、流量
bowel on                          # 开始接管（系统代理）；bowel off 恢复原样
bowel mode global                 # rule | global | direct
bowel routes                      # 路由和各自使用的节点
bowel route use ai auto           # 给路由选节点，或 auto 自动
bowel nodes --test                # 重新测速后列出节点
bowel providers add <地址|文件>    # 还有 list、update、remove、enable、disable
bowel rules add openai.com ai     # 还有 list、remove、test <地址>
bowel conns --watch               # 持续显示新连接和结束的连接
bowel logs -f --level warning     # 持续输出内核日志
bowel settings set logLevel debug # 还有 list、get
bowel tun on                      # 以及 bowel gateway on
bowel tui                         # 全屏界面
bowel bar                         # 给状态栏每秒输出一行
```

每个命令都有 `--help` 和例子，读取类命令支持 `--json`。全屏界面 `bowel tui`、给 waybar 等状态栏用的 `bowel bar`、每个路由组一个端口、退出码、不开窗口运行的 `bowel daemon`，以及 agent 能做什么，见 [docs/cli.zh-CN.md](docs/cli.zh-CN.md)。

## 开发

需要 Go 1.27.3+（`GOTOOLCHAIN=auto` 会自动下载）。不需要 Node 或 Bun。

```bash
./scripts/fetch-core.sh     # 下载 mihomo 内核和 GeoSite/GeoIP 数据到 resources/
go tool mygo dev            # 运行，改代码自动重建
go tool mygo build          # 打包 bowel.app 和 .dmg（已签名，未公证）
./scripts/release.sh        # 打包、公证并装订，发版用这个
CGO_ENABLED=0 go test ./... # 包含会启动真实内核的测试
./scripts/site-shots.sh     # 重新生成 README 和官网的截图（需要 cwebp）
```

在任意桌面系统上都可以构建 Windows 或 Linux 版，先下载对应平台的内核。Windows 安装程序还需要 NSIS。

```bash
GOOS=linux GOARCH=amd64 ./scripts/fetch-core.sh
go tool mygo build -platform linux/amd64 .
```

移动端代码在 `mobile/`，需要 Flutter 和 gomobile，Android 还需要 Android SDK 和 NDK：

```bash
./mobile/android/build.sh       # Android：mihomo、启动器和桥接层
./mobile/bridge/build.sh        # iOS：桥接层和隧道
go generate ./...               # 从桌面端生成 App 的翻译和皮肤
cd mobile/flutter && flutter run
```

Flutter App 的翻译表和皮肤由桌面端的 Go 代码生成，过期时 `go test` 会失败。桌面端没有的、App 自己的文案在 `internal/i18n/flutter.go`。

- 工作原理与代码结构：[docs/architecture.md](docs/architecture.md)
- 产品信息架构：[docs/product-ia.md](docs/product-ia.md)
- 官网是 `site/` 下的静态页面，直接打开 `site/index.html` 即可预览。网站上的文档由 `docs/guide/`（以及 `docs/cli*.md`、`docs/gateway.md`）里的 Markdown 生成：`pip install -r scripts/site-requirements.txt` 后运行 `python3 scripts/build-site.py`，整个网站写到 `build/site/`。合入 `main` 后自动上线。改了用法的 PR，同时改 `docs/guide/` 里对应的页面，中英文都改。
- 发版时先改 `mygo.json` 里的版本号，再推送对应的标签（如 `v0.2.0`）。GitHub Actions 会构建 macOS、Windows、Linux 和 Android 版，并发布到同一个 Release。iOS 版只在手动运行工作流时构建，不会发布。

bowel 原名 Moho。Go 模块仍是 `github.com/oboo/moho`，Bundle ID 仍是 `dev.moho.app`（保留 macOS 已授予的摄像头和录屏权限）。首次启动时，`~/Library/Application Support/Moho` 里的数据会迁移到 `…/bowel`。

## 致谢

- [mihomo](https://github.com/MetaCubeX/mihomo)：代理内核，GPL-3.0。已发布的版本把它作为独立进程运行，不链接它的代码。iOS 必须链接它，因此 iOS 版暂未发布。
- [MyGo](https://github.com/egoist/mygo)：原生 Go UI 框架。
- [Flutter](https://flutter.dev)：移动端界面框架。
- [OPPO Sans](resources/fonts/)：桌面版的界面字体。手机版用系统字体。

## 贡献者

<a href="https://github.com/21stware/bowel/graphs/contributors"><img src="https://contrib.rocks/image?repo=21stware/bowel" alt="贡献者"></a>

## 许可证

bowel 采用 [PolyForm Noncommercial License 1.0.0](LICENSE) 授权，仅限非商业用途。如需商业授权，请[提交 issue](https://github.com/21stware/bowel/issues)。

Linux 发行版、软件仓库和应用商店可以依照[打包豁免](PACKAGING-EXCEPTION.md)从源码编译并分发 bowel，分发方是否为商业机构都可以。最终用户拿到的 bowel 仍仅限非商业用途。豁免以英文原文为准。

随应用打包的 mihomo 内核仍适用 GPL-3.0。桌面版的资源目录里附有它的许可证（macOS 上是 `Contents/Resources/licenses/mihomo/`），Android 版在设置中注明，每个 Release 都附带对应版本的源码。`assets/flags/` 中的旗帜图片和 `resources/fonts/` 中的字体属于第三方文件，仍适用各自的许可证。

## Star 趋势

<a href="https://star-history.com/#21stware/bowel&Date">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=21stware/bowel&type=Date&theme=dark">
    <img src="https://api.star-history.com/svg?repos=21stware/bowel&type=Date" width="600" alt="Star 趋势">
  </picture>
</a>
