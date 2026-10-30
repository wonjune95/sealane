#!/usr/bin/env bash
# End-to-end check of cluster registration with a dedicated account: registers the cluster Sealane
# runs in as a "remote" cluster from an admin kubeconfig and checks that only the generated
# account's token is stored and that Sealane works through it.
# Usage: hack/e2e-cluster-add.sh BASE_URL ADMIN_PASSWORD ADMIN_KUBECONFIG [NAMESPACE]
# The kubeconfig's server must be reachable from the Sealane pod (https://kubernetes.default.svc on kind).
# Needs: curl, jq, kubectl.
set -euo pipefail
BASE=${1:?base url}
PASS=${2:?admin password}
KC=${3:?admin kubeconfig file}
NS=${4:-sealane}
JAR=$(mktemp)
BODY=$(mktemp)
fail() { echo "FAIL: $*" >&2; exit 1; }
api() {
  local m=$1 p=$2 b=${3:-}
  if [ -n "$b" ]; then
    curl -s --max-time 60 -o "$BODY" -w '%{http_code}' -b "$JAR" -c "$JAR" -X "$m" -H 'Content-Type: application/json' -d "$b" "$BASE$p" || true
  else
    curl -s --max-time 60 -o "$BODY" -w '%{http_code}' -b "$JAR" -c "$JAR" -X "$m" "$BASE$p" || true
  fi
}
expect() { local want=$1 got=$2 what=$3; [ "$got" = "$want" ] || fail "$what: expected HTTP $want, got $got: $(head -c 300 "$BODY")"; echo "ok  $what ($got)"; }

echo "== cluster registration with a dedicated account"
expect 200 "$(api POST /api/login "{\"username\":\"admin\",\"password\":\"$PASS\"}")" "login admin"
REQ=$(jq -n --rawfile kc "$KC" '{kubeconfig: $kc, setup: true}')
expect 200 "$(api PUT /api/clusters/self "$REQ")" "register with setup"
jq -e '.connected == true and .account == "sealane-remote/sealane"' "$BODY" >/dev/null || fail "registration result: $(cat "$BODY")"
kubectl -n sealane-remote get serviceaccount sealane >/dev/null || fail "account not created"
STORED=$(kubectl -n "$NS" get secret sealane-clusters -o jsonpath='{.data.self}' | base64 -d)
grep -q 'token:' <<<"$STORED" || fail "stored kubeconfig has no token"
! grep -q 'client-key-data' <<<"$STORED" || fail "the admin client key was stored"
echo "ok  only the generated account's token is stored"
expect 200 "$(api GET "/api/resources/virtualservices.networking.istio.io?cluster=self")" "list through the account"
expect 200 "$(api PUT /api/clusters/self "$REQ")" "register again (idempotent)"
expect 204 "$(api DELETE /api/clusters/self)" "delete registration"
echo "ALL OK"
