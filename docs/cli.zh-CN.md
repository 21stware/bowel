# 命令行

同一个程序也是命令行，用来操作运行中的 bowel。怎么把它放进 `PATH`、最常用的命令，见 [README](../README.zh-CN.md#命令行)；这一页是其余的部分。

## 全屏界面

`bowel tui` 是同一套功能的全屏界面：概览、连接、日志、规则、Provider 和设置六个页面，实时刷新。概览页有一组开关（接管、系统代理、TUN、网关模式、允许局域网）和路由列表：在开关上按回车或空格切换，在路由上按回车选节点（`t` 测速），`m` 切换模式，`a` 添加订阅；在路由上还可以用[路由组](#路由组)的按键设置它。设置页可以修改任意一项设置。`/` 过滤连接，`?` 查看全部按键。会改变网络的操作先问 y/n。通过 SSH 也能用：`ssh server -t bowel tui`。

## 放进状态栏

`bowel bar` 每秒给 waybar（Omarchy 的顶栏，Hyprland、Sway 常用的也是它）输出一行：文字是当前网速，提示里是状态、模式、接管、各路由的节点和今日流量，另有 `on`、`off`、`paused`、`stopped` 和 `degraded`、`error` 这些 class 可以按状态改样式。它只需启动一次，bowel 重启或没在运行时会接着等。不带命令再运行一次程序会把窗口调出来，所以单击就能打开窗口：

```jsonc
// ~/.config/waybar/config.jsonc：同时把 "custom/bowel" 加进某个 modules 列表
"custom/bowel": {
  "exec": "/opt/bowel/bowel bar",
  "return-type": "json",
  "on-click": "/opt/bowel/bowel"
}
```

在 `style.css` 里用 `#custom-bowel.off`、`#custom-bowel.stopped` 等改样式。polybar、i3blocks 等读纯文字的状态栏用 `bowel bar --format text`；按间隔运行命令的再加 `--once`。用 `bowel daemon` 运行时没有窗口可开。

## 路由组

窗口里「路由组」列表能做的，这里都能做，所以用 `bowel daemon` 或在 Docker 里运行的 bowel 不开窗口也能配好。`bowel route show` 按生效顺序列出路由组，每个带上节点策略和主测速站点；`bowel route show <路由组>` 显示一个路由组的全部设置：节点策略、范围、备用顺序、正在用的节点、测速站点、服务和你的规则。路由组可以用 ID 或名字指定。

```bash
bowel route new Netflix --before media            # 新建自己的路由组：自动选最快的节点，还没有规则
bowel rules add netflix.com Netflix               # 把流量送过来
bowel route move Netflix --at 1                   # 从上到下匹配；也可以用 --before / --after 另一个路由组
bowel route strategy Netflix fixed --node "🇯🇵 Tokyo 02"
bowel route order Netflix "🇯🇵 Tokyo 02" "🇸🇬 Singapore 01"   # 顺序备用的列表，第一个优先
bowel route strategy Netflix auto                 # 改回自动最快；每种策略的设置都会保留
bowel route scope Netflix --regions JP,SG --providers a1b2c3 --except   # 自动最快的范围；all 为全部
bowel route strategy Netflix --by download --every 12   # 按下载速度选，每 6、12 或 24 小时测一轮
bowel route sites Netflix https://www.netflix.com fast.com   # 测速站点，第一个是主目标；--reset 恢复默认
bowel route direct Netflix on                     # 不经过节点直接连接；节点策略保留，关掉后照旧
bowel route rename Netflix Video
bowel route remove Video --to media               # 它的服务和规则交给 media（不写时交给 default）
```

自动最快的范围是地区（国家或地区代码）、来源 Provider（只用所选，或加 `--except` 排除所选）和用 `--nodes` 手动挑的节点三者同时满足；列表用逗号分隔，名字里本身有逗号时写成 JSON 列表（`'["a, b","c"]'`）。新的主测速站点会先经这个路由组正在用的节点试一次，不通就不改。「其他流量」「直连」「拦截」一直都在、不能改名，「其他流量」固定在最后；「直连」和「拦截」从不走节点，没有节点策略和测速站点。

正在使用某个 Provider 配置自带的路由时，路由组是那份配置的：这些命令会拒绝，`bowel route use` 仍可以给其中 select 类型的组选成员，`bowel routing bowel` 改回 bowel 自己的路由组（这期间它们一直保留着）。`bowel routing <Provider>` 改用那份配置的路由，`bowel routing` 查看正在用的是哪个。

在 `bowel tui` 概览页的路由上，`s` 设置节点策略，`J`、`K` 下移和上移，`e` 修改测速站点，`R` 改名，`d` 删除，`n` 新建路由组；在它的节点列表里，`o` 把节点加入或移出备用顺序。agent 也有同样的 MCP 工具（`list_route_groups`、`create_route_group`、`move_route_group`、`set_route_strategy`、`set_route_sites`、`rename_route_group`、`delete_route_group`、`set_routing_source`），和它的其他变更走同一个策略。

## 每个路由组一个端口

想让本机的两个程序用不同的出口访问同一个网站，可以给它们各配一个代理端口：`bowel settings set extraPorts '[{"port":7901,"route":"ai"},{"port":7902,"route":"media"}]'` 会在 `127.0.0.1` 上再开几个混合端口，每个端口的流量固定走对应的路由组（用 `bowel routes` 里的 ID），不看规则，也不受全局、直连模式影响。正在使用导入配置自带的路由时，`route` 改写这份配置里路由组的名称（`bowel routes` 在路由下方列出）；指向的路由组不存在时，端口不会打开。同一个名字可以既是 bowel 的路由 ID，又是这份配置里的一个路由组：这样的条目在两种路由下端口都会打开，流量走当前路由里叫这个名字的那个路由组。每个端口和主端口一样同时支持 HTTP 和 SOCKS5（包括 UDP）。锁定了对应路由组的出口（`bowel route lock ai`）后，从这个端口进来的流量只走节点，走不了就断开，不会直连。

端口默认只给本机用，条目里写上 `"lan": true` 才对局域网开放：`bowel settings set extraPorts '[{"port":7901,"route":"ai","lan":true}]'`。开放的方式和主端口一样：只在 `allowLan` 打开时生效，只接受 `lanAllowedIps` 里的网段，设置了 `authentication` 就要密码（本机不用）。在容器里（`BOWEL_CONTAINER`），容器的回环地址只有它自己能用，所以每个额外端口都按这个方式开放；还可以用环境变量 `BOWEL_EXTRA_PORTS` 设置，见 [docker.zh-CN.md](docker.zh-CN.md#给其他容器当出口)。这些端口不会写进导出的配置，界面里也没有对应的设置；设成 `'[]'` 即关闭。

## 在脚本里用

每个命令都有 `--help` 和例子。读取类命令支持 `--json`，`logs -f` 和 `conns --watch` 加上它后每行输出一个 JSON 对象。输出不是终端或设置了 `NO_COLOR` 时，不显示颜色和进度动画。退出码：0 成功，1 执行失败（例如 bowel 没在运行），2 命令写错了。`bowel completion bash|zsh|fish|powershell` 输出补全脚本。

## 无窗口运行

`bowel daemon` 不开窗口、菜单栏图标和 Dock 图标，只运行内核、监控和命令行通道。它在前台运行，收到 SIGINT、SIGTERM 或 `bowel stop` 后退出，并像应用退出时一样恢复系统代理，适合交给 systemd、launchd 或服务包装器托管。用 `BOWEL_DATA_DIR`（或 `--data-dir`）可以给它单独的数据目录。daemon 不会自动升级。daemon 运行时再打开应用不会有反应，需要先停掉 daemon。

```ini
# /etc/systemd/system/bowel.service
[Service]
ExecStart=/opt/bowel/bowel daemon
Environment=BOWEL_DATA_DIR=/var/lib/bowel
Restart=on-failure
```

## Agent

你自己敲的命令不会被询问。由 agent 运行的命令（Claude Code、Codex、Cursor 等，按它们的环境变量识别）和 MCP 用同一个策略：询问、允许或拒绝。daemon 没有窗口可以弹框，策略为“询问”时会拒绝 agent 的变更；可以用 `bowel settings set mcpPolicy allow` 放行，这条命令只有你自己能执行。

这些环境变量过不了 `docker exec` 和 `ssh`。Docker 镜像里所有命令都经由 `docker exec`，所以只有背后是你的终端（`docker exec -it`）的命令才算你的；没有终端的命令算 agent 的，它的修改遵循策略。经由 `ssh` 时，agent 运行的命令仍然看起来和你的一样，也不会被询问。但两者都放宽不了策略：放宽（从“询问”到“允许”，或从“拒绝”到“询问”）要你本人在终端里确认。`bowel settings set mcpPolicy allow` 会让你输入 `allow`，没有终端可以询问时会被拒绝：在脚本里、不带 `-it` 的 `docker exec` 里、不带 `-t` 的 `ssh` 里都是这样。`bowel tui` 的设置页也这样确认；应用的 设置 > MCP 不需要。收紧策略从不询问。
