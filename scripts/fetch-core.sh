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
if [ "$OS" = windows ]; then
  ASSET="mihomo-$OS-$ARCH-$VERSION.zip"; BIN="mihomo.exe"
else
  ASSET="mihomo-$OS-$ARCH-$VERSION.gz"; BIN="mihomo"
fi
URL="https://github.com/MetaCubeX/mihomo/releases/download/$VERSION/$ASSET"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
echo "fetching $URL"
curl -fL --retry 3 -o "$TMP/$ASSET" "$URL"
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
    echo "fetching $src"
    curl -fL --retry 3 -o "$GEO/$dst" "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/$src"
  fi
done
ls -la "$GEO"
