# Operations

## High availability

Sealane keeps no state of its own. Accounts live in the accounts ConfigMap, remote clusters in the clusters Secret, and sessions in signed cookies, so replicas are interchangeable and you scale with `replicaCount`. Two replicas plus the shutdown delay (`SHUTDOWN_DELAY`, default `5s`) keep requests flowing through a rolling update (the old pod keeps serving for 5 s after it leaves the Service), so set `replicaCount: 2` for HA.

Every replica must sign sessions with the **same key**, otherwise a cookie issued by one pod is rejected by another (and an SSO sign-in that comes back to a different pod fails with `oidc_error=state`):

- **Helm** — `sessionSecret.create=true` (the default) generates a random key once into the `<release>-session` Secret and keeps it across upgrades, so sessions also survive restarts.
- **GitOps (Argo CD, Flux) or `helm template`** — these cannot use Helm's `lookup` and would render a new key on every sync. Create a Secret yourself (key `session-secret`) and set `sessionSecret.existingSecret`.
- **`install.yaml`** — rendered with `sessionSecret.create=false`, so each pod uses a random per-boot key: a restart signs everyone out, and you should stay at one replica.

The change history on the home page is in memory per pod (last 200 entries) and resets on restart; with several replicas each pod shows the changes it served. The structured audit log (below) is the durable record.

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

Use `helm upgrade` with the new `--version`. Release-specific steps (new values keys, renamed resources) are in [Upgrading](upgrading.md); read it before crossing a minor version. The accounts ConfigMap and the clusters Secret are not managed by Helm, so upgrades never touch them.

## Backup

Two objects hold everything worth keeping, both in Sealane's namespace:

```bash
kubectl -n sealane get configmap sealane-accounts -o yaml > sealane-accounts.yaml   # local accounts and roles
kubectl -n sealane get secret sealane-clusters -o yaml > sealane-clusters.yaml      # remote cluster kubeconfigs (credentials — store it like one)
```

The session Secret (`<release>-session`) only keeps people signed in; losing it signs everyone out and nothing else. Routing resources themselves live in the clusters, not in Sealane. If you use SSO, the IdP client and the client-secret Secret are part of your IdP configuration.

## Uninstall

`helm uninstall sealane -n sealane` removes the release. The accounts ConfigMap and clusters Secret survive — delete them too for a full cleanup.
