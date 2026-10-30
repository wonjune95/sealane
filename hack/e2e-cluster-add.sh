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
expect 200 "$(api GET /api/clusters)" "list clusters"
jq -e 'map(select(.name=="self" and .dedicated==true)) | length == 1' "$BODY" >/dev/null || fail "self should be listed as dedicated: $(cat "$BODY")"
expect 204 "$(api DELETE '/api/clusters/self?cleanup=true')" "delete registration and Sealane's account"
! kubectl get clusterrole sealane-remote >/dev/null 2>&1 || fail "cluster role sealane-remote left behind"
! kubectl get clusterrolebinding sealane-remote >/dev/null 2>&1 || fail "cluster role binding sealane-remote left behind"
for _ in $(seq 1 60); do kubectl get namespace sealane-remote >/dev/null 2>&1 || break; sleep 2; done
! kubectl get namespace sealane-remote >/dev/null 2>&1 || fail "namespace sealane-remote not removed"
echo "ok  Sealane's account removed from the cluster"
REQ2=$(jq -n --rawfile kc "$KC" '{kubeconfig: $kc, setup: false}')
expect 200 "$(api PUT /api/clusters/pasted "$REQ2")" "register with the pasted credentials"
expect 502 "$(api DELETE '/api/clusters/pasted?cleanup=true')" "cleanup refused without a Sealane account"
expect 204 "$(api DELETE /api/clusters/pasted)" "delete registration"
echo "ALL OK"
