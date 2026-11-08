#!/usr/bin/env bash
# Renders the screenshots the README and the website show (TestSiteShots,
# off screen, 2×) and writes them to site/shots/ as WebP. Needs cwebp
# (brew install webp).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/site/shots"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
cd "$ROOT"
BOWEL_SITE_SHOTS="$TMP" CGO_ENABLED=0 go test -count=1 -run '^TestSiteShots$' . >/dev/null
mkdir -p "$OUT"
for png in "$TMP"/*.png; do
  name="$(basename "$png" .png)"
  cwebp -quiet -q 84 -m 6 "$png" -o "$OUT/$name.webp"
done
# The app icon (assets/icon, the designer's full-bleed render), for the site's header and favicon.
sips -s format png -Z 256 assets/icon/bowel-1024.png --out "$OUT/../icon.png" >/dev/null
echo "wrote $(ls "$OUT" | wc -l | tr -d ' ') screenshots to site/shots/"
