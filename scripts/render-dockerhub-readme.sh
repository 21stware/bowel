#!/usr/bin/env bash
# Prints docs/docker.md as the image's overview on Docker Hub
# (.github/workflows/docker.yml). Docker Hub has none of the files beside the
# doc, so its relative links are made to point at them on GitHub.
#
#   scripts/render-dockerhub-readme.sh docs/docker.md https://github.com/21stware/bowel/blob/main/docs/ >overview.md
set -euo pipefail
DOC="$1"
BASE="$2" # the doc's folder on GitHub, ending in /
NAME="$(basename "$DOC")"

{
  # Docker Hub shows the repository's name above this, so the doc's title
  # gives way to what bowel is.
  echo "# bowel"
  echo
  echo "[bowel](https://github.com/21stware/bowel) is a mihomo (Clash Meta) client for macOS, Windows, Linux and Android. This image is bowel for servers, NAS boxes and routers. [中文说明](${BASE}docker.zh-CN.md)"
  sed '1{/^# /d;}' "$DOC"
} | sed -E \
  -e "s|\]\((#[^)]*)\)|](${BASE}${NAME}\1)|g" \
  -e "s|\]\(([^)#:][^):]*)\)|](${BASE}\1)|g"
