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
# Each step is on its own line so that set -e stops the script when one fails
# (a failure on the left of && does not).
if [ "$OS" = windows ]; then
  # unzip is often missing (WSL, slim containers); Python's zipfile does the same.
  if command -v unzip >/dev/null; then
    unzip -o -q "$TMP/$ASSET" -d "$TMP/x"
  elif command -v python3 >/dev/null; then
    python3 -I -m zipfile -e "$TMP/$ASSET" "$TMP/x"
  else
    echo "need unzip or python3 to extract $ASSET" >&2
    exit 1
  fi
  mv "$TMP"/x/*.exe "$DEST/$BIN"
else
  gunzip -c "$TMP/$ASSET" > "$DEST/$BIN"
  chmod +x "$DEST/$BIN"
fi
"$DEST/$BIN" -v 2>/dev/null || true
echo "installed $DEST/$BIN"

# fetch_source <url> <api path> <file>: like fetch, for files outside a release.
fetch_source() {
  curl -fL --retry 3 -o "$3" "$1" && return
  command -v gh >/dev/null || return 1
  echo "direct download failed; trying the GitHub API"
  gh api -H "Accept: application/vnd.github.raw" "$2" > "$3"
}

# GPL-3.0 notices for the binary above, which the app bundle carries from here.
NOTICES="$ROOT/resources/licenses/mihomo"
mkdir -p "$NOTICES"
fetch_source "https://raw.githubusercontent.com/MetaCubeX/mihomo/$VERSION/LICENSE" \
  "repos/MetaCubeX/mihomo/contents/LICENSE?ref=$VERSION" "$NOTICES/LICENSE"
# The notices below and the app's About section say GPL-3.0, so a future
# version under other terms has to stop the build until they are updated.
if ! grep -q 'GNU GENERAL PUBLIC LICENSE' "$NOTICES/LICENSE" || ! grep -q 'Version 3' "$NOTICES/LICENSE"; then
  echo "mihomo $VERSION is not under GPL-3.0; update the notices in this script" >&2
  exit 1
fi
cat > "$NOTICES/SOURCE.txt" <<EOF
mihomo $VERSION
Copyright (C) the mihomo and Clash authors

This program is free software under the GNU General Public License, version 3.
See LICENSE in this folder. It comes with ABSOLUTELY NO WARRANTY.

bowel runs the unmodified official binary ($ASSET) as a separate program.

Source code for this version:
  https://github.com/MetaCubeX/mihomo/tree/$VERSION
A copy is attached to each bowel release:
  https://github.com/21stware/bowel/releases
EOF

# A release attaches the matching source, so it stays available from bowel.
if [ -n "${MIHOMO_SOURCE_DIR:-}" ]; then
  mkdir -p "$MIHOMO_SOURCE_DIR"
  fetch_source "https://github.com/MetaCubeX/mihomo/archive/refs/tags/$VERSION.tar.gz" \
    "repos/MetaCubeX/mihomo/tarball/$VERSION" "$MIHOMO_SOURCE_DIR/mihomo-$VERSION-source.tar.gz"
fi

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
