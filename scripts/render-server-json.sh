#!/usr/bin/env bash
# Prints the server.json that publishes a release's MCPB bundle to the MCP
# Registry as io.github.21stware/bowel (.github/workflows/mcp-registry.yml).
#
#   scripts/render-server-json.sh 0.4.7 <sha256 of bowel-0.4.7.mcpb> >server.json
set -euo pipefail
VERSION="$1"
SHA256="$2"

cat <<EOF
{
  "\$schema": "https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json",
  "name": "io.github.21stware/bowel",
  "title": "bowel",
  "description": "Read and troubleshoot bowel, a mihomo (Clash Meta) proxy client; changes ask you first.",
  "websiteUrl": "https://bowel.dev/en/",
  "repository": {
    "url": "https://github.com/21stware/bowel",
    "source": "github"
  },
  "icons": [
    {
      "src": "https://bowel.dev/icon.png",
      "mimeType": "image/png",
      "sizes": ["256x256"]
    }
  ],
  "version": "$VERSION",
  "packages": [
    {
      "registryType": "mcpb",
      "identifier": "https://github.com/21stware/bowel/releases/download/v$VERSION/bowel-$VERSION.mcpb",
      "fileSha256": "$SHA256",
      "transport": {
        "type": "stdio"
      }
    }
  ]
}
EOF
