#!/usr/bin/env bash
# Prints the Scoop manifest of a release, for bucket/bowel.json in
# 21stware/scoop-bucket. It unpacks the release's Windows archive: the
# files the installer installs, which the app's own updates replace, in a
# folder of Scoop's. The app's data is in %APPDATA%\bowel, which Scoop leaves
# alone. checkver and autoupdate let the bucket's tools (checkver.ps1 -u,
# Excavator) follow new releases without this script.
#
#   scripts/render-scoop.sh 0.2.0 <sha256 of the windows-amd64 .tar.gz> <sha256 of the windows-arm64 one> >bucket/bowel.json
set -euo pipefail
VERSION="$1"
SHA_AMD64="$2"
SHA_ARM64="$3"

URL=https://github.com/21stware/bowel
DOWNLOAD="$URL/releases/download/v"

cat <<EOF
{
    "version": "$VERSION",
    "description": "A desktop client for mihomo (Clash Meta)",
    "homepage": "https://bowel.dev/",
    "license": {
        "identifier": "PolyForm-Noncommercial-1.0.0",
        "url": "$URL/blob/main/LICENSE"
    },
    "architecture": {
        "64bit": {
            "url": "$DOWNLOAD$VERSION/bowel-$VERSION-windows-amd64.tar.gz",
            "hash": "$SHA_AMD64"
        },
        "arm64": {
            "url": "$DOWNLOAD$VERSION/bowel-$VERSION-windows-arm64.tar.gz",
            "hash": "$SHA_ARM64"
        }
    },
    "shortcuts": [
        [
            "bowel.exe",
            "bowel"
        ]
    ],
    "checkver": {
        "github": "$URL"
    },
    "autoupdate": {
        "architecture": {
            "64bit": {
                "url": "$DOWNLOAD\$version/bowel-\$version-windows-amd64.tar.gz"
            },
            "arm64": {
                "url": "$DOWNLOAD\$version/bowel-\$version-windows-arm64.tar.gz"
            }
        },
        "hash": {
            "url": "\$baseurl/SHA256SUMS"
        }
    }
}
EOF
