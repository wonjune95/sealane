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

## v1.0 — Policy and ecosystem (shipped in 1.0.0)
- Versioning and support policy, governance and adopters files.
- End-to-end tests against two Istio versions, and an upgrade test from 0.8 to 1.0.
- Release notes kept in the repository.
- Ecosystem submission kit (Artifact Hub, announcement, integration page and Landscape drafts).

## 1.1 (shipped)
- Mesh analysis: cross-resource checks for routes, subsets, gateways and selectors that lead nowhere.
- Service view: everything that routes to, configures or secures one Service.
- Status pills in resource lists from Gateway API conditions and Istio validation messages.
- Local login throttle.
- IdP groups in the `login` log line.
- Option to hide the local login form when SSO is configured (`oidc.hideLocalLogin`).

## 1.2 (shipped)
- Sidebar gives its height back to the resource-kind list; the header cluster picker shows a connection dot and links to cluster management.
- Home cards for mesh analysis (error and warning counts) and the Services view.
- Fixes for the severity pill wrapping in Korean and toolbar search boxes at narrow widths.

## 1.3 (shipped)
- Curated forms for VirtualService and DestinationRule.
- One-line list summaries for every managed kind.
- Sortable, paged lists.

## 1.4 (shipped)
- One concurrent snapshot per request for the flow map, analysis, service view and route check; Endpoints read once; client rate limit raised.
- Services list with per-Service counts of routes, destination rules, policies and findings.

## 1.5 (shipped)
- Sidecar-injection check in the analysis (native sidecars included).
- Findings open the offending field in the editor.
- "Only services with mesh config" filter on the Services list.

## 1.6 (shipped)
- A remediation hint under every analysis finding.
- Service picker in the request console that fills the URL and port.
- "New VirtualService / DestinationRule for this service" from the Service page.

## 1.7 (shipped)
- Analysis flags a VirtualService destination port the Service does not expose.
- The VirtualService created from the Service page carries the Service port.
- Page kickers translated.

## 1.8 (shipped)
- Analysis flags an HTTPRoute backendRef port the Service does not expose (Istio resolves the ref and still answers 503).
- "New HTTPRoute for this service" from the Service page.

## 1.9 (shipped)
- Sealane's own server error messages carry a stable `code`; the UI shows them in the active language.

## 1.10 (shipped)
- Chart `accounts` value seeds the local accounts ConfigMap from Helm values (GitOps installs).

## 1.11 (shipped)
- The chart and install.yaml ship the accounts ConfigMap and a `sealane-cm` settings ConfigMap (OIDC keys documented in comments, read through `envFrom`), like Argo CD's `argocd-cm`.

## After 1.0 (candidates, not promised)
- Recipes and wizards for common routing changes.
- Revision history with rollback.
- OIDC refresh and logout at the IdP.
- Metrics overlay on the flow map.
- Conversion from Istio routing to Gateway API.
- Bulk export and import.
- Kubernetes impersonation mode: act as the signed-in user instead of the ServiceAccount.
- Korean translation of the docs site.
- Gateway API policy kinds as they reach the standard channel.

## Not planned
- CLI, operator/CRDs, Kustomize overlays, real-time streaming, bundled Dex. Open an issue if you need one of these and say why.
