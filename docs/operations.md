# Operations

## High availability

Sealane keeps no state of its own. Accounts live in the accounts ConfigMap, remote clusters in the clusters Secret, and sessions in signed cookies, so replicas are interchangeable and you scale with `replicaCount`. Two replicas plus the shutdown delay (`SHUTDOWN_DELAY`, default `5s`) keep requests flowing through a rolling update (the old pod keeps serving for 5 s after it leaves the Service), so set `replicaCount: 2` for HA.

Every replica must sign sessions with the **same key**, otherwise a cookie issued by one pod is rejected by another (and an SSO sign-in that comes back to a different pod fails with `oidc_error=state`):

- **Helm** — `sessionSecret.create=true` (the default) generates a random key once into the `<release>-session` Secret and keeps it across upgrades, so sessions also survive restarts.
- **`install.yaml`, GitOps (Argo CD, Flux) or `helm template`** — no `lookup` there, so no key is rendered; Sealane generates one on first start and stores it in the `sealane-secret` Secret (key `SESSION_SECRET`), like Argo CD's `argocd-secret`. Every pod reads that Secret, so restarts and replicas share the key. Set `sessionSecret.existingSecret` (key `session-secret`) to bring your own instead.

The change history lives in ConfigMap `sealane-history` and Secret `sealane-history-objects` (`<release>-history…` with Helm) in Sealane's namespace, so replicas share it and restarts keep it. The list holds the last 200 changes; the objects Secret keeps before/after versions up to about 768 KiB and drops the oldest first, so older entries can lose their restore content. Versions 1.15 to 1.18 kept the objects in a ConfigMap of the same name; 1.19 moves them into the Secret at startup and deletes the ConfigMap. Deleting any of them only clears the history. The structured audit log (below) stays the record to ship to your log system.

## Metrics

`/metrics` serves Prometheus metrics: `http_requests_total` (by `method`, `status`), `http_request_duration_seconds` (by `method`), `sealane_build_info` (by `version`, always 1), `sealane_history_write_failures_total`, `sealane_login_failures_total` (by `reason`), `sealane_account_read_errors_total` and the standard Go and process collectors. With the Prometheus Operator, set `serviceMonitor.enabled=true` and the chart creates a `ServiceMonitor` that scrapes `/metrics` on the `http` port.

Alerts worth having, as a starting point:

```yaml
groups:
  - name: sealane
    rules:
      - alert: SealaneDown
        expr: absent(up{job="sealane"} == 1)
        for: 5m
      - alert: SealaneServerErrors          # the API server or a remote cluster is failing Sealane's calls
        expr: sum(rate(http_requests_total{job="sealane",status=~"5.."}[5m])) > 0.1
        for: 10m
      - alert: SealaneHistoryNotSaved       # changes go through, but their history is lost (RBAC, size, API errors)
        expr: increase(sealane_history_write_failures_total{job="sealane"}[15m]) > 0
      - alert: SealaneSignInFailures        # someone guessing passwords, or a broken SSO setup
        expr: sum(increase(sealane_login_failures_total{job="sealane"}[10m])) > 20
      - alert: SealaneCannotReadAccounts    # account changes then reach Sealane only through the mounted copy
        expr: increase(sealane_account_read_errors_total{job="sealane"}[10m]) > 5
```

`job` is whatever your scrape config names it. The log message for a history failure says why (`change history not saved`).

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

## Backup and restore

Routing resources live in the clusters, not in Sealane. What Sealane keeps is in its own namespace (names for release `sealane`; with Helm they follow `<release>-…`):

| Object | Holds | Without it |
|---|---|---|
| ConfigMap `sealane-accounts` | local accounts, roles, bcrypt hashes | no one can sign in locally; a new `admin` with a new password is made |
| Secret `sealane-clusters` | remote cluster kubeconfigs (credentials) | remote clusters must be registered again |
| ConfigMap `sealane-cm` | `OIDC_*` and other settings (`install.yaml`; Helm renders it from values) | SSO is off |
| Secret `sealane-secret` | the generated session key, and `OIDC_CLIENT_SECRET` if you put it there | everyone signs in again; SSO fails without its client secret |
| Secret `sealane-session` | the session key with Helm's `sessionSecret.create` | everyone signs in again |
| ConfigMap `sealane-tls-certs` | private CA certificates | SSO to a privately signed IdP fails |
| ConfigMap `sealane-history`, Secret `sealane-history-objects` | change history | the history starts empty |
| ConfigMap `sealane-history-objects` | objects of revisions recorded by 1.15–1.18, until 1.19 moves them into the Secret | those revisions can no longer be compared or restored |
| Secret `sealane-initial-admin` | the generated first admin password | if it was never changed, no one knows the admin password |

Secrets you named yourself are not in the list: `sessionSecret.existingSecret` and `oidc.clientSecretRef.name`. Back them up with your other credentials. With Helm, also keep your values: `helm get values sealane -n sealane -o yaml > sealane-values.yaml`.

Back up:

```bash
kubectl -n sealane get configmap sealane-accounts sealane-cm sealane-tls-certs sealane-history sealane-history-objects \
  --ignore-not-found -o yaml > sealane-configmaps.yaml
kubectl -n sealane get secret sealane-clusters sealane-secret sealane-session sealane-history-objects sealane-initial-admin \
  --ignore-not-found -o yaml > sealane-secrets.yaml   # credentials: store it encrypted
```

Restore into a new cluster, before installing Sealane there, so it finds them on first start:

```bash
kubectl create namespace sealane
# drop the old object identity so the objects can be created again
grep -vE '^    (resourceVersion|uid|creationTimestamp):' sealane-configmaps.yaml | kubectl create -f -
grep -vE '^    (resourceVersion|uid|creationTimestamp):' sealane-secrets.yaml | kubectl create -f -
# then install as usual: helm install ... -f sealane-values.yaml, or kubectl apply -f install.yaml
```

Objects that came from a Helm install carry Helm's ownership annotations, so `helm install` with the same release name and namespace adopts them. Check the result: sign in with an existing account, open **Settings** (registered clusters show *Connected*), and in a resource's **History** tab compare an older revision with the current object.

Restoring onto the running install instead (to roll back an accidental edit) is `kubectl apply -f` of the same files after removing the same three fields; Sealane picks up accounts within seconds and clusters on the next request.

## Uninstall

`helm uninstall sealane -n sealane` (or `kubectl delete -f install.yaml`) removes Sealane. What it created at run time stays: the accounts ConfigMap, the clusters, session, initial-admin and history Secrets and the history ConfigMap. Since Sealane lives in a namespace of its own, `kubectl delete namespace sealane` removes all of it. Remote clusters registered with the dedicated account keep that account until you delete each registration with the account removal option first (see [Multi-cluster](multi-cluster.md)).
