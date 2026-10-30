#!/usr/bin/env bash
# End-to-end check of impersonation mode, run after hack/e2e.sh on an install upgraded with
# impersonation.enabled=true: writes must run as sealane:admin, so the default binding lets them
# through and removing it makes the cluster (not Sealane) refuse them.
# Usage: hack/e2e-impersonation.sh BASE_URL NAMESPACE ADMIN_PASSWORD [RELEASE]
# Needs: curl, jq, kubectl.
set -euo pipefail
BASE=${1:?base url}
NS=${2:?namespace}
PASS=${3:?admin password}
REL=${4:-sealane}
TYPE=virtualservices.networking.istio.io
NAME=sealane-e2e-imp
JAR=$(mktemp)
BODY=$(mktemp)

fail() { echo "FAIL: $*" >&2; exit 1; }
api() {
  local m=$1 p=$2 b=${3:-}
  if [ -n "$b" ]; then
    curl -s --max-time 20 -o "$BODY" -w '%{http_code}' -b "$JAR" -c "$JAR" -X "$m" -H 'Content-Type: application/json' -d "$b" "$BASE$p" || true
  else
    curl -s --max-time 20 -o "$BODY" -w '%{http_code}' -b "$JAR" -c "$JAR" -X "$m" "$BASE$p" || true
  fi
}
expect() { local want=$1 got=$2 what=$3; [ "$got" = "$want" ] || fail "$what: expected HTTP $want, got $got: $(head -c 300 "$BODY")"; echo "ok  $what ($got)"; }

VS=$(cat <<JSON
{"apiVersion":"networking.istio.io/v1","kind":"VirtualService","metadata":{"name":"$NAME","namespace":"$NS"},
 "spec":{"hosts":["imp.example.com"],"http":[{"route":[{"destination":{"host":"imp.$NS.svc.cluster.local"}}]}]}}
JSON
)

echo "== impersonation: default binding allows writes"
expect 200 "$(api POST /api/login "{\"username\":\"admin\",\"password\":\"$PASS\"}")" "login admin"
expect 201 "$(api POST "/api/resources/$TYPE" "$VS")" "create as sealane:admin"
expect 200 "$(api GET "/api/access/$TYPE/$NS/$NAME?verbs=update,delete")" "access check"
jq -e '.update == true and .delete == true' "$BODY" >/dev/null || fail "access should allow update and delete: $(cat "$BODY")"
expect 204 "$(api DELETE "/api/resources/$TYPE/$NS/$NAME")" "delete as sealane:admin"

echo "== impersonation: without the binding the cluster refuses"
kubectl delete clusterrolebinding "$REL-writer" >/dev/null
code=
for _ in $(seq 1 30); do # the RBAC authorizer's cache needs a moment
  code=$(api POST "/api/resources/$TYPE?dryRun=true" "$VS")
  [ "$code" = 403 ] && break
  sleep 1
done
expect 403 "$code" "create refused by cluster RBAC"
grep -q 'sealane:admin' "$BODY" || fail "the refusal should name the impersonated user: $(head -c 300 "$BODY")"
expect 200 "$(api GET "/api/access/$TYPE/$NS/-?verbs=create")" "access check without rights"
jq -e '.create == false' "$BODY" >/dev/null || fail "access should deny create: $(cat "$BODY")"
expect 200 "$(api GET "/api/resources/$TYPE")" "reads still work (ServiceAccount)"
echo "ALL OK"
