#!/usr/bin/env bash
# Builds the MCPB bundle of bowel's MCP server from mcp/, for one version:
# the manifest with that version, the launcher in mcp/server and the icon.
# Run it from the repository's root; it needs Node, whose npx fetches the
# MCPB command line that checks the manifest and packs the bundle.
#
#   scripts/build-mcpb.sh 0.4.7 out    # writes out/bowel-0.4.7.mcpb
#
# Every path stays relative, so that a Windows Node can follow them too.
set -euo pipefail
VERSION="$1"
OUT="$2"
MCPB="@anthropic-ai/mcpb@2.1.2"

stage="$OUT/mcpb-stage"
rm -rf "$stage"
mkdir -p "$stage"
trap 'rm -rf "$stage"' EXIT

cp -R mcp/server "$stage/server"
cp site/icon.png "$stage/icon.png"
node -e '
const fs = require("fs");
const m = JSON.parse(fs.readFileSync("mcp/manifest.json", "utf8"));
m.version = process.argv[1];
fs.writeFileSync(process.argv[2], JSON.stringify(m, null, 2) + "\n");
' "$VERSION" "$stage/manifest.json"

npx --yes "$MCPB" validate "$stage/manifest.json"
npx --yes "$MCPB" pack "$stage" "$OUT/bowel-$VERSION.mcpb"
