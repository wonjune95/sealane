# Releasing

Before the first release: enable GitHub Pages once: Settings → Pages → Build and deployment → Source: GitHub Actions.

1. Make sure `main` is green (ci, e2e, codeql). The release workflow refuses a tag whose commit has no successful `e2e` run on `main`, so push first and wait for e2e.
2. Bump `charts/sealane/Chart.yaml` `version` and `appVersion` to the new version, plus the spots `helm package --version` does not override: the `artifacthub.io/images` annotation in `charts/sealane/Chart.yaml` and every `--version X.Y.Z` / image-tag line (`git grep -n 'OLD\.VERSION' -- README*.md charts docs ':!docs/releases' ':!docs/upgrading.md'` finds them). Write `docs/releases/vX.Y.Z.md` (highlights, upgrade notes, install block) and add the page to the "Release notes" section of `mkdocs.yml`. The release step fails without the file. Commit.
3. Tag and push: `git tag vX.Y.Z && git push origin vX.Y.Z`.
4. The `release` workflow first re-runs the checks on the tagged commit (Go tests, web build and lint, i18n, Helm lint) and confirms its e2e run. It then builds the image for amd64 and scans it with Trivy: a HIGH or CRITICAL vulnerability that has a fix fails the release before anything is pushed. After that it pushes and signs the multi-arch image (version tag only), attaches an SBOM, extracts `THIRD_PARTY_NOTICES` from the image build, pushes and signs the chart, renders `install.yaml` and `impersonation.yaml`, writes `image-digest.txt` and `SHA256SUMS` over all assets and signs it (`SHA256SUMS.sigstore.json`), moves `latest` to the new digest, and creates the GitHub Release with the notes file followed by auto-generated PR/contributor lines (categories come from `.github/release.yml` labels).

   To accept a finding that has no impact, add it to `.trivyignore` with a reason and an expiry (`CVE-2026-12345 exp:2027-01-31`), so it comes back for review.
5. Verify:
   ```bash
   cosign verify --certificate-identity-regexp 'https://github.com/wonjune95/sealane/' \
     --certificate-oidc-issuer https://token.actions.githubusercontent.com ghcr.io/wonjune95/sealane:X.Y.Z
   skopeo inspect --raw docker://ghcr.io/wonjune95/sealane:X.Y.Z | jq -r '.manifests[] | select(.platform.os != "unknown") | .platform.architecture'   # amd64, arm64
   helm show chart oci://ghcr.io/wonjune95/charts/sealane --version X.Y.Z
   gh release download vX.Y.Z -p SHA256SUMS -p SHA256SUMS.sigstore.json
   cosign verify-blob --bundle SHA256SUMS.sigstore.json --certificate-identity-regexp 'https://github.com/wonjune95/sealane/' \
     --certificate-oidc-issuer https://token.actions.githubusercontent.com SHA256SUMS
   ```
   The multi-arch index also carries two `unknown/unknown` entries: the provenance attestations build-push-action attaches. The filter above skips them.
6. Artifact Hub re-indexes the OCI repository within about 30 minutes.
7. Verified publisher: `hack/artifacthub-publish.sh` (once, and again whenever `artifacthub-repo.yml` changes).

The image and the chart (`ghcr.io/wonjune95/charts/sealane:X.Y.Z`) are both signed keyless (GitHub OIDC identity via Sigstore), so there is no public key to publish; verify the chart with the same `cosign verify` flags. Use cosign 3.x: from 1.0.0 on, signatures are stored as OCI referrers (Sigstore bundles), not as `.sig` tags, and only cosign 3.x reads both forms.
