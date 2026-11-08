<p align="center">
  <img src="site/icon.png" width="112" alt="bowel">
</p>

<h1 align="center">bowel</h1>

<p align="center">
bowel is a <a href="https://github.com/MetaCubeX/mihomo">mihomo</a> client for macOS.
</p>

<p align="center">
  <a href="https://github.com/21stware/bowel/releases/latest"><b>Download for macOS</b></a> ·
  <a href="https://21stware.github.io/bowel/">Website</a> ·
  <a href="README.zh-CN.md">简体中文</a>
</p>

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="site/shots/home-en-dark.webp">
    <img src="site/shots/home-en.webp" width="880" alt="bowel main window">
  </picture>
</p>

The interface is built in Go and rendered directly via the GPU, with no WebView.

## Features

### Routes
Route traffic by purpose, and select exit nodes manually or automatically by speed.

<img src="site/shots/routes-en.webp" width="880" alt="Routes">

### Connections
Inspect the app, destination, rule, exit, and speed for each connection, or reroute domains on the fly.

<img src="site/shots/activity-en.webp" width="880" alt="Connections log">

### Providers
Import subscription links, node URLs, YAML configs, or QR codes, with data usage and expiry shown for each source.

<img src="site/shots/providers-en.webp" width="880" alt="Providers">

### Menu bar
Toggle system proxy takeover, switch proxy modes, and select default nodes from the menu bar.

<img src="site/shots/panel-en.webp" width="300" alt="Menu bar panel">

### Skins
Six skins that change the look of the whole interface.

<table>
  <tr>
    <td><img src="site/shots/skin-broadsheet-en.webp" alt="Broadsheet"><br><sub>Broadsheet</sub></td>
    <td><img src="site/shots/skin-phosphor-en.webp" alt="Phosphor"><br><sub>Phosphor</sub></td>
    <td><img src="site/shots/skin-neon-en.webp" alt="Neon"><br><sub>Neon</sub></td>
  </tr>
  <tr>
    <td><img src="site/shots/skin-blueprint-en.webp" alt="Blueprint"><br><sub>Blueprint</sub></td>
    <td><img src="site/shots/skin-sakura-en.webp" alt="Sakura"><br><sub>Sakura</sub></td>
    <td><img src="site/shots/skin-paper-en.webp" alt="Paper"><br><sub>Paper (default)</sub></td>
  </tr>
</table>

### Other
- **Custom rules**: Route specific domains, keywords, or IP ranges, and test where addresses go.
- **Measured exit locations**: Verify true exit locations through Cloudflare trace.
- **Gateway mode**: Turn this Mac into a LAN gateway for devices like TVs and consoles.
- **TUN**: Intercept traffic from applications that ignore system proxy settings.
- **MCP**: Built-in MCP server for Claude Code and other agents to read status and troubleshoot connections. It asks you before changing settings by default.
  ```bash
  claude mcp add --scope user bowel -- /Applications/bowel.app/Contents/MacOS/bowel mcp
  ```
- **Settings and languages**: Searchable settings (⌘F); English, Simplified Chinese, Traditional Chinese and Japanese; light and dark.

## Install

1. Download `bowel-<version>-arm64.dmg` from [Releases](https://github.com/21stware/bowel/releases/latest) and drag bowel into Applications.
2. Add a provider (a subscription URL, node links or a QR code) and turn on takeover.

Needs macOS 12 or later. Released builds are for Apple silicon, signed with a Developer ID and notarized by Apple.

## Develop

Needs Go 1.27.3+ (`GOTOOLCHAIN=auto` fetches it). No Node or Bun.

```bash
./scripts/fetch-core.sh     # mihomo + GeoSite/GeoIP data into resources/
go tool mygo dev            # the app, rebuilt on every change
go tool mygo build          # bowel.app and a .dmg, signed but not notarized
./scripts/release.sh        # build, notarize and staple: use this for releases
CGO_ENABLED=0 go test ./... # includes tests that start the real core
./scripts/site-shots.sh     # re-render the README's and the website's screenshots (needs cwebp)
```

- How it works and where the code is: [docs/architecture.md](docs/architecture.md)
- The product's information architecture: [docs/product-ia.md](docs/product-ia.md)
- The website is static, in `site/`; open `site/index.html` to preview it. Changes to it go live when they reach `main`.
- To release, set the version in `mygo.json` and push a matching tag, such as `v0.1.0`. GitHub Actions builds, signs and notarizes the disk image and attaches it to a new release.

bowel was called Moho. The Go module is still `github.com/oboo/moho` and the bundle identifier `dev.moho.app`, which keeps macOS's camera and screen-recording permissions. On first launch the data in `~/Library/Application Support/Moho` moves to `…/bowel`.

## Credits

- [mihomo](https://github.com/MetaCubeX/mihomo): the proxy core, GPL-3.0. bowel runs it as a separate program and does not link it.
- [MyGo](https://github.com/egoist/mygo): the native Go UI toolkit.
- [OPPO Sans](resources/fonts/): the interface's typeface.

## License

bowel is licensed under the [PolyForm Noncommercial License 1.0.0](LICENSE), which allows noncommercial use only. For a commercial license, [open an issue](https://github.com/21stware/bowel/issues).

The bundled mihomo core stays under GPL-3.0. The app carries its license in `Contents/Resources/licenses/mihomo/`, and each release attaches the matching source. The flag images in `assets/flags/` and the fonts in `resources/fonts/` are third-party files and keep their own licenses.
