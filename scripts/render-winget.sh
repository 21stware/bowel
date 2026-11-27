#!/usr/bin/env bash
# Writes the winget manifests of a release into a directory: the version,
# installer and default locale (en-US) manifests, and a zh-CN locale, which
# microsoft/winget-pkgs keeps in manifests/2/21stware/bowel/<version>.
#
# They install the release's NSIS installers, which mygo build makes: per
# user, in %LOCALAPPDATA%\Programs\bowel, with no elevation, and listed in
# Settings > Apps under the key dev.moho.app, the identifier in mygo.json.
# /S installs silently and quits the app if it runs; /D= picks the folder.
#
#   scripts/render-winget.sh 0.2.0 <sha256 of the amd64 setup .exe> <sha256 of the arm64 one> out [release date]
set -euo pipefail
VERSION="$1"
SHA_AMD64="$(printf %s "$2" | tr a-f A-F)"
SHA_ARM64="$(printf %s "$3" | tr a-f A-F)"
OUT="$4"
DATE="${5:-$(date -u +%Y-%m-%d)}"

ID=21stware.bowel
SCHEMA=1.12.0
URL=https://github.com/21stware/bowel
DOWNLOAD="$URL/releases/download/v$VERSION"
HERE="$(cd "$(dirname "$0")" && pwd)"
COPYRIGHT="$(sed -n 's/^Required Notice: //p' "$HERE/../LICENSE" | tr -d '\r')"
mkdir -p "$OUT"

# The comment that names the schema of a manifest type, which editors use.
header() {
	echo "# yaml-language-server: \$schema=https://aka.ms/winget-manifest.$1.$SCHEMA.schema.json"
	echo "# Written by scripts/render-winget.sh of $URL"
	echo
}

{
	header version
	cat <<EOF
PackageIdentifier: $ID
PackageVersion: $VERSION
DefaultLocale: en-US
ManifestType: version
ManifestVersion: $SCHEMA
EOF
} >"$OUT/$ID.yaml"

{
	header installer
	cat <<EOF
PackageIdentifier: $ID
PackageVersion: $VERSION
InstallerType: nullsoft
Scope: user
InstallModes:
  - interactive
  - silent
  - silentWithProgress
# NSIS's own, which winget passes to nullsoft installers as well. The
# installer has no progress window of its own when silent.
InstallerSwitches:
  Silent: /S
  SilentWithProgress: /S
# NSIS's exit codes. 1: cancelled on one of the installer's pages. 2: the
# app was running, and the user chose to keep it open or it could not be
# stopped.
ExpectedReturnCodes:
  - InstallerReturnCode: 1
    ReturnResponse: cancelledByUser
  - InstallerReturnCode: 2
    ReturnResponse: packageInUse
UpgradeBehavior: install
ProductCode: dev.moho.app
ReleaseDate: $DATE
AppsAndFeaturesEntries:
  - DisplayName: bowel
    ProductCode: dev.moho.app
InstallationMetadata:
  DefaultInstallLocation: '%LOCALAPPDATA%\Programs\bowel'
Installers:
  - Architecture: x64
    InstallerUrl: $DOWNLOAD/bowel-setup-$VERSION-amd64.exe
    InstallerSha256: $SHA_AMD64
  - Architecture: arm64
    InstallerUrl: $DOWNLOAD/bowel-setup-$VERSION-arm64.exe
    InstallerSha256: $SHA_ARM64
ManifestType: installer
ManifestVersion: $SCHEMA
EOF
} >"$OUT/$ID.installer.yaml"

{
	header defaultLocale
	cat <<EOF
PackageIdentifier: $ID
PackageVersion: $VERSION
PackageLocale: en-US
Publisher: 21stware
PublisherUrl: https://github.com/21stware
PublisherSupportUrl: $URL/issues
PackageName: bowel
PackageUrl: https://bowel.dev/en/
License: PolyForm-Noncommercial-1.0.0
LicenseUrl: $URL/blob/main/LICENSE
Copyright: $COPYRIGHT
ShortDescription: A desktop client for mihomo (Clash Meta)
Description: |-
  bowel is a mihomo (Clash Meta) client for Windows, macOS, Linux and Android: a native app drawn on the GPU, not Electron or a WebView.
  AI services, streaming, development and the rest are separate routes, each with its own node, chosen by hand or by speed.
  It imports subscription links, node links, QR codes and complete Clash or mihomo configs, shows the app, rule, exit and speed of each connection, and has an MCP server through which agents read its state and troubleshoot connections.
Moniker: bowel
Tags:
  - clash
  - clash-meta
  - mihomo
  - proxy
  - tun
  - vpn
ReleaseNotesUrl: $URL/releases/tag/v$VERSION
Documentations:
  - DocumentLabel: Docs
    DocumentUrl: https://bowel.dev/en/docs/
ManifestType: defaultLocale
ManifestVersion: $SCHEMA
EOF
} >"$OUT/$ID.locale.en-US.yaml"

{
	header locale
	cat <<EOF
PackageIdentifier: $ID
PackageVersion: $VERSION
PackageLocale: zh-CN
PackageUrl: https://bowel.dev/
ShortDescription: mihomo（Clash Meta）桌面客户端
Description: |-
  bowel 是 Windows、macOS、Linux 和 Android 上的 mihomo（Clash Meta）客户端，原生应用，通过 GPU 渲染，不基于 Electron，也不用 WebView。
  AI 服务、流媒体、开发等各是一条路由，各用各的节点，可以手选，也可以按测速自动选。
  订阅链接、节点链接、二维码，以及完整的 Clash 或 mihomo 配置都能导入；每条连接的应用、规则、出口和速度都看得到；Agent 可以通过 MCP 服务器读取状态、排查连接。
Tags:
  - clash
  - clash-meta
  - mihomo
  - proxy
  - 代理
ReleaseNotesUrl: $URL/releases/tag/v$VERSION
Documentations:
  - DocumentLabel: 文档
    DocumentUrl: https://bowel.dev/docs/
ManifestType: locale
ManifestVersion: $SCHEMA
EOF
} >"$OUT/$ID.locale.zh-CN.yaml"
