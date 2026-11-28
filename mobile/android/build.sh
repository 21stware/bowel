#!/bin/bash
# Builds what the Android app ships besides Dart and Kotlin, for 64-bit ARM
# phones (arm64-v8a) and the 32-bit ARM userspace many TV boxes run, even on
# 64-bit chips (armeabi-v7a):
#   jniLibs/<abi>/libmihomo.so    mihomo itself, as its own executable,
#                                 with the owner module to tell apps apart
#   jniLibs/<abi>/liblauncher.so  hands it the TUN descriptor
#   libs/bridge.aar               mobile/bridge, which does not link mihomo
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
mkdir -p "$app/libs"

host="$(uname -s | tr '[:upper:]' '[:lower:]')-x86_64"
clang="$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/$host/bin"

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

# A rule for an app needs the app's package name, which from Android 10 only
# the VPN service can ask for. The owner module asks it (see its doc), and
# owner_android.go puts that in mihomo's main package as its package-name
# resolver. mihomo asks that resolver only after reading the kernel's socket
# tables, which apps may not do from Android 10, unless it is built as Clash
# Meta for Android; so with a resolver it is asked alone, as there.
tunnel_go=tunnel/tunnel.go
perl -0pi -e 's/if !features\.CMFA \{(\n\s*\/\/ normal check for process)/if !features.CMFA && process.DefaultPackageNameResolver == nil {$1/' \
	"$patched/$tunnel_go"
if cmp -s "$src/$tunnel_go" "$patched/$tunnel_go"; then
	echo "the process lookup not found in mihomo $MIHOMO_VERSION" >&2
	exit 1
fi
cp "$here/owner/_mihomo/owner_android.go" "$patched/"
(cd "$patched" && go mod edit \
	-require=github.com/oboo/moho/mobile/android/owner@v0.0.0 \
	-replace=github.com/oboo/moho/mobile/android/owner="$here/owner")

# build_for abi compiler goenv... builds both executables into jniLibs/<abi>.
build_for() {
	local abi="$1" cc="$clang/$2"
	shift 2
	local build_env=(GOOS=android CGO_ENABLED=1 CC="$cc" "$@")
	local jni="$app/src/main/jniLibs/$abi"
	mkdir -p "$jni"

	echo "building mihomo $MIHOMO_VERSION for $abi"
	(cd "$patched" && env "${build_env[@]}" go build -trimpath -buildmode=pie -tags with_gvisor \
		-ldflags "-s -w -X github.com/metacubex/mihomo/constant.Version=$MIHOMO_VERSION" \
		-o "$jni/libmihomo.so" .)

	echo "building launcher for $abi"
	(cd "$here/launcher" && env "${build_env[@]}" go build -trimpath -buildmode=pie \
		-ldflags "-s -w" -o "$jni/liblauncher.so" .)
}
build_for arm64-v8a aarch64-linux-android24-clang GOARCH=arm64
build_for armeabi-v7a armv7a-linux-androideabi24-clang GOARCH=arm GOARM=7

[ "${1:-all}" = core ] && exit 0

echo "building bridge.aar"
# gomobile runs go from temporary folders, where auto-switching picks an older
# toolchain: pin the one in use here.
root="$(go env GOROOT)"
export PATH="$root/bin:$HOME/go/bin:$PATH" GOTOOLCHAIN=local
cd "$here/../bridge"
gomobile bind -target=android/arm64,android/arm -androidapi 24 -trimpath -ldflags="-s -w" \
	-o "$app/libs/bridge.aar" .
