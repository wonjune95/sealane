#!/usr/bin/env bash
# End-to-end smoke test against a freshly installed Sealane (initial admin/admin account).
# Usage: hack/e2e.sh http://localhost:8080 [namespace-for-the-test-VirtualService]
# Needs: curl, jq. Exits non-zero on the first failed check.
set -euo pipefail
BASE=${1:?base url, e.g. http://localhost:8080}
NS=${2:-default}
TYPE=virtualservices.networking.istio.io
NAME=sealane-e2e
JAR=$(mktemp)
BODY=$(mktemp)
NEWPASS="E2e-Pass-$(date +%s)!"
trap 'rm -f "$JAR" "$BODY"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
# api <method> <path> [json-body] → prints the status code, body goes to $BODY
api() {
  local m=$1 p=$2 b=${3:-}
  if [ -n "$b" ]; then
    curl -s --max-time 20 -o "$BODY" -w '%{http_code}' -b "$JAR" -c "$JAR" -X "$m" -H 'Content-Type: application/json' -d "$b" "$BASE$p" || true
  else
    curl -s --max-time 20 -o "$BODY" -w '%{http_code}' -b "$JAR" -c "$JAR" -X "$m" "$BASE$p" || true
  fi
}
expect() { local want=$1 got=$2 what=$3; [ "$got" = "$want" ] || fail "$what: expected HTTP $want, got $got: $(head -c 300 "$BODY")"; echo "ok  $what ($got)"; }
# Account writes patch the ConfigMap; the mounted copy syncs within ~1 min, so retry the login (up to 2 min: kubelet sync period + ConfigMap cache TTL).
login_retry() {
  local code
  for _ in $(seq 1 24); do
    code=$(api POST /api/login "{\"username\":\"$1\",\"password\":\"$2\"}")
    [ "$code" = 200 ] && break
    sleep 5
  done
  echo "$code"
}

echo "== probes"
expect 200 "$(curl -s --max-time 20 -o "$BODY" -w '%{http_code}' "$BASE/healthz")" "healthz"
expect 200 "$(curl -s --max-time 20 -o "$BODY" -w '%{http_code}' "$BASE/readyz")" "readyz"

echo "== initial login and forced password change"
expect 200 "$(api POST /api/login '{"username":"admin","password":"admin"}')" "login admin/admin"
expect 200 "$(api GET /api/capabilities)" "capabilities"
[ "$(jq -r '.mustChangePassword' "$BODY")" = "true" ] || fail "mustChangePassword should be true on first login"
expect 204 "$(api POST /api/account/password "{\"currentPassword\":\"admin\",\"newPassword\":\"$NEWPASS\"}")" "change password"
expect 200 "$(login_retry admin "$NEWPASS")" "login with new password"
expect 200 "$(api GET /api/capabilities)" "capabilities after change"
[ "$(jq -r '.mustChangePassword // false' "$BODY")" = "false" ] || fail "mustChangePassword should be false now"
[ "$(jq -r '.virtualServiceInstalled' "$BODY")" = "true" ] || fail "Istio CRDs not detected"
[ "$(jq -r '.httpRouteInstalled' "$BODY")" = "true" ] || fail "Gateway API CRDs not detected"
echo "    appVersion=$(jq -r '.appVersion' "$BODY") istiod=$(jq -r '.istiodVersion // "n/a"' "$BODY")"

echo "== VirtualService dry-run → create → list → flow map → delete"
VS=$(cat <<EOF
{"apiVersion":"networking.istio.io/v1","kind":"VirtualService","metadata":{"name":"$NAME","namespace":"$NS"},
 "spec":{"hosts":["e2e.example.com"],"http":[{"route":[{"destination":{"host":"e2e.$NS.svc.cluster.local"}}]}]}}
EOF
)
expect 200 "$(api POST "/api/resources/$TYPE?dryRun=true" "$VS")" "dry-run create"
expect 201 "$(api POST "/api/resources/$TYPE" "$VS")" "create"
expect 200 "$(api GET "/api/resources/$TYPE")" "list"
jq -e --arg n "$NAME" --arg ns "$NS" 'map(select(.name==$n and .namespace==$ns)) | length == 1' "$BODY" >/dev/null || fail "created VirtualService missing from list"
expect 200 "$(api GET /api/flowmap)" "flow map"
expect 204 "$(api DELETE "/api/resources/$TYPE/$NS/$NAME")" "delete"
expect 200 "$(api GET "/api/resources/$TYPE")" "list after delete"
jq -e --arg n "$NAME" --arg ns "$NS" 'map(select(.name==$n and .namespace==$ns)) | length == 0' "$BODY" >/dev/null || fail "VirtualService still listed after delete"

echo "== viewer is read-only"
code=$(api PUT /api/accounts/e2e-viewer '{"role":"viewer","password":"Viewer-Pass-2026!"}')
{ [ "$code" = 200 ] || [ "$code" = 204 ]; } || fail "create viewer account: got $code: $(head -c 300 "$BODY")"
echo "ok  create viewer account ($code)"
expect 200 "$(login_retry e2e-viewer 'Viewer-Pass-2026!')" "login as viewer"
expect 403 "$(api POST "/api/resources/$TYPE?dryRun=true" "$VS")" "viewer write is forbidden"
echo "ALL OK"
