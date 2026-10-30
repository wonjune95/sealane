#!/usr/bin/env bash
# Writes THIRD_PARTY_NOTICES (or the file given) for a local build: the license text of every Go
# module and npm package that ships in Sealane. The image build makes the same file (Dockerfile),
# and each release attaches it. Needs: go, node, web/node_modules (npm ci).
set -euo pipefail
cd "$(dirname "$0")/.."
out=${1:-THIRD_PARTY_NOTICES}
{
  echo "Sealane (MIT, see LICENSE) includes the third-party software below, each under its own license."
  echo
  hack/go-notices.sh
  echo
  (cd web && node ../hack/npm-notices.mjs)
} >"$out"
echo "wrote $out ($(wc -c <"$out") bytes)"
