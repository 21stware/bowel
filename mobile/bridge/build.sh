#!/bin/bash
# Builds Bridge.xcframework (the app) and Tunnel.xcframework (the extension).
set -euo pipefail
cd "$(dirname "$0")"
out=../flutter/ios/Frameworks
mkdir -p "$out"
# gomobile runs go from temporary folders, where auto-switching picks an older
# toolchain: pin the one this module resolves to.
root=$(PATH="/usr/local/go/bin:$PATH" go env GOROOT)
export PATH="$root/bin:$HOME/go/bin:$PATH" GOTOOLCHAIN=local
gomobile bind -target=ios,iossimulator -iosversion=16.0 -trimpath -ldflags="-s -w" -o "$out/Bridge.xcframework" .
gomobile bind -target=ios,iossimulator -iosversion=16.0 -trimpath -ldflags="-s -w" -tags with_gvisor -o "$out/Tunnel.xcframework" ./tunnel
