#!/usr/bin/env bash
# Installs Ubuntu packages on a GitHub runner, and survives its mirrors being
# out of reach: the runner's own list is tried once, briefly, then mirrors
# that share nothing with Ubuntu's own hosts. `apt-get update` exits 0 with
# stale lists when a mirror cannot be reached, so it is the install that is
# watched. Timeouts are short on purpose: a runner that cannot reach a
# mirror waits out every index of every suite, and with apt's defaults one
# failed round took ten minutes.
#
#   scripts/apt-install.sh nsis
set -uo pipefail
[ "$#" -gt 0 ] || { echo "usage: $0 <package>..." >&2; exit 2; }

OPTS=(-o Acquire::Retries=0 -o Acquire::http::Timeout=5 -o Acquire::https::Timeout=5 -o Acquire::ForceIPv4=true)

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

try "$@" && exit 0
echo "apt-install: the runner's mirrors failed" >&2

for mirror in https://mirrors.edge.kernel.org/ubuntu https://mirror.us.leaseweb.net/ubuntu https://archive.ubuntu.com/ubuntu; do
  echo "apt-install: trying $mirror" >&2
  use_mirror "$mirror"
  try "$@" && exit 0
done

echo "apt-install: could not install: $*" >&2
exit 1
