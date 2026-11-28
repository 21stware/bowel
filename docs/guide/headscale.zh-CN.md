# Tailscale 与 Headscale

bowel 能和 tailnet 一起用：组网里的地址和名字不走代理节点。用的是 Tailscale 官方的服务器还是自建的 [Headscale](https://github.com/juanfont/headscale)，都可以。

- **macOS、Windows、Linux**：用系统里装的 Tailscale 客户端。在客户端里接好 Headscale 就行，bowel 不用另外配。
- **Android**：系统同时只能有一个 VPN，所以 bowel 自带一个 Tailscale 节点，不装官方 App 也能访问 tailnet。接 Headscale 要在 bowel 里填控制服务器。

## 电脑上

在 Tailscale 客户端里登录到 Headscale，例如：

```sh
tailscale up --login-server https://headscale.example.com
```

bowel 每 20 秒读一次客户端的状态，读到下面这些就直接连接，规则、全局、直连三种模式都一样：

- `100.64.0.0/10` 和 `fd7a:115c:a1e0::/48` 的地址
- MagicDNS 的名字：域名后缀从客户端的状态里读，Headscale 的 `base_domain` 和 `ts.net` 一样处理，不用手填
- 短主机名，例如 `ssh mybox`
- 子网路由，以及管理员配的 Split DNS 域名

出口节点（exit node）和 TUN 抢默认路由，bowel 不做兼容，设置页会提示。具体怎么做的见 [developer networks](../dev-network.md)（英文）。

## Android 上

打开「设置 → Tailscale」。

1. **控制服务器**：填 Headscale 的地址，例如 `https://headscale.example.com`，http 也可以（没配 TLS 的 Headscale 也能接）。留空就用 Tailscale 官方的服务器。
2. **MagicDNS 域名**：填了控制服务器后才有这一行，填 Headscale 配置里的 `base_domain`，例如 `tail.example.com`。bowel 没法从节点那里知道这个域名，所以要手填；不填的话只能用 `100.x` 地址访问组网里的设备。这个域名不能是控制服务器的域名或它的上级，例如控制服务器是 `headscale.example.com` 时不能填 `example.com`，不然登录页也会被送进还没登录的节点（Headscale 自己也不让这样配）。
3. **设备名**：在 tailnet 里显示的名字，留空是 `bowel-android`。
4. 打开「内置 Tailscale」。要先开启 bowel 的 VPN，并关掉官方 Tailscale App。
5. 登录，二选一：
   - **用浏览器**：点「登录」→「打开登录链接」，按 Headscale 页面上说的，在服务器上注册这台设备。
   - **用 auth key**：在「可选：不用浏览器登录」下填 auth key。先在 Headscale 上用 `headscale preauthkeys create` 生成一个，十六进制的和 `hskey-auth-…` 开头的都能用。key 只在第一次登录时用，之后登录状态保存在 bowel 里，所以建议用一次性的 key。

登录后，`100.64.0.0/10`、`*.ts.net` 和填的 MagicDNS 域名下的名字，在三种模式下都走内置节点，其余流量照旧。bowel 自己从 tailnet 里取订阅时也走它。

### 换控制服务器

节点只属于一个控制服务器，所以改「控制服务器」会退出登录：节点换一个新的状态目录，旧的删掉；之前填的 auth key 也会清掉，除非这次一起填了新的。改完要在新的服务器上重新登录。

「退出登录」只让这台设备忘掉登录状态，控制服务器上还留着它，需要的话在 Headscale 上删除（`headscale nodes delete`）。

## 另一种做法：导入 `type: tailscale` 节点

在有内置节点之前，接 Headscale 的办法是导入一个 mihomo 的 `type: tailscale` 节点，写上 `control-url` 和 `auth-key`，再新建一个路由组固定到它，自己加上组网的 IP 段和域名规则。这样做只在规则模式下有效，现在用不着了；已经这样配的照常能用。

这种节点通向的是组网里的设备，没有出口节点时测速站点本来连不上，所以测速不通时显示「—」，不算坏了；自动最快也不会选它去上网。
