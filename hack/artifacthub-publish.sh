#!/usr/bin/env bash
# Pushes artifacthub-repo.yml as the Artifact Hub metadata artifact for the chart's OCI repository.
# Needs: oras, a GHCR login with packages:write (docker login ghcr.io), repositoryID filled in.
set -euo pipefail
cd "$(dirname "$0")/.."
grep -q '^repositoryID: "[^"]\+"' artifacthub-repo.yml || { echo "fill repositoryID in artifacthub-repo.yml first" >&2; exit 1; }
oras push ghcr.io/wonjune95/charts/sealane:artifacthub.io \
  --config /dev/null:application/vnd.cncf.artifacthub.config.v1+yaml \
  artifacthub-repo.yml:application/vnd.cncf.artifacthub.repository-metadata.layer.v1.yaml
echo "pushed; Artifact Hub re-indexes within ~30 minutes"
