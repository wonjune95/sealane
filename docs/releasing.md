# Releasing

1. Make sure `main` is green (ci, e2e, codeql).
2. Bump `charts/sealane/Chart.yaml` `version` and `appVersion` to the new version and commit.
3. Tag and push: `git tag vX.Y.Z && git push origin vX.Y.Z`.
4. The `release` workflow builds the multi-arch image, signs it, attaches an SBOM, pushes the chart, renders `install.yaml` and creates the GitHub Release with auto-generated notes (categories come from `.github/release.yml` labels).
5. Verify:
   ```bash
   cosign verify --certificate-identity-regexp 'https://github.com/wonjune95/sealane/' \
     --certificate-oidc-issuer https://token.actions.githubusercontent.com ghcr.io/wonjune95/sealane:X.Y.Z
   skopeo inspect --raw docker://ghcr.io/wonjune95/sealane:X.Y.Z | jq '.manifests[].platform.architecture'   # amd64, arm64
   helm show chart oci://ghcr.io/wonjune95/charts/sealane --version X.Y.Z
   ```
6. Artifact Hub re-indexes the OCI repository within about 30 minutes.

Images are signed keyless (GitHub OIDC identity via Sigstore), so there is no public key to publish. The chart's `artifacthub.io/signKey` annotation points to this page for that reason.
