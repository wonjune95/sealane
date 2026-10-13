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
CREATED_VS=
CREATED_VIEWER=

fail() { echo "FAIL: $*" >&2; exit 1; }
# api <method> <path> [json-body] → prints the status code, body goes to $BODY
api() {
  local m=$1 p=$2 b=${3:-}
  : >"$BODY"
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
# Best-effort cleanup of what a failed run left behind; deletes run as admin (the jar may hold the viewer session).
cleanup() {
  if [ -n "$CREATED_VS$CREATED_VIEWER" ] && [ -s "$JAR" ]; then
    api POST /api/login "{\"username\":\"admin\",\"password\":\"$NEWPASS\"}" >/dev/null 2>&1 || true
    [ -z "$CREATED_VS" ] || api DELETE "/api/resources/$TYPE/$NS/$NAME" >/dev/null 2>&1 || true
    [ -z "$CREATED_VIEWER" ] || api DELETE /api/accounts/e2e-viewer >/dev/null 2>&1 || true
  fi
  rm -f "$JAR" "$BODY"
}
trap cleanup EXIT

echo "== probes"
expect 200 "$(curl -s --max-time 20 -o "$BODY" -w '%{http_code}' "$BASE/healthz")" "healthz"
expect 200 "$(curl -s --max-time 20 -o "$BODY" -w '%{http_code}' "$BASE/readyz")" "readyz"

echo "== initial login and forced password change"
# the app creates the accounts ConfigMap at startup; its optional mount may not be synced yet
expect 200 "$(login_retry admin admin)" "login admin/admin"
expect 200 "$(api GET /api/capabilities)" "capabilities"
[ "$(jq -r '.mustChangePassword' "$BODY")" = "true" ] || fail "mustChangePassword should be true on first login"
expect 204 "$(api POST /api/account/password "{\"currentPassword\":\"admin\",\"newPassword\":\"$NEWPASS\"}")" "change password"
expect 200 "$(login_retry admin "$NEWPASS")" "login with new password"
expect 200 "$(api GET /api/capabilities)" "capabilities after change"
[ "$(jq -r '.mustChangePassword // false' "$BODY")" = "false" ] || fail "mustChangePassword should be false now"
[ "$(jq -r '.virtualServiceInstalled' "$BODY")" = "true" ] || fail "Istio CRDs not detected"
[ "$(jq -r '.httpRouteInstalled' "$BODY")" = "true" ] || fail "Gateway API CRDs not detected"
jq -e '.appVersion | type=="string" and length>0' "$BODY" >/dev/null || fail "appVersion missing from capabilities"
echo "    appVersion=$(jq -r '.appVersion' "$BODY") istiod=$(jq -r '.istiodVersion // "n/a"' "$BODY")"

echo "== VirtualService dry-run → create → list → flow map → delete"
VS=$(cat <<EOF
{"apiVersion":"networking.istio.io/v1","kind":"VirtualService","metadata":{"name":"$NAME","namespace":"$NS"},
 "spec":{"hosts":["e2e.example.com"],"http":[{"route":[{"destination":{"host":"e2e.$NS.svc.cluster.local"}}]}]}}
EOF
)
expect 200 "$(api POST "/api/resources/$TYPE?dryRun=true" "$VS")" "dry-run create"
expect 201 "$(api POST "/api/resources/$TYPE" "$VS")" "create"
CREATED_VS=1
expect 200 "$(api GET "/api/resources/$TYPE")" "list"
jq -e --arg n "$NAME" --arg ns "$NS" 'map(select(.name==$n and .namespace==$ns)) | length == 1' "$BODY" >/dev/null || fail "created VirtualService missing from list"
expect 200 "$(api GET /api/flowmap)" "flow map"
expect 204 "$(api DELETE "/api/resources/$TYPE/$NS/$NAME")" "delete"
CREATED_VS=
expect 200 "$(api GET "/api/resources/$TYPE")" "list after delete"
jq -e --arg n "$NAME" --arg ns "$NS" 'map(select(.name==$n and .namespace==$ns)) | length == 0' "$BODY" >/dev/null || fail "VirtualService still listed after delete"

echo "== viewer is read-only"
CREATED_VIEWER=1
code=$(api PUT /api/accounts/e2e-viewer '{"role":"viewer","password":"Viewer-Pass-2026!"}')
{ [ "$code" = 200 ] || [ "$code" = 204 ]; } || fail "create viewer account: got $code: $(head -c 300 "$BODY")"
echo "ok  create viewer account ($code)"
expect 200 "$(login_retry e2e-viewer 'Viewer-Pass-2026!')" "login as viewer"
expect 403 "$(api POST "/api/resources/$TYPE?dryRun=true" "$VS")" "viewer write is forbidden"
echo "admin password is now: $NEWPASS (fresh-install e2e; change it or reinstall)"
echo "ALL OK"
