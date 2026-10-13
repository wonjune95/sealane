# Roadmap

Sealane is moving from an internal tool to a public project in four releases. Dates are not promised; each release ships when it is ready.

## v0.8 — Sealane (shipped in 0.8.0)
- Rename from Istio Dashboard, English UI with a Korean locale, contributor files.

## v0.9 — Install anywhere (shipped in 0.9.0)
- Multi-arch images (amd64/arm64), signed with cosign, SBOM attached.
- `kubectl apply -f install.yaml` from GitHub Releases, Artifact Hub listing.
- Chart: values schema, README, NOTES, optional Ingress/HTTPRoute, stable session secret.
- Real `/readyz`, app version in the UI, lint/vulnerability checks, end-to-end test on kind + Istio.

## v0.10 — SSO (shipped in 0.10.0)
- OIDC login (any provider; Keycloak, Dex and Google examples) with group-to-role mapping. Local accounts stay.
- Documentation site at https://wonjune95.github.io/sealane/.

## v1.0 — Policy and ecosystem (shipped in 1.0.0, this release)
- Versioning and support policy, governance and adopters files.
- End-to-end tests against two Istio versions, and an upgrade test from 0.8 to 1.0.
- Release notes kept in the repository.
- Ecosystem submission kit (Artifact Hub, announcement, integration page and Landscape drafts).

## After 1.0 (candidates, not promised)
- Kubernetes impersonation mode: act as the signed-in user instead of the ServiceAccount.
- Error codes for server messages, so the UI can translate them.
- Korean translation of the docs site.
- OIDC refresh and logout at the IdP.
- Gateway API policy kinds as they reach the standard channel.

## Not planned
- CLI, operator/CRDs, Kustomize overlays, real-time streaming, bundled Dex. Open an issue if you need one of these and say why.
