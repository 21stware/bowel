#!/bin/bash
# Builds what the Android app ships besides Dart and Kotlin:
#   jniLibs/arm64-v8a/libmihomo.so    mihomo itself, as its own executable
#   jniLibs/arm64-v8a/liblauncher.so  hands it the TUN descriptor
#   libs/bridge.aar                   mobile/bridge, which does not link mihomo
#
# ./build.sh          everything
# ./build.sh core     only the two executables
set -euo pipefail

MIHOMO_VERSION=v1.19.33
NDK_VERSION=28.2.13676358

here="$(cd "$(dirname "$0")" && pwd)"
# The SDK Flutter builds with, whatever the shell's ANDROID_HOME says. A fresh
# checkout, as in CI, has no local.properties yet and goes by ANDROID_HOME.
props="$here/../flutter/android/local.properties"
[ ! -f "$props" ] || ANDROID_HOME="$(sed -n 's/^sdk.dir=//p' "$props")"
export ANDROID_HOME ANDROID_NDK_HOME="$ANDROID_HOME/ndk/$NDK_VERSION"

app="$here/../flutter/android/app"
jni="$app/src/main/jniLibs/arm64-v8a"
mkdir -p "$jni" "$app/libs"

host="$(uname -s | tr '[:upper:]' '[:lower:]')-x86_64"
cc="$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/$host/bin/aarch64-linux-android24-clang"
build_env=(GOOS=android GOARCH=arm64 CGO_ENABLED=1 CC="$cc")

GOMODCACHE="$(go env GOMODCACHE)"
src="$GOMODCACHE/github.com/metacubex/mihomo@$MIHOMO_VERSION"
[ -d "$src" ] || go mod download "github.com/metacubex/mihomo@$MIHOMO_VERSION"

# Bringing up a TUN on Android makes mihomo read /data/system/packages.list,
# which apps may not open. Per-app rules live in VpnService here, so that step
# is dropped at build time. The cmfa tag drops it too, but also removes the
# controller endpoints the bridge reloads through. It is patched in a copy, as
# go does not allow -overlay to replace files in the module cache.
patched="$(mktemp -d)"
trap 'rm -rf "$patched"' EXIT
cp -R "$src/." "$patched"
chmod -R u+w "$patched"
android_tun=listener/sing_tun/server_android.go
perl -0pi -e 's/(func \(l \*Listener\) buildAndroidRules\(tunOptions \*tun\.Options\) error \{\n).*?\n\}\n/$1\treturn nil\n}\n/s' \
	"$patched/$android_tun"
if cmp -s "$src/$android_tun" "$patched/$android_tun"; then
	echo "buildAndroidRules not found in mihomo $MIHOMO_VERSION" >&2
	exit 1
fi

echo "building mihomo $MIHOMO_VERSION"
(cd "$patched" && env "${build_env[@]}" go build -trimpath -buildmode=pie -tags with_gvisor \
	-ldflags "-s -w -X github.com/metacubex/mihomo/constant.Version=$MIHOMO_VERSION" \
	-o "$jni/libmihomo.so" .)

echo "building launcher"
(cd "$here/launcher" && env "${build_env[@]}" go build -trimpath -buildmode=pie \
	-ldflags "-s -w" -o "$jni/liblauncher.so" .)

[ "${1:-all}" = core ] && exit 0

echo "building bridge.aar"
# gomobile runs go from temporary folders, where auto-switching picks an older
# toolchain: pin the one in use here.
root="$(go env GOROOT)"
export PATH="$root/bin:$HOME/go/bin:$PATH" GOTOOLCHAIN=local
cd "$here/../bridge"
gomobile bind -target=android/arm64 -androidapi 24 -trimpath -ldflags="-s -w" \
	-o "$app/libs/bridge.aar" .
