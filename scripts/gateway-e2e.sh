#!/usr/bin/env bash
# Runs gateway mode for real, in three containers on one Docker network:
#
#   gateway  bowel daemon as root, TUN and gateway mode on
#   client   a device whose router and DNS are set to the gateway
#   node     a mihomo that listens as a Shadowsocks server, the gateway's node;
#            it also plays the router that hands out addresses (dnsmasq)
#   lease    a device that asks for its address by DHCP
#
# and checks what a device on the LAN would see, and what is left of the
# gateway when bowel stops, crashes, comes back after a reboot, or loses
# its core. Nothing outside the containers changes. Needs Docker and Go.
#
#   ./scripts/gateway-e2e.sh          run and clean up
#   KEEP=1 ./scripts/gateway-e2e.sh   leave the containers for a look
set -euo pipefail

MIHOMO_VERSION="${MIHOMO_VERSION:-v1.19.33}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
NET=bowel-e2e
SUBNET=172.31.77
GW=$SUBNET.2 CLIENT=$SUBNET.10 NODE=$SUBNET.20 LEASE=$SUBNET.30
IMAGE=debian:stable-slim
WORK="$(mktemp -d)"

cleanup() {
  [ -n "${KEEP:-}" ] || {
    docker rm -f bowel-e2e-gw bowel-e2e-client bowel-e2e-node bowel-e2e-lease >/dev/null 2>&1 || true
    docker network rm $NET >/dev/null 2>&1 || true
  }
  rm -rf "$WORK"
}
trap cleanup EXIT

arch="$(docker version --format '{{.Server.Arch}}')"
echo "building bowel and fetching mihomo $MIHOMO_VERSION for linux/$arch"
(cd "$ROOT" && GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build -o "$WORK/bowel" .)
curl -fsSL --retry 3 -o "$WORK/mihomo.gz" \
  "https://github.com/MetaCubeX/mihomo/releases/download/$MIHOMO_VERSION/mihomo-linux-$arch-$MIHOMO_VERSION.gz"
gunzip "$WORK/mihomo.gz"
chmod +x "$WORK/mihomo"
cat > "$WORK/node.yaml" <<EOF
mode: direct
listeners:
  - {name: ss-in, type: shadowsocks, listen: 0.0.0.0, port: 8388, cipher: aes-128-gcm, password: e2e, udp: true}
EOF

docker rm -f bowel-e2e-gw bowel-e2e-client bowel-e2e-node bowel-e2e-lease >/dev/null 2>&1 || true
docker network rm $NET >/dev/null 2>&1 || true
docker network create --subnet $SUBNET.0/24 $NET >/dev/null

# The containers resolve through a public resolver until the test points
# the client at the gateway: Docker's own is not reachable through TUN.
run() { # run <name> <ip> <docker args...>
  local name=$1 ip=$2
  shift 2
  docker run -d --init --name "$name" --network $NET --ip "$ip" --dns 1.1.1.1 "$@" $IMAGE sleep infinity >/dev/null
  docker exec "$name" sh -c 'apt-get update -qq >/dev/null 2>&1 && apt-get install -y -qq iproute2 curl dnsutils ca-certificates iputils-ping procps dnsmasq-base udhcpc >/dev/null 2>&1'
}
echo "starting the containers"
run bowel-e2e-node $NODE --cap-add NET_ADMIN
run bowel-e2e-client $CLIENT --cap-add NET_ADMIN
run bowel-e2e-lease $LEASE --cap-add NET_ADMIN
run bowel-e2e-gw $GW --privileged --sysctl net.ipv4.ip_forward=0 \
  -e BOWEL_DATA_DIR=/data -e MOHO_CORE=/opt/bowel/mihomo -e MYGO_ENV=production

docker exec bowel-e2e-node mkdir -p /node
docker cp -q "$WORK/mihomo" bowel-e2e-node:/node/mihomo
docker cp -q "$WORK/node.yaml" bowel-e2e-node:/node/config.yaml
docker exec -d bowel-e2e-node sh -c '/node/mihomo -d /node >/node/log 2>&1'

docker exec bowel-e2e-gw mkdir -p /opt/bowel
docker cp -q "$WORK/bowel" bowel-e2e-gw:/opt/bowel/bowel
docker cp -q "$WORK/mihomo" bowel-e2e-gw:/opt/bowel/mihomo
docker exec bowel-e2e-gw ln -sf /opt/bowel/bowel /usr/local/bin/bowel

gw() { docker exec bowel-e2e-gw sh -c "$1"; }
client() { docker exec bowel-e2e-client sh -c "$1"; }
daemon() { docker exec -d bowel-e2e-gw sh -c 'bowel daemon >>/var/log/bowel.log 2>&1'; }
# up waits for the daemon to answer, and for the gateway to be measured on.
up() {
  for _ in $(seq 60); do
    gw 'bowel status --json 2>/dev/null' | grep -A2 '"gateway"' | grep -q '"status": "on"' && return 0
    sleep 0.5
  done
  return 1
}
forwarding() { gw 'cat /proc/sys/net/ipv4/ip_forward'; }
# serving waits for bowel to hand out addresses: before each start it asks
# the LAN for a few seconds whether someone else does.
serving() {
  for _ in $(seq "${1:-40}"); do
    gw 'ss -lun' | grep -q ':67 ' && return 0
    sleep 0.5
  done
  return 1
}
cores() { gw "ps -eo stat,comm | grep -c '^[^Z].* mihomo' || true"; }
fetch() { client 'curl -sS -m 10 -o /dev/null -w "%{http_code}" https://example.com 2>/dev/null || true'; }

fails=0
check() { # check <what> <got> <want>
  if [ "$2" = "$3" ]; then
    echo "  ok    $1"
  else
    echo "  FAIL  $1: got '$2', want '$3'"
    fails=$((fails + 1))
  fi
}
like() { # like <what> <got> <pattern>
  if echo "$2" | grep -Eq "$3"; then
    echo "  ok    $1"
  else
    echo "  FAIL  $1: got '$2', want /$3/"
    fails=$((fails + 1))
  fi
}

echo "turning the gateway on"
daemon
for _ in $(seq 40); do gw 'bowel status >/dev/null 2>&1' && break; sleep 0.5; done
gw 'bowel gateway on' >/dev/null
link="ss://$(printf 'aes-128-gcm:e2e' | base64)@$NODE:8388#e2e-node"
gw "bowel providers add '$link'" >/dev/null 2>&1
up || true
sleep 3
client "ip route replace default via $GW && echo 'nameserver $GW' > /etc/resolv.conf"

echo "the gateway itself"
status="$(gw 'bowel status --json')"
like "measured on" "$(echo "$status" | grep -A2 '"gateway"')" '"status": "on"'
check "IPv4 forwarding turned on" "$(forwarding)" 1
like "no error for a system proxy a server can't have" "$(gw 'bowel status' | head -1)" '正常|OK|降级|Degraded'
like "the resolver listens on port 53" "$(gw 'ss -lun')" ':53 '

echo "a device behind it"
like "DNS answers with a fake IP" "$(client 'dig +short +time=3 +tries=1 example.com')" '^198\.18\.'
like "a hard-coded resolver is answered too" "$(client 'dig +short +time=3 +tries=1 example.org @8.8.8.8')" '^198\.18\.'
like "DNS over TCP" "$(client 'dig +tcp +short +time=3 +tries=1 example.net')" '^198\.18\.'
check "HTTPS by name" "$(fetch)" 200
like "HTTP to an address, with no lookup" "$(client 'curl -sS -m 10 -o /dev/null -w "%{http_code}" http://1.1.1.1')" '^(200|301)$'
like "UDP that is not DNS" "$(client 'dig +short +time=4 +tries=3 example.com @208.67.222.222 -p 5353')" '^[0-9]+\.'
like "ping" "$(client 'ping -c 3 -W 3 1.1.1.1 | tail -2')" ' [1-3] received'
like "a neighbour on the LAN is reached directly" "$(client "ping -c 1 -W 2 $NODE | tail -2")" ' 0% packet loss'
like "the traffic left through the node" "$(docker exec bowel-e2e-node grep -c -- '--> example.com:443' /node/log)" '^[1-9]'
# Devices are counted from the connections that are open, so one is held.
docker exec -d bowel-e2e-client sh -c 'curl -s -m 20 --limit-rate 200 -o /dev/null https://www.cloudflare.com/'
sleep 9
like "the gateway counts the device" "$(gw 'bowel status --json' | grep '"devices"')" '[1-9]'

echo "handing out addresses"
# The node plays the router's DHCP server.
router_dhcp() { docker exec -d bowel-e2e-node dnsmasq --keep-in-foreground --port=0 --dhcp-range=$SUBNET.150,$SUBNET.160,2m --dhcp-option=3,$NODE; sleep 1; }
router_dhcp
like "the check finds the router's DHCP server" "$(gw 'bowel gateway check --json' | tr -d ' \n')" "\"servers\":\[\"$NODE\"\]"
like "automatic is refused while it answers" "$(gw 'bowel gateway auto 2>&1; echo exit=$?' | tr '\n' ' ')" "$NODE.*exit=1"
check "and nothing was started" "$(gw 'ss -lun | grep -c ":67 " || true')" 0
docker exec bowel-e2e-node pkill dnsmasq || true
sleep 1
like "the check finds none once it is off" "$(gw 'bowel gateway check --json' | tr -d ' \n')" '"servers":\[\]'
like "automatic turns on" "$(gw 'bowel gateway auto 2>&1; echo exit=$?')" 'exit=0'
like "bowel listens for DHCP" "$(gw 'ss -lun')" ':67 '
like "status says automatic, serving" "$(gw 'bowel status --json' | tr -d ' \n')" '"auto":\{"supported":true,"wanted":true,"status":"on"'
# A device asks for an address: it gets one of the pool, with the gateway
# as its router and DNS, and gets out through it.
lease() { docker exec bowel-e2e-lease sh -c "$1"; }
lease "ip addr flush dev eth0; udhcpc -i eth0 -n -q -t 5 -T 2 >/tmp/udhcpc.log 2>&1; cat /tmp/udhcpc.log | tail -3" >/dev/null || true
like "a device got an address of the pool" "$(lease 'ip -4 -o addr show eth0')" "inet $SUBNET\.1[0-9][0-9]/24"
like "its router is the gateway" "$(lease 'ip route show default')" "default via $GW "
like "its DNS is the gateway" "$(lease 'cat /etc/resolv.conf')" "nameserver $GW"
check "it gets out" "$(lease 'curl -sS -m 10 -o /dev/null -w "%{http_code}" https://example.com 2>/dev/null || true')" 200
sleep 6
like "the gateway lists it as automatic" "$(gw 'bowel gateway devices --json' | tr -d ' \n')" '"via":"auto","online":true'
like "and the hand-pointed device as manual" "$(gw 'bowel gateway devices --json' | tr -d ' \n')" "\"ip\":\"$CLIENT\"[^}]*\"via\":\"manual\""
# The router's DHCP comes back: bowel says so.
router_dhcp
like "a second server is reported" "$(gw 'bowel gateway check 2>&1')" "$NODE"
like "and shown as an error of the automatic step" "$(gw 'bowel status --json' | tr -d ' \n')" '"wanted":true,"status":"error"'
docker exec bowel-e2e-node pkill dnsmasq || true
gw 'bowel gateway check' >/dev/null

echo "bowel stops"
gw 'bowel stop' >/dev/null
sleep 3
check "forwarding is back as it was" "$(forwarding)" 0
check "no core is left" "$(cores)" 0
check "nothing is left in the data folder" "$(gw 'ls /data | grep -c before || true')" 0
daemon
up || true
check "started again, the gateway is back by itself" "$(fetch)" 200
serving || true
like "and hands out addresses again" "$(gw 'ss -lun')" ':67 '
like "the device's lease is remembered" "$(gw 'cat /data/gateway-leases.json')" "$SUBNET\.1[0-9][0-9]"

echo "bowel starts while the router hands out addresses again"
gw 'bowel stop' >/dev/null
sleep 3
router_dhcp
daemon
up || true
sleep 9
check "it stays out" "$(gw 'ss -lun | grep -c ":67 " || true')" 0
like "and says who answers" "$(gw 'bowel status --json' | tr -d ' \n')" "\"wanted\":true,\"status\":\"error\",\"detail\":\"$NODE"
check "the device pointed by hand is not touched by it" "$(fetch)" 200
docker exec bowel-e2e-node pkill dnsmasq || true
serving 90 || true
like "it starts by itself once that is off" "$(gw 'ss -lun')" ':67 '

echo "bowel is killed"
# The core is bowel's child and may end with it; what counts is that the
# next start (a service manager's, in real life) brings everything back.
gw 'pkill -9 -x bowel'
sleep 2
daemon
up || true
check "started again, one core" "$(cores)" 1
check "and the device still gets out" "$(fetch)" 200

echo "the machine restarts"
gw 'pkill -9 -x bowel; pkill -9 -x mihomo; sleep 1; echo 0 > /proc/sys/net/ipv4/ip_forward'
daemon
up || true
check "forwarding is on again" "$(forwarding)" 1
check "the device gets out" "$(fetch)" 200

echo "the core is killed under bowel"
gw 'pkill -9 -x mihomo'
sleep 6
check "bowel started it again" "$(cores)" 1
check "the device gets out" "$(fetch)" 200
gw 'for i in 1 2 3; do pkill -9 -x mihomo; sleep 1; done'
sleep 14
check "and again after three in a row" "$(fetch)" 200

echo "the gateway is turned off"
gw 'bowel gateway off' >/dev/null
sleep 2
check "forwarding is back as it was" "$(forwarding)" 0
like "port 53 is closed" "$(gw 'ss -lun | grep -c ":53 " || true')" '^0$'
like "and so is DHCP's" "$(gw 'ss -lun | grep -c ":67 " || true')" '^0$'
like "the next time starts at manual" "$(gw 'bowel status --json' | tr -d ' \n')" '"auto":\{"supported":true,"wanted":false,"status":"off"'

echo
if [ "$fails" -gt 0 ]; then
  echo "$fails check(s) failed. The daemon's log:"
  gw 'tail -30 /var/log/bowel.log' || true
  exit 1
fi
echo "all checks passed"
