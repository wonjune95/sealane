# Upgrading

## Upgrading to the current version

From any 1.x release you can go straight to the latest; there is no need to step through each minor version. Back up first (see [Backup and restore](operations.md#backup-and-restore)), then:

1. **Coming from 1.10 or earlier with Helm:** adopt the accounts ConfigMap once, as described in [From 1.10.x to 1.11.x](#from-110x-to-111x). Without it the upgrade stops with `invalid ownership metadata` and changes nothing.
2. **Upgrade:**

   With Helm (`--reset-then-reuse-values` needs Helm 3.14 or later; with an older Helm pass your values again with `-f values.yaml` instead):

   ```bash
   helm upgrade sealane oci://ghcr.io/wonjune95/charts/sealane --version 1.19.0 -n <namespace> --reset-then-reuse-values
   ```

   With `install.yaml`:

   ```bash
   kubectl apply -f https://github.com/wonjune95/sealane/releases/download/v1.19.0/install.yaml
   ```

   **Only if you use impersonation mode** (`IMPERSONATION=true` on the Deployment), apply its RBAC after `install.yaml`. Without the mode, skip it: it makes Sealane's own role read-only, and writes would then fail.

   ```bash
   kubectl apply -f https://github.com/wonjune95/sealane/releases/download/v1.19.0/impersonation.yaml
   ```

3. **Read the section for the versions you cross**, at least [From 1.11–1.18 to 1.19](#from-111118-to-119): everyone signs in again once, and an `admin` still on the old default password must change it first.
4. **Check:** `kubectl -n <namespace> rollout status deploy/sealane`, sign in, and open **Settings** to see the version at the bottom and the registered clusters connected.

From 0.x, follow the sections below in order up to 1.0, then the steps above.

## From Istio Dashboard (≤ 0.7) to Sealane (0.8)

Sealane is the new name of Istio Dashboard. Images, charts and default resource names changed; your data did not. Accounts live in the accounts ConfigMap and remote clusters in the clusters Secret, and both survive the move.

1. Install the new release next to the old one, pointing it at the existing ConfigMap and Secret:

   ```bash
   helm install sealane oci://ghcr.io/wonjune95/charts/sealane --version 0.8.0 -n <namespace> \
     --set accountsConfigMap=istio-dashboard-accounts \
     --set clustersSecret=istio-dashboard-clusters
   kubectl -n <namespace> rollout status deploy/sealane
   ```

   Copy any custom values from your old release explicitly (for example `requestTester.enabled`, `image.*`, `resources`); `helm get values istio-dashboard -n <namespace>` shows them.

2. Point your HTTPRoute or Ingress backend at the `sealane` Service (port 8080) and open the UI. Existing accounts work unchanged; everyone signs in again once because the session cookie was renamed.

3. Remove the old release: `helm uninstall istio-dashboard -n <namespace>`. The ConfigMap and Secret were not managed by that chart and stay in place.

If you prefer the new default names, copy the data into new objects (requires `jq`) and drop the two `--set` flags:

```bash
kubectl -n <namespace> get cm istio-dashboard-accounts -o json | jq '{apiVersion, kind, metadata: {name: "sealane-accounts"}, data}' | kubectl -n <namespace> apply -f -
kubectl -n <namespace> get secret istio-dashboard-clusters -o json | jq '{apiVersion, kind, type, metadata: {name: "sealane-clusters"}, data}' | kubectl -n <namespace> apply -f -
```

Then `helm upgrade sealane oci://ghcr.io/wonjune95/charts/sealane --version 0.8.0 -n <namespace> --set accountsConfigMap=sealane-accounts --set clustersSecret=sealane-clusters` (or omit the flags) and delete the old objects once the new release is healthy.

### What else changed in 0.8

- Image: `ghcr.io/wonjune95/sealane` (the old `istio-dashboard` package stays but will not get new tags).
- Chart: `oci://ghcr.io/wonjune95/charts/sealane`, source under `charts/sealane`.
- UI language: English by default, Korean available from the header toggle (browser language is detected).
- Accounts mount path inside the pod: `/etc/sealane/accounts`.

## From 0.8.x to 0.9.x

0.9 adds new values keys (`sessionSecret`, `ingress`, `httpRoute`). `helm upgrade --reuse-values` from 0.8.0 renders fine but keeps the 0.8 shape: no session Secret (sessions still reset on restart), no Ingress or HTTPRoute. To pick up the new defaults, use `--reset-then-reuse-values` (Helm 3.14+), which resets to the new chart defaults and then reapplies your previous values, or pass your values explicitly:

```bash
helm upgrade sealane oci://ghcr.io/wonjune95/charts/sealane --version 0.9.0 -n <namespace> --reset-then-reuse-values
```

0.9 also creates a `<release>-session` Secret by default so sessions survive restarts. Set `sessionSecret.create=false` to opt out (GitOps users should set `sessionSecret.existingSecret` instead).

## From 0.9.x to 0.10.x

0.10 adds single sign-on through new `oidc.*` values. OIDC is off by default (`oidc.enabled=false`), so nothing changes unless you enable it; see [Authentication](authentication.md#single-sign-on-oidc).

```bash
helm upgrade sealane oci://ghcr.io/wonjune95/charts/sealane --version 0.10.0 -n <namespace> --reset-then-reuse-values
```

With plain `--reuse-values` the new `oidc.defaultRole` key is absent. If you then enable OIDC, users with no matching group are refused until you set `oidc.defaultRole` explicitly (for example `viewer`).

Existing sessions are invalidated once, because the session cookie format gained a provider field. Everyone signs in again after the upgrade.

## From 0.10.x to 1.0.x

1.0 has no breaking changes: values, environment variables, the accounts ConfigMap and the clusters Secret keep their format, and existing sessions stay valid. From 1.0 on, [Versioning and support](versioning.md) describes what stays stable.

```bash
helm upgrade sealane oci://ghcr.io/wonjune95/charts/sealane --version 1.0.1 -n <namespace> --reset-then-reuse-values
```

Two behaviors change:

- The new `SHUTDOWN_DELAY` (default `5s`) makes the pod wait before it closes the listener on SIGTERM, so termination takes 5 seconds longer. Set `SHUTDOWN_DELAY=0s` to turn it off (from 1.0.1 through the `extraEnv` value; on 1.0.0 with `kubectl -n <namespace> set env deploy/sealane SHUTDOWN_DELAY=0s`).
- Cookies are marked `Secure` automatically when `OIDC_REDIRECT_URL` is `https://`. If TLS terminates elsewhere and Sealane is still reached over plain http on that URL, logins would fail. Keep the redirect URL on the scheme users actually use.

## From 1.0.x to 1.1.x

1.1 has no breaking changes; values, environment variables, the accounts ConfigMap, the clusters Secret and existing sessions keep working.

```bash
helm upgrade sealane oci://ghcr.io/wonjune95/charts/sealane --version 1.1.0 -n <namespace> --reset-then-reuse-values
```

- The ClusterRole gains `pods` get/list (labels for the [mesh analysis](features.md#mesh-analysis) pod checks). The chart and `install.yaml` apply it; with a hand-maintained ClusterRole that lacks it, the analysis page skips the pod checks and says so. The request tester now adds only `pods/exec`.
- The local login form throttles repeated failures: five failed sign-ins inside ten minutes, counted per user name and per client address as reported by your proxy (the last `X-Forwarded-For` entry, else the connection address), block that key for a minute (HTTP 429). The per-user limit holds even if a client forges the header. Without a proxy header the connection address is shared by everyone behind a port-forward or a LoadBalancer without proxy protocol, so five failures by anyone block that address for a minute; and one wrong password per minute keeps a named account blocked, because the block restarts while the five failures stay inside the window.

## From 1.10.x to 1.11.x

The chart now renders the accounts ConfigMap (empty, `helm.sh/resource-policy: keep`) and a new `<release>-cm` ConfigMap that carries the single sign-on settings; the Deployment reads both through `envFrom`. Helm refuses to take over a ConfigMap it did not create, so label the existing accounts ConfigMap once before upgrading:

```bash
NS=<namespace>; CM=sealane-accounts      # or istio-dashboard-accounts
kubectl -n $NS annotate cm $CM meta.helm.sh/release-name=sealane meta.helm.sh/release-namespace=$NS
kubectl -n $NS label cm $CM app.kubernetes.io/managed-by=Helm
helm upgrade sealane oci://ghcr.io/wonjune95/charts/sealane --version 1.11.0 -n $NS --reset-then-reuse-values
```

Without the two commands the upgrade stops with `invalid ownership metadata` and changes nothing. Accounts, `oidc.*` values and `clientSecretRef` keep working unchanged; `install.yaml` users just apply the new file (it adopts the existing ConfigMap) and get `sealane-cm` to configure SSO without Helm.

## From 1.11–1.18 to 1.19

1.19 is a security release. No values change meaning and no data is migrated, but several behaviors do:

- **Everyone signs in again once.** Sessions now carry a fingerprint of the account's password, so older cookies are refused.
- **No more default password.** A fresh install creates `admin` with a random password in Secret `<release>-initial-admin` (`sealane-initial-admin` for `install.yaml`). Existing accounts are untouched; an `admin` still on `admin` / `admin` can sign in, but every other request is refused until the password is changed.
- **Account changes apply within seconds.** Sealane reads the accounts ConfigMap through the API. Deleting an account, changing its role or its password ends its sessions; changing your own password keeps the current session and ends the others.
- **Cookies and cross-site requests.** Session cookies become `Secure` when the request came over TLS or with `X-Forwarded-Proto: https`; set `secureCookies: true` if your TLS proxy sends no such header. Browsers' cross-site `POST`/`PUT`/`DELETE` are refused. If saving anything fails with HTTP 403 behind a proxy that rewrites the `Host` header, set `TRUSTED_ORIGINS` to the public URL (for example `extraEnv: [{name: TRUSTED_ORIGINS, value: "https://sealane.example.com"}]`).
- **Change history.** Revisions keep their objects in Secret `<release>-history-objects`. At startup 1.19 moves the objects that 1.15–1.18 kept in the ConfigMap of that name into the Secret and deletes the ConfigMap. Viewers now see the list of changes but not the stored versions.
- **No Role in `istio-system`.** The Istio version now comes from the istiod pods, so the chart and `install.yaml` no longer create `sealane-istiod-reader` there, and `install.yaml` applies cleanly on clusters without Istio. Helm removes the old Role; after `install.yaml` remove it yourself: `kubectl -n istio-system delete role,rolebinding sealane-istiod-reader --ignore-not-found`.
- **Remote clusters.** Registered clusters keep working. New registrations refuse kubeconfigs that point to files or use `http://`. To use **Also remove Sealane's account** when deleting a cluster registered before 1.19, register it again first: that adds the `sealane-remote-cleanup` role the removal needs. Until then the removal fails with 403; delete without the option and run the `kubectl delete` shown in [Multi-cluster](multi-cluster.md#deleting-a-cluster).
- **Impersonation mode.** Sealane's own ClusterRole becomes read-only on the managed kinds. With `install.yaml`, apply `impersonation.yaml` after `install.yaml` (it now carries that role) every time you upgrade. On remote clusters, register again to drop the dedicated account's write rights, and again whenever you turn the mode on or off.
- **Explicit kinds.** Sealane's ClusterRole (and a remote dedicated account, once registered again) lists the managed kinds by name instead of every resource in the four API groups.
- **RBAC.** The namespace Role adds `update` on Sealane's own Secrets (session key, initial admin password, history objects) and `delete` on the old history-objects ConfigMap; the chart and `install.yaml` bring them. With a hand-maintained Role that lacks them, Sealane stops at startup when it cannot read or store its session key (the log says why), instead of running with a key the other replicas do not share.
- **Upstream errors.** When a cluster refuses Sealane's own credentials (an expired remote token), the API answers 502 `cluster-auth-failed` instead of 401, so the UI shows an error instead of the sign-in page.
- **Metrics.** `sealane_build_info`, `sealane_history_write_failures_total`, `sealane_login_failures_total` and `sealane_account_read_errors_total` are new; see [Operations](operations.md#metrics).

## Rolling back

Back up before upgrading (see [Backup and restore](operations.md#backup-and-restore)); then a rollback is:

```bash
# Helm: pick the revision from helm history
helm history sealane -n <namespace>
helm rollback sealane <revision> -n <namespace>
# install.yaml: apply the previous release's file (and its impersonation.yaml if you use the mode)
kubectl apply -f https://github.com/wonjune95/sealane/releases/download/v<previous>/install.yaml
```

The accounts ConfigMap and clusters Secret have kept their format since 1.0, so any 1.x reads what a later 1.x wrote. Going back from 1.19 to 1.18 or earlier:

- Everyone signs in again once (the older session format).
- Revisions show in the list but cannot be compared or restored, because older versions read objects only from the ConfigMap and 1.19 moved them into the Secret. Going forward again brings them back.
- With impersonation mode on, register remote clusters again after the rollback: 1.19 made their dedicated accounts read-only, and older versions write as that account.
- Secret `<release>-initial-admin` is ignored; the account it seeded keeps its password.
- Remote dedicated accounts keep the cleanup role, which older versions do not use.

Rolling back past 1.11 with Helm keeps the adopted accounts ConfigMap: it has `helm.sh/resource-policy: keep`, so Helm leaves it in place.
