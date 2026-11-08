#!/usr/bin/env bash
# Downloads the mihomo core for this machine (or $GOOS/$GOARCH) from the
# official MetaCubeX release into resources/<os>-<arch>/bin/, where MyGo
# picks it up as a platform resource.
set -euo pipefail
VERSION="${MIHOMO_VERSION:-v1.19.33}"
OS="${GOOS:-$(go env GOOS)}"
ARCH="${GOARCH:-$(go env GOARCH)}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEST="$ROOT/resources/$OS-$ARCH/bin"
mkdir -p "$DEST"

# fetch <repo> <tag> <asset> <file>: the release download URL can answer 504
# while GitHub's API serves the same file, so the gh CLI is the fallback.
fetch() {
  echo "fetching $1 $2 $3"
  curl -fL --retry 3 -o "$4" "https://github.com/$1/releases/download/$2/$3" && return
  command -v gh >/dev/null || return 1
  echo "direct download failed; trying the GitHub API"
  gh release download "$2" --repo "$1" --pattern "$3" --output "$4" --clobber
}

if [ "$OS" = windows ]; then
  ASSET="mihomo-$OS-$ARCH-$VERSION.zip"; BIN="mihomo.exe"
else
  ASSET="mihomo-$OS-$ARCH-$VERSION.gz"; BIN="mihomo"
fi
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
fetch MetaCubeX/mihomo "$VERSION" "$ASSET" "$TMP/$ASSET"
if [ "$OS" = windows ]; then
  unzip -o -q "$TMP/$ASSET" -d "$TMP/x" && mv "$TMP"/x/*.exe "$DEST/$BIN"
else
  gunzip -c "$TMP/$ASSET" > "$DEST/$BIN" && chmod +x "$DEST/$BIN"
fi
"$DEST/$BIN" -v 2>/dev/null || true
echo "installed $DEST/$BIN"

# Geo data the rules need (GEOSITE / GEOIP), so the first start works offline.
GEO="$ROOT/resources/geo"
mkdir -p "$GEO"
for f in "geosite.dat:GeoSite.dat" "geoip-lite.dat:GeoIP.dat"; do
  src="${f%%:*}"; dst="${f##*:}"
  if [ ! -s "$GEO/$dst" ]; then
    fetch MetaCubeX/meta-rules-dat latest "$src" "$GEO/$dst"
  fi
done
ls -la "$GEO"
