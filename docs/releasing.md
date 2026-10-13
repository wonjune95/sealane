# Releasing

Before the first release: enable GitHub Pages once: Settings → Pages → Build and deployment → Source: GitHub Actions.

1. Make sure `main` is green (ci, e2e, codeql).
2. Bump `charts/sealane/Chart.yaml` `version` and `appVersion` to the new version, plus the spots `helm package --version` does not override: the `artifacthub.io/images` annotation in `charts/sealane/Chart.yaml` and every `--version X.Y.Z` / image-tag line (`git grep -n 'OLD\.VERSION' -- README*.md charts docs ':!docs/releases' ':!docs/upgrading.md'` finds them). Write `docs/releases/vX.Y.Z.md` (highlights, upgrade notes, install block) and add the page to the "Release notes" section of `mkdocs.yml`. The release step fails without the file. Commit.
3. Tag and push: `git tag vX.Y.Z && git push origin vX.Y.Z`.
4. The `release` workflow builds the multi-arch image, signs it, attaches an SBOM, pushes the chart, renders `install.yaml` and creates the GitHub Release with the notes file followed by auto-generated PR/contributor lines (categories come from `.github/release.yml` labels).
5. Verify:
   ```bash
   cosign verify --certificate-identity-regexp 'https://github.com/wonjune95/sealane/' \
     --certificate-oidc-issuer https://token.actions.githubusercontent.com ghcr.io/wonjune95/sealane:X.Y.Z
   skopeo inspect --raw docker://ghcr.io/wonjune95/sealane:X.Y.Z | jq -r '.manifests[] | select(.platform.os != "unknown") | .platform.architecture'   # amd64, arm64
   helm show chart oci://ghcr.io/wonjune95/charts/sealane --version X.Y.Z
   ```
   The multi-arch index also carries two `unknown/unknown` entries: the provenance attestations build-push-action attaches. The filter above skips them.
6. Artifact Hub re-indexes the OCI repository within about 30 minutes.
7. Verified publisher: `hack/artifacthub-publish.sh` (once, and again whenever `artifacthub-repo.yml` changes).

The image and the chart (`ghcr.io/wonjune95/charts/sealane:X.Y.Z`) are both signed keyless (GitHub OIDC identity via Sigstore), so there is no public key to publish; verify the chart with the same `cosign verify` flags.
