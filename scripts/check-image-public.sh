#!/usr/bin/env bash
# Checks that each image can be pulled without logging in, as anyone who runs
# `docker pull` would: it asks the registry for an anonymous token and then for
# the tag's manifest (a HEAD request, which Docker Hub does not count as a
# pull). Each name that can't be pulled gets an error that says what to change;
# the script exits 1 once all of them are checked. .github/workflows/docker.yml
# runs it after pushing.
#
#   scripts/check-image-public.sh ghcr.io/21stware/bowel:0.4.9 docker.io/21stware/bowel:0.4.9
set -euo pipefail

ACCEPT='application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json'
TRIES=3 # a tag that was just pushed can take a moment to show
DELAY=10

# status_of URL [TOKEN]: the HTTP status of a HEAD request, 000 when it fails.
status_of() {
  local auth=() code
  if [ -n "${2:-}" ]; then auth=(-H "Authorization: Bearer $2"); fi
  code="$(curl -sS -o /dev/null -I -w '%{http_code}' --max-time 30 "${auth[@]}" -H "Accept: $ACCEPT" "$1" 2>/dev/null || true)"
  echo "${code:-000}"
}

# probe REF: prints ok, private, missing or error, and after it what the
# registry said.
probe() {
  local ref="$1" registry path repo tag api url challenge realm service body status token
  registry="${ref%%/*}"
  path="${ref#*/}"
  repo="${path%:*}"
  tag="${path##*:}"
  api="$registry"
  if [ "$registry" = docker.io ]; then
    api=registry-1.docker.io
    case "$repo" in */*) ;; *) repo="library/$repo" ;; esac
  fi
  url="https://$api/v2/$repo/manifests/$tag"

  # A registry that wants a token says where to get one.
  challenge="$(curl -sS -I --max-time 30 -H "Accept: $ACCEPT" "$url" 2>/dev/null |
    tr -d '\r' | sed -n 's/^[Ww][Ww][Ww]-[Aa]uthenticate: *[Bb]earer //p' || true)"
  token=""
  if [ -n "$challenge" ]; then
    realm="$(sed -n 's/.*realm="\([^"]*\)".*/\1/p' <<<"$challenge")"
    service="$(sed -n 's/.*service="\([^"]*\)".*/\1/p' <<<"$challenge")"
    body="$(curl -sS -G --max-time 30 -w '\n%{http_code}' --data-urlencode "service=$service" \
      --data-urlencode "scope=repository:$repo:pull" "$realm" 2>/dev/null || true)"
    status="${body##*$'\n'}"
    body="${body%$'\n'*}"
    case "$status" in
      200) token="$(sed -n 's/.*"token" *: *"\([^"]*\)".*/\1/p' <<<"$body")" ;;
      # GHCR: 401 for a private package, 403 for none at all.
      401) echo "private: the registry gives no anonymous token for $repo (401)"; return ;;
      403) echo "missing: the registry has no $repo (403)"; return ;;
      *) echo "error: the token request answered ${status:-nothing}"; return ;;
    esac
  fi

  status="$(status_of "$url" "$token")"
  case "$status" in
    200) echo "ok" ;;
    # Docker Hub gives anyone a token, and then refuses a private or missing
    # repository the same way.
    401 | 403) echo "private: an anonymous token may not pull $repo ($status), so it is private or does not exist" ;;
    404) echo "missing: the registry has no $repo:$tag (404)" ;;
    *) echo "error: the registry answered $status" ;;
  esac
}

# hint KIND REF: what to change.
hint() {
  local kind="$1" registry="${2%%/*}" path="${2#*/}" owner name
  path="${path%:*}"
  owner="${path%%/*}"
  name="${path#*/}"
  case "$kind:$registry" in
    private:ghcr.io)
      echo "Set the package's visibility to Public: https://github.com/orgs/$owner/packages/container/$name/settings (Danger Zone, Change visibility; for a personal account's package, /users/$owner/ in place of /orgs/$owner/)." ;;
    private:docker.io)
      echo "Create the repository $path on Docker Hub as a public one, or make it public in its settings there, and check that the variable DOCKERHUB_IMAGE names it." ;;
    private:*)
      echo "Make $registry/$path public in that registry." ;;
    missing:*)
      echo "The push did not leave this tag there: see the push step above." ;;
    *)
      echo "Run the check again; if it keeps failing, see whether $registry is up." ;;
  esac
}

failed=0
for ref in "$@"; do
  for try in $(seq "$TRIES"); do
    result="$(probe "$ref")"
    case "$result" in ok | private:*) break ;; esac
    if [ "$try" -lt "$TRIES" ]; then sleep "$DELAY"; fi
  done
  kind="${result%%:*}"
  case "$kind" in
    ok) echo "$ref: anyone can pull it"; continue ;;
    private) lead="can't be pulled without logging in" ;;
    missing) lead="is not there" ;;
    *) lead="could not be checked" ;;
  esac
  echo "::error::$ref $lead: ${result#*: }. $(hint "$kind" "$ref")"
  failed=1
done
exit "$failed"
