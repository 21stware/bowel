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
| `ghcr.io/21stware/bowel:0.4.8` | 某一个版本 |

同一个镜像也在 Docker Hub 上，叫 `21stware/bowel`（`docker pull 21stware/bowel`）。群晖的 Container Manager、Unraid 等 NAS 界面默认在那里搜索；本文的命令用哪个名字都可以。

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

容器里没有窗口可以弹出询问，所以在默认的「询问」策略下，agent 能读取一切，但改不了任何东西。要让 agent 能修改，执行 `docker exec bowel bowel settings set mcpPolicy allow`。同一台主机上的 agent 也可以运行 `docker exec -i bowel bowel mcp`，也就是 stdio 版本的 MCP，不需要令牌。

## 健康检查和日志

镜像的健康检查是 `bowel health`：daemon 有响应、内核在运行、端口能连上时为健康。`docker logs bowel` 是 daemon 的日志；内核的日志用 `docker exec bowel bowel logs -f` 查看。`docker stop` 会干净地停止它。

## 自己构建

```bash
docker build -t bowel .
docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v7 -t you/bowel --push .
```

所有执行命令的构建阶段都在构建机上交叉编译，所以多平台构建不需要模拟器。
