# Docker

这个镜像就是 `bowel daemon` 加上旁边的 mihomo 内核，给没有屏幕的服务器、NAS、软路由和开发机用。它没有窗口，也没有网页：在容器里运行 `bowel tui` 就是界面，其余交给命令行，agent 通过 MCP 接入。支持 amd64、arm64 和 armv7，下载约 40 MB，里面没有 shell 和包管理器。

```bash
docker run -d --name bowel --restart unless-stopped \
  -p 7899:7899 -v bowel:/data \
  -e BOWEL_SUBSCRIPTION='https://example.com/your-subscription' \
  ghcr.io/21stware/bowel
```

然后让程序走 `http://<主机>:7899`（HTTP 和 SOCKS5 共用这个端口），并打开界面：

```bash
docker exec -it bowel bowel tui
```

`docker exec bowel bowel status`、`bowel nodes`、`bowel route use ai <节点>`、`bowel logs -f` 等所有命令都这样用，见 [cli.md](cli.md)。可以设个别名少打几个字：`alias bowel='docker exec -it bowel bowel'`。

## 镜像

| 标签 | 内容 |
| --- | --- |
| `ghcr.io/21stware/bowel:latest` | 最新版本 |
| `ghcr.io/21stware/bowel:0.4` | 最新的 0.4.x |
| `ghcr.io/21stware/bowel:0.4.9` | 某一个版本；镜像从 0.4.9 开始才有 |

Docker Hub 上也有同样的标签，叫 `oboochin/bowel`（`docker pull oboochin/bowel`）。群晖的 Container Manager 等 NAS 界面默认在 Docker Hub 上搜索；本文的命令用哪个名字都可以。两边都是公开的，拉取不需要 `docker login`。

amd64 镜像用的是 mihomo 的 `v1` 构建，任何 x86-64 CPU 都能运行，包括很多 NAS 用的 Atom 和赛扬。每个版本发布之后才推送镜像；镜像不会自己更新，拉取新标签即可更新（`docker compose pull && docker compose up -d`），数据卷原样保留。

## Compose

[docker/compose.yaml](docker/compose.yaml) 是最常见的用法，给局域网当代理：

```yaml
services:
  bowel:
    image: ghcr.io/21stware/bowel
    container_name: bowel
    restart: unless-stopped
    ports:
      - "7899:7899"
      - "127.0.0.1:7900:7900" # 给本机 agent 用的 MCP；去掉 127.0.0.1: 就对局域网开放
    volumes:
      - bowel:/data
    environment:
      BOWEL_SUBSCRIPTION: https://example.com/your-subscription
      TZ: Asia/Shanghai
volumes:
  bowel:
```

[docker/compose.tun.yaml](docker/compose.tun.yaml) 在主机网络上运行 TUN，接管主机自己的流量，或者配合网关模式使用（见下文）。

## NAS

**群晖**（Container Manager，DSM 7.2 或更新版本）。在「项目」里「创建」一个项目，路径选一个单独的文件夹，用编辑器创建 `docker-compose.yml`，粘贴 [docker/compose.yaml](docker/compose.yaml) 并填上你的订阅。不用 compose 的话：在「镜像仓库」里搜索 `oboochin/bowel`（默认就在 Docker Hub 上搜）并「下载」，再到「映像」里选中它、点「运行」：本地端口 7899 对应容器的 7899，把一个文件夹挂载到 `/data`，环境变量里填 `BOWEL_SUBSCRIPTION` 和 `TZ`。镜像里没有 shell，界面要通过 SSH 打开：`sudo docker exec -it bowel bowel tui`。

**Unraid。** [docker/unraid.xml](docker/unraid.xml) 是 Docker 页面用的模板：代理端口、放在 `/mnt/user/appdata/bowel` 的 `/data`、订阅，以及谁能用代理。Unraid 把容器的模板放在 `/boot/config/plugins/dockerMan/templates-user`；在 Unraid 的终端里把它存到那里，再从「Apps」的「Previous Apps」安装（需要 Community Applications）：

```bash
curl -fsSL -o /boot/config/plugins/dockerMan/templates-user/my-bowel.xml \
  https://raw.githubusercontent.com/21stware/bowel/main/docs/docker/unraid.xml
```

界面是在 Unraid 的终端里运行 `docker exec -it bowel bowel tui`；容器菜单里的「Console」没有 shell 可开。要让别的机器上的 agent 用 MCP，用「Add another Path, Port, Variable, Label or Device」加上 7900 端口。要用 TUN：「Network Type」选「Host」，在「Extra Parameters」（「Advanced View」里）加上 `--cap-add=NET_ADMIN`，把 `/dev/net/tun` 加为「Device」，并把变量 `BOWEL_MCP_HTTP` 设成 `127.0.0.1:7900`（在主机网络上，7900 就是服务器自己的端口）；然后见 [TUN 和网关模式](#tun-和网关模式)。

## 设置

容器通过环境变量配置。第一次启动时会写入适合容器的设置：代理对私有地址开放（10/8、172.16/12、192.168/16、127/8、fc00::/7），不设系统代理，不查进程，语言跟随 `LANG`（没有就用英文）。`/data` 里已经有 `settings.json` 时不会改动它。下面这些变量每次启动都会生效，并写进设置：

| 变量 | 作用 |
| --- | --- |
| `BOWEL_SUBSCRIPTION` | 订阅地址或节点链接，每行一个，或用逗号分隔。还没有对应 Provider 的会被添加，下载失败会重试几次。日志里只写 Provider 的 ID 和域名，不写地址本身。 |
| `BOWEL_MIXED_PORT` | 容器内的代理端口（7899）。 |
| `BOWEL_ALLOW_LAN` | 设为 `false` 时只给容器自己用，比如在主机网络上跑 TUN。 |
| `BOWEL_LAN_ALLOWED_IPS` | 允许使用端口的地址，CIDR，用逗号或空格分隔。用 `-p` 映射时，Docker 会把客户端的真实地址交给容器，所以在有公网地址的服务器上，互联网上的客户端在列进来之前都会被拒绝；同时请设置 `authentication`（`bowel settings set authentication '["用户:密码"]'`）。 |
| `BOWEL_MCP_HTTP` | MCP over HTTP 监听的地址（`:7900`）；留空则关闭。见 [Agent](#agentmcp)。 |
| `BOWEL_MCP_TOKEN` | 自己指定 MCP 令牌，代替第一次启动时生成的那个。 |
| `TZ` | 日志时间和统计按天划分所用的时区。 |

其他都是设置项，用 `bowel settings set <key> <值>` 修改（`bowel settings list` 列出全部），也可以在 TUI 的设置页里改，或者交给 agent。比如 `bowel settings set language zh` 换成中文。设置保存在 `/data/settings.json`，和 Provider、规则、统计放在一起；请把 `/data` 放在数据卷上。容器默认以 root 运行；用 `--user` 换成别的用户时，这个用户要拥有 `/data`。

## TUN 和网关模式

代理端口不需要额外权限。TUN 会接管它所在网络的全部流量，需要 TUN 设备和配置网络的权限：

```bash
docker run -d --name bowel --restart unless-stopped \
  --network host --cap-add NET_ADMIN --device /dev/net/tun \
  -v bowel:/data ghcr.io/21stware/bowel
docker exec bowel bowel tun on
```

在主机网络上，TUN 接管主机自己的流量；再执行 `bowel gateway on`，主机就成了局域网的路由器：把网关和 DNS 设成它的设备都会经过 bowel。网关模式需要主机网络（它在物理网卡上服务局域网，而不是 Docker 的网桥），还需要 IP 转发。Docker 本来就会在主机上打开转发；如果没开，请在主机上执行 `sysctl -w net.ipv4.ip_forward=1`，因为容器里的 `/proc/sys` 是只读的。如果用 Docker 的网桥网络，TUN 接管的是这个容器，以及共用它网络的容器（`--network container:bowel`）的流量，可以用来让某一个服务单独走 bowel。缺少设备或权限时，`bowel tun on` 会说明缺的是哪一个。Docker Desktop 的主机网络是它虚拟机的网络，所以在 macOS 和 Windows 上请直接用 App。

## Agent（MCP）

镜像在 7900 端口提供 MCP over HTTP，用令牌保护。令牌在第一次启动时生成，保存在 `/data/mcp-token`。下面这条命令会打印令牌，以及把 bowel 加到 agent 里的配置：

```bash
docker exec bowel bowel mcp token
```

```bash
claude mcp add --transport http bowel http://<主机>:7900/mcp --header "Authorization: Bearer <令牌>"
```

其他客户端用同样的 URL 和请求头（`"type": "http"`、`"url"`、`"headers"`）。`BOWEL_MCP_TOKEN` 可以指定自己的令牌（至少 16 个字符）；`BOWEL_MCP_HTTP` 留空则关闭这个端点。它是明文 HTTP，令牌也明文传输：只在 `127.0.0.1`、VPN 或内网上发布 7900，或者放在保留 `Host` 请求头的 TLS 反向代理后面。

容器里没有窗口可以弹出询问，所以在默认的「询问」策略下，经 MCP 连接的 agent 能读取一切，但改不了任何东西。要让 agent 能修改，你自己执行 `docker exec -it bowel bowel settings set mcpPolicy allow`，在它询问时输入 `allow`（也可以在 `bowel tui` 的设置页里选）。不带 `-it` 会被拒绝：`docker exec` 带进去的是容器的环境变量，不是调用者的，所以 bowel 分不清 agent 的 `docker exec` 和你的，只有你在终端前能回答。也因为这样，agent 用 `docker exec bowel bowel …` 运行的命令会被当成你的，不会被询问；要让 agent 的修改遵循策略，就让它用 MCP。同一台主机上的 agent 也可以运行 `docker exec -i bowel bowel mcp`，也就是 stdio 版本的 MCP，不需要令牌。

## 健康检查和日志

镜像的健康检查是 `bowel health`：daemon 有响应、内核在运行、端口能连上时为健康。容器的端口不论开不开 TUN 都在承载流量，所以 `bowel status` 把容器当作正在接管来判断：内核出错、端口打不开、锁定的路由组因为没有节点而拦下所有连接，都会写在第一行和 `--json` 的 `issue` 里。`docker logs bowel` 是 daemon 的日志；内核的日志用 `docker exec bowel bowel logs -f` 查看。`docker stop` 会干净地停止它。

## 给其他容器当出口

其他容器和局域网里的机器都可以经过 bowel 出去，并且自己选出口。有三种接入方式：

- **代理端口 7899。** 和其他客户端一样，流量按规则走。
- **每个路由组一个端口。** `BOWEL_EXTRA_PORTS: 7901=ai 7902=media` 会打开 7901 和 7902，同时支持 HTTP 和 SOCKS5（包括 UDP）。从哪个端口进来，流量就固定走对应的路由组，不看规则，也不受模式影响。在容器里，每个额外端口都和 7899 一样开放：只接受 `BOWEL_LAN_ALLOWED_IPS` 里的网段（一开始是私有地址），设置了 `authentication` 就要密码。所以 `-p 7901:7901` 之后局域网就能用，其他容器用 `bowel:7901` 访问。
- **TUN，给共用 bowel 网络的容器**（`network_mode: service:bowel`）。它的每个连接都经过 bowel，不看代理变量的程序也一样，再由规则分流。

出口锁定就是断网保护（kill switch）：锁定的路由组只走自己的节点，走不了就断开，绝不直连。节点都不通、订阅还没下载下来、节点被删掉时都是如此。能管住多少取决于接入方式：通过锁定路由组的端口，从这个端口进来的流量全部管住；在 TUN 下，管住的是规则分到锁定路由组的流量。默认路由不能锁定，直连路由按设计直连（国内网站和局域网）。容器里没有系统代理，所以不管接管是开还是关，锁定都一样生效。

这些都可以在每次启动时用环境变量设置，一个 compose 文件就够了：

| 变量 | 作用 |
| --- | --- |
| `BOWEL_EXTRA_PORTS` | 额外端口，写成 `7901=ai 7902=media`（端口=路由 ID，用空格或逗号分隔；`bowel routes` 列出 ID），也可以直接写这项设置的 JSON。`none` 关闭全部。 |
| `BOWEL_LOCK` | 要锁定出口的路由组 ID（`ai dev`），其余的会解锁。如果有一项不是能锁定的路由组（`default`、`direct`、`reject` 不能锁定），能锁的照样锁上，但不会解锁任何路由组，写错一个字也不会放松锁定。`none` 全部解锁。 |
| `BOWEL_TUN` | `true` 打开 TUN，和 `bowel tun on` 一样。需要 `NET_ADMIN` 和 `/dev/net/tun`。 |
| `BOWEL_SETTINGS` | 其他设置，写成以设置键为键的 JSON 对象（`{"sniff": true}`），检查方式和 `bowel settings set` 相同。它最先应用，所以上面几个变量优先。不能设置 `mcpPolicy`。 |

每个变量单独应用、单独保存：某一项被设置拒绝（比如端口被占用）只会记一行日志，其他的照常生效。

### 让 Agent 只经过 bowel 出去

[docker/compose.agent.yaml](docker/compose.agent.yaml) 让 Agent、开发容器或 CI 执行器使用 bowel 的网络，打开 TUN，并锁定 `ai` 和 `dev`：

```yaml
services:
  bowel:
    image: ghcr.io/21stware/bowel
    cap_add: [NET_ADMIN]
    devices: [/dev/net/tun:/dev/net/tun]
    dns: [1.1.1.1]
    volumes: [bowel:/data]
    environment:
      BOWEL_SUBSCRIPTION: https://example.com/your-subscription
      BOWEL_TUN: "true"
      BOWEL_LOCK: ai dev
      BOWEL_SETTINGS: '{"sniff": true}'
  agent:
    image: node:22-bookworm
    network_mode: service:bowel
    depends_on:
      bowel: {condition: service_healthy}
```

Agent 调用的 API（`ai`），以及它访问 GitHub、npm、PyPI 和 Docker Hub 的流量（`dev`），都只走节点，走不了就断开。`docker exec bowel bowel logs` 会显示每个连接走了哪个路由组、哪个节点。

锁定是按规则生效的，所以规则得看到域名，下面两行就是为此准备的。`dns` 让 Docker 的解析器经过 TUN 去问这个服务器，由 bowel 来回答；不加的话由宿主机的解析器回答，bowel 只能看到 IP，API 调用就匹配不到 `ai` 的规则，转而走默认路由。`sniff` 从流量里读出域名，照顾那些自己解析域名的程序。没有命中任何规则的流量走默认路由：有节点就走节点，没有节点就直连。如果容器的每个连接都必须走节点，请改用锁定路由组的端口，再配合内部网络，见下一个例子。

共用 bowel 网络的容器也共用它的端口：Agent 可以在 `http://127.0.0.1:7900/mcp` 访问 bowel 的 MCP（令牌用 `docker exec bowel bowel mcp token` 查看），额外端口在 `127.0.0.1:<端口>`。

### 下载器只能经过 bowel 出去

[docker/compose.downloads.yaml](docker/compose.downloads.yaml) 把 qBittorrent 放在一个内部网络里，网络里只有它和 bowel，所以它没有出去的路由，也解析不了外部域名。它的代理是 bowel 的 7902 端口，这个端口固定走 `media`，并且已锁定：

```yaml
services:
  bowel:
    networks: [default, downloads]
    environment:
      BOWEL_EXTRA_PORTS: 7902=media
      BOWEL_LOCK: media
  qbittorrent:
    image: lscr.io/linuxserver/qbittorrent
    networks: [downloads]
  webui:
    image: caddy:2-alpine
    networks: [default, downloads]
    ports: ["8080:8080"]
    command: caddy reverse-proxy --from :8080 --to qbittorrent:8080
networks:
  downloads:
    internal: true
```

在 qBittorrent（`http://<主机>:8080`，初始密码在 `docker logs qbittorrent` 里）的 工具 → 选项 → 连接 → 代理服务器 里选 SOCKS5，主机 `bowel`，端口 `7902`，并让 peer 连接和域名解析都经过代理。qBittorrent 绕开代理发出的流量无路可走；经过这个端口的流量只走 `media` 的节点，走不了就断开。所以节点断了或者订阅失败时，下载会停下来，而不是用你自己的 IP 继续下。

内部网络不能发布端口，所以用反向代理把网页界面接出来。qBittorrent 要求两边端口一致：要换端口，请同时修改 `WEBUI_PORT`、端口映射和 caddy 的两个端口。`media` 可以换成任何路由组，最好给下载单独建一个路由组，用专门的节点。很多机场禁止 BT 下载，请先确认。

### 每个容器一个端口

[docker/compose.ports.yaml](docker/compose.ports.yaml) 用常见的代理环境变量给两个容器配了两个出口：`assistant` 用 `bowel:7901`（`ai`），`downloader` 用 `bowel:7902`（`media`）。大小写两种变量都要写，因为 curl 只认小写的 `http_proxy`；两种都不认的程序会自己直接出去，这类程序请用 TUN 的方式。端口也发布到了宿主机上，所以局域网里的机器同样可以用 `http://<主机>:7901` 选出口。加上 `BOWEL_LOCK: ai media` 后，路由组没有节点时端口会拒绝连接，而不是直连。

在 Mac 或 PC 上，App 也能这样用：打开「允许局域网连接」，再执行 `bowel settings set extraPorts '[{"port":7901,"route":"ai","lan":true}]'`（见 [cli.zh-CN.md](cli.zh-CN.md#每个路由组一个端口)）。

## 自己构建

```bash
docker build -t bowel .
docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v7 -t you/bowel --push .
```

所有执行命令的构建阶段都在构建机上交叉编译，所以多平台构建不需要模拟器。
