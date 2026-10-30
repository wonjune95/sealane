#!/usr/bin/env bash
# Upgrade check: run "pre" against a fresh install of an older release (0.8.0 … 1.18.x, which start
# with admin/admin), upgrade the chart, run "post"; after a rollback, run "rollback".
# Usage: hack/upgrade-check.sh pre|post|rollback http://localhost:8080 [state-file]
# pre     : logs in as admin/admin, sets a new password, creates a viewer account, writes the state file.
# post    : logs in with the state file's passwords (admin and viewer), checks appVersion changed and the viewer is still read-only.
# rollback: the same accounts work on the old version again and appVersion is the old one.
set -euo pipefail
PHASE=${1:?pre|post}; BASE=${2:?base url}; STATE=${3:-/tmp/sealane-upgrade-state}
JAR=$(mktemp); BODY=$(mktemp); trap 'rm -f "$JAR" "$BODY"' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }
api() { local m=$1 p=$2 b=${3:-}; : >"$BODY"
  if [ -n "$b" ]; then curl -s --max-time 20 -o "$BODY" -w '%{http_code}' -b "$JAR" -c "$JAR" -X "$m" -H 'Content-Type: application/json' -d "$b" "$BASE$p" || true
  else curl -s --max-time 20 -o "$BODY" -w '%{http_code}' -b "$JAR" -c "$JAR" -X "$m" "$BASE$p" || true; fi; }
expect() { [ "$2" = "$1" ] || fail "$3: expected HTTP $1, got $2: $(head -c 300 "$BODY")"; echo "ok  $3 ($2)"; }
# Releases before 1.19 see a password change only when the mounted ConfigMap refreshes (about a
# minute), and failed tries count against the sign-in throttle (429 for a minute): wait it out.
login_retry() { local code; for _ in $(seq 1 24); do code=$(api POST /api/login "{\"username\":\"$1\",\"password\":\"$2\"}"); [ "$code" = 200 ] && break; if [ "$code" = 429 ]; then sleep 61; else sleep 10; fi; done; echo "$code"; }

case "$PHASE" in
pre)
  ADMIN_PASS="Up-Admin-$(date +%s)!"; VIEWER_PASS="Up-Viewer-$(date +%s)!"
  expect 200 "$(login_retry admin admin)" "login admin/admin (0.8.0)"
  expect 204 "$(api POST /api/account/password "{\"currentPassword\":\"admin\",\"newPassword\":\"$ADMIN_PASS\"}")" "change admin password"
  expect 200 "$(login_retry admin "$ADMIN_PASS")" "login with new admin password"
  code=$(api PUT /api/accounts/upgrade-viewer "{\"role\":\"viewer\",\"password\":\"$VIEWER_PASS\"}")
  { [ "$code" = 200 ] || [ "$code" = 204 ]; } || fail "create viewer: $code $(head -c 300 "$BODY")"
  expect 200 "$(api GET /api/capabilities)" "capabilities before upgrade"
  OLD=$(jq -r '.appVersion // ""' "$BODY")
  printf 'ADMIN_PASS=%s\nVIEWER_PASS=%s\nOLD_VERSION=%s\n' "$ADMIN_PASS" "$VIEWER_PASS" "$OLD" >"$STATE"
  echo "pre OK (appVersion before: ${OLD:-n/a})" ;;
post)
  # shellcheck disable=SC1090
  . "$STATE"
  expect 200 "$(login_retry admin "$ADMIN_PASS")" "admin password survived the upgrade"
  expect 200 "$(api GET /api/capabilities)" "capabilities after upgrade"
  NEW=$(jq -r '.appVersion // ""' "$BODY"); [ -n "$NEW" ] && [ "$NEW" != "$OLD_VERSION" ] || fail "appVersion did not change: '$OLD_VERSION' -> '$NEW'"
  [ "$(jq -r '.mustChangePassword // false' "$BODY")" = "false" ] || fail "admin forced to change password again after upgrade"
  expect 200 "$(login_retry upgrade-viewer "$VIEWER_PASS")" "viewer account survived the upgrade"
  expect 403 "$(api POST '/api/resources/virtualservices.networking.istio.io?dryRun=true' '{"apiVersion":"networking.istio.io/v1","kind":"VirtualService","metadata":{"name":"x","namespace":"default"},"spec":{"hosts":["x"]}}')" "viewer still read-only"
  echo "post OK (appVersion: $OLD_VERSION -> $NEW)" ;;
rollback)
  # shellcheck disable=SC1090
  . "$STATE"
  expect 200 "$(login_retry admin "$ADMIN_PASS")" "admin password survived the rollback"
  expect 200 "$(api GET /api/capabilities)" "capabilities after rollback"
  NEW=$(jq -r '.appVersion // ""' "$BODY"); [ "$NEW" = "$OLD_VERSION" ] || fail "appVersion after rollback: '$NEW', want '$OLD_VERSION'"
  expect 200 "$(login_retry upgrade-viewer "$VIEWER_PASS")" "viewer account survived the rollback"
  expect 403 "$(api POST '/api/resources/virtualservices.networking.istio.io?dryRun=true' '{"apiVersion":"networking.istio.io/v1","kind":"VirtualService","metadata":{"name":"x","namespace":"default"},"spec":{"hosts":["x"]}}')" "viewer still read-only"
  echo "rollback OK (appVersion: $NEW)" ;;
*) fail "phase must be pre, post or rollback" ;;
esac
