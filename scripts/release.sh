#!/usr/bin/env bash
# Builds bowel for release: mygo builds the app and the disk image and signs
# them with the Developer ID in mygo.json; this script then notarizes the
# disk image and staples the ticket to it and to the app.
#
# mygo can notarize by itself (macos.notarize), but it runs
# `notarytool submit --wait --output-format json`, which dies with a bus
# error under Xcode 27's notarytool after the upload. The plain-text form
# works, so the notarizing happens here.
#
# Needs the notary credentials in the keychain once:
#   xcrun notarytool store-credentials notary --key <AuthKey_….p8> --key-id <id> --issuer <uuid>
# or, where there is no such keychain (GitHub Actions), the App Store Connect
# key in NOTARY_KEY (a path), NOTARY_KEY_ID and NOTARY_ISSUER.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
if [ -n "${NOTARY_KEY:-}" ]; then
	AUTH=(--key "$NOTARY_KEY" --key-id "$NOTARY_KEY_ID" --issuer "$NOTARY_ISSUER")
else
	AUTH=(--keychain-profile "${NOTARY_PROFILE:-notary}")
fi

go tool mygo build -platform darwin/arm64 -skip-notarize .

DIR="$ROOT/build/darwin-arm64"
# macOS 27 lets the core tell which program opened a connection only when
# its signing identifier has a dot in it; codesign makes it of the file's
# name (core.BinName). Rules by application depend on it, so a build where
# it came out otherwise is not released.
CORE="$DIR/bowel.app/Contents/Resources/bin/dev.moho.app.core.bin"
CORE_ID="$(codesign -d -v "$CORE" 2>&1 | sed -n 's/^Identifier=//p')"
if [ "$CORE_ID" != dev.moho.app.core ]; then
	echo "the core is signed as '$CORE_ID', not dev.moho.app.core: $CORE" >&2
	exit 1
fi
DMG="$(ls -t "$DIR"/*.dmg | head -1)"
LOG="$(mktemp)"; trap 'rm -f "$LOG"' EXIT
xcrun notarytool submit "$DMG" "${AUTH[@]}" --wait >"$LOG" 2>&1 || true
tr '\r' '\n' <"$LOG" | grep -v '^Current status' >&2
if ! grep -q 'status: Accepted' "$LOG"; then
	echo "notarization was not accepted; see: xcrun notarytool log <id> ${AUTH[*]}" >&2
	exit 1
fi
xcrun stapler staple "$DMG"
xcrun stapler staple "$DIR/bowel.app"
spctl -a -t open --context context:primary-signature "$DMG"
echo "notarized: $DMG"
