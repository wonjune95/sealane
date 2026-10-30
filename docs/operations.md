# Operations

## High availability

Sealane keeps no state of its own. Accounts live in the accounts ConfigMap, remote clusters in the clusters Secret, and sessions in signed cookies, so replicas are interchangeable and you scale with `replicaCount`. Two replicas plus the shutdown delay (`SHUTDOWN_DELAY`, default `5s`) keep requests flowing through a rolling update (the old pod keeps serving for 5 s after it leaves the Service), so set `replicaCount: 2` for HA.

Every replica must sign sessions with the **same key**, otherwise a cookie issued by one pod is rejected by another (and an SSO sign-in that comes back to a different pod fails with `oidc_error=state`):

- **Helm** — `sessionSecret.create=true` (the default) generates a random key once into the `<release>-session` Secret and keeps it across upgrades, so sessions also survive restarts.
- **`install.yaml`, GitOps (Argo CD, Flux) or `helm template`** — no `lookup` there, so no key is rendered; Sealane generates one on first start and stores it in the `sealane-secret` Secret (key `SESSION_SECRET`), like Argo CD's `argocd-secret`. Every pod reads that Secret, so restarts and replicas share the key. Set `sessionSecret.existingSecret` (key `session-secret`) to bring your own instead.

The change history lives in the `sealane-history` and `sealane-history-objects` ConfigMaps (`<release>-history…` with Helm) in Sealane's namespace, so replicas share it and restarts keep it. The list holds the last 200 changes; the objects ConfigMap keeps before/after versions up to about 768 KiB and drops the oldest first, so older entries can lose their restore content. Back them up with the namespace if you need the history after a reinstall; deleting them only clears the history. The structured audit log (below) stays the record to ship to your log system.

## Metrics

`/metrics` serves Prometheus metrics: `http_requests_total` (by `method`, `status`), `http_request_duration_seconds` (by `method`) and the standard Go and process collectors. With the Prometheus Operator, set `serviceMonitor.enabled=true` and the chart creates a `ServiceMonitor` that scrapes `/metrics` on the `http` port.

Probes: `/healthz` (liveness) and `/readyz` (readiness — pings the Kubernetes API server with a 2 s timeout and returns 503 `not ready` on failure). The chart wires both.

## Logs

Logs are JSON lines on stdout (`slog`). The useful messages:

| `msg` | When | Fields |
|---|---|---|
| `request` | every HTTP request | `method`, `path`, `status`, `dur_ms` |
| `resource mutated` | a create/update/delete through Sealane succeeded | `audit=true`, `dangerous`, `cluster`, `user`, `verb`, `type`, `namespace`, `name` |
| `HIGH-RISK resource mutated` (warn) | same, for a high-risk kind | same, `dangerous=true` |
| `resource mutation failed` (error) | the API server rejected a write | same, plus `err` |
| `login` | successful local or SSO sign-in | `user`, `role` (and `provider` for SSO) |
| `login failed` (warn) | wrong local username or password | `user` |
| `oidc callback failed` (warn) | an SSO sign-in failed | `reason` (`state`, `access_denied`, `idp`), `err` |

Ship them with your usual log collector and alert on `dangerous=true`. Dry runs are not logged as mutations.

## Upgrades

Use `helm upgrade` with the new `--version`. Release-specific steps (new values keys, renamed resources) are in [Upgrading](upgrading.md); read it before crossing a minor version. Upgrades never touch existing accounts (the chart renders the accounts ConfigMap without data and keeps it on uninstall) or the clusters Secret (not managed by Helm).

## Backup

Two objects hold everything worth keeping, both in Sealane's namespace:

```bash
kubectl -n sealane get configmap sealane-accounts -o yaml > sealane-accounts.yaml   # local accounts and roles
kubectl -n sealane get secret sealane-clusters -o yaml > sealane-clusters.yaml      # remote cluster kubeconfigs (credentials — store it like one)
```

The session Secret (`<release>-session`) only keeps people signed in; losing it signs everyone out and nothing else. Routing resources themselves live in the clusters, not in Sealane. If you use SSO, the IdP client and the client-secret Secret are part of your IdP configuration.

## Uninstall

`helm uninstall sealane -n sealane` removes the release. The accounts ConfigMap and clusters Secret survive — delete them too for a full cleanup.
