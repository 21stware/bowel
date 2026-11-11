#!/usr/bin/env bash
# Installs Ubuntu packages on a GitHub runner, and survives its mirror being
# down: the runner's own list is tried a few times, then two mirrors that
# share nothing with it. `apt-get update` exits 0 with stale lists when a
# mirror cannot be reached, so it is the install that is watched.
#
#   scripts/apt-install.sh nsis
set -uo pipefail
[ "$#" -gt 0 ] || { echo "usage: $0 <package>..." >&2; exit 2; }

OPTS=(-o Acquire::Retries=3 -o Acquire::http::Timeout=20 -o Acquire::https::Timeout=20)

try() {
  sudo apt-get "${OPTS[@]}" update
  sudo apt-get "${OPTS[@]}" install -y "$@"
}

# use_mirror points every Ubuntu source at one mirror, whichever way the
# image names them: the mirror list of the runner images, or plain sources.
use_mirror() {
  if [ -f /etc/apt/apt-mirrors.txt ]; then
    printf '%s/\tpriority:1\n' "$1" | sudo tee /etc/apt/apt-mirrors.txt >/dev/null
    return
  fi
  local f
  for f in /etc/apt/sources.list.d/ubuntu.sources /etc/apt/sources.list; do
    [ -f "$f" ] && sudo sed -i -E "s#https?://[A-Za-z0-9.-]+/ubuntu/?#$1/#g" "$f"
  done
  return 0
}

for attempt in 1 2 3; do
  try "$@" && exit 0
  echo "apt-install: attempt $attempt with the runner's mirrors failed" >&2
  sleep $((attempt * 10))
done

for mirror in https://archive.ubuntu.com/ubuntu https://mirrors.edge.kernel.org/ubuntu; do
  echo "apt-install: trying $mirror" >&2
  use_mirror "$mirror"
  try "$@" && exit 0
  sleep 10
done

echo "apt-install: could not install: $*" >&2
exit 1
