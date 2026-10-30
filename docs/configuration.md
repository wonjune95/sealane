# Configuration

Sealane is configured with a few flags and environment variables. When you install with Helm, the chart sets the environment from its values, so most people only touch the [Helm values](#helm-values). The pod also reads every key of the `sealane-cm` ConfigMap and the optional `sealane-secret` Secret as environment variables (`envFrom`), so with `install.yaml` you set the `OIDC_*` keys there and restart the pod — the ConfigMap ships with the keys listed in comments, like Argo CD's `argocd-cm`.

## Flags and environment variables

| Flag / env | Default | Description |
|---|---|---|
| `--addr` | `:8080` | listen address |
| `--version` | | print the version and exit |
| `--dev` | `false` | skip login + use local kubeconfig (**refuses to boot in-cluster**) |
| `--kubeconfig` | (default loading rules) | dev-only kubeconfig path |
| `ACCOUNTS_DIR` | `/etc/sealane/accounts` | accounts ConfigMap mount path |
| `ACCOUNTS_CONFIGMAP` / `POD_NAMESPACE` | (set by Helm) | CM that password changes / initial-admin creation patch |
| `ISTIO_NAMESPACE` | `istio-system` (set by the chart from `istioNamespace`) | namespace where istiod runs; used to show the control-plane version on the home page |
| `SESSION_SECRET` | (set by Helm from `<release>-session`) | session cookie signing key; empty = read or generate one in the `SESSION_SECRET_NAME` Secret |
| `SESSION_SECRET_NAME` | `sealane-secret` (set by the chart) | Secret where Sealane stores a generated key (key `SESSION_SECRET`) when `SESSION_SECRET` is empty, so restarts and replicas share it; unset = random per boot (restart = re-login) |
| `SSL_CERT_DIR` | `/etc/ssl/certs:/etc/sealane/tls-certs` (set by the chart) | Go reads every certificate in these directories as trust roots; the second is the `<release>-tls-certs` ConfigMap mount (see [Private CA](authentication.md#private-ca)) |
| `HISTORY_CONFIGMAP` | `sealane-history` (set by the chart) | ConfigMap that lists the last 200 changes made through Sealane; the objects go to `<name>-objects`. Both are created on the first change. Unset = history in memory only (dev mode) |
| `IMPERSONATION` | unset (chart: `impersonation.enabled`) | `true` = resource writes run as `sealane:admin` / `sealane:editor` through Kubernetes impersonation (see [Impersonation mode](security.md#impersonation-mode)) |
| `REQUEST_TESTER` | (set by Helm) | `true` enables the in-pod request tester (needs pods/exec, admin only) |
| `CLUSTERS_SECRET` / `POD_NAMESPACE` | (set by Helm) | multi-cluster: Secret holding remote kubeconfigs (empty = local only) |
| `SHUTDOWN_DELAY` | `5s` | wait before closing the listener on SIGTERM so in-flight requests and endpoint propagation finish; set `0s` to disable through the `extraEnv` Helm value (`- {name: SHUTDOWN_DELAY, value: "0s"}`) or by editing install.yaml |

### Single sign-on variables

Single sign-on is configured through `OIDC_*` variables (the chart's `oidc.*` values; see [Authentication](authentication.md#single-sign-on-oidc)):

| Env | Default | Description |
|---|---|---|
| `OIDC_ISSUER_URL` | (set by Helm from `oidc.issuerURL`; empty = SSO off) | OpenID Connect issuer; setting it turns single sign-on on |
| `OIDC_CLIENT_ID` / `OIDC_CLIENT_SECRET` / `OIDC_REDIRECT_URL` | (set by Helm; the secret from `oidc.clientSecretRef` or Secret `sealane-secret`) | required when `OIDC_ISSUER_URL` is set; the server refuses to start without them |
| `OIDC_SCOPES` | `openid profile email groups` | space-separated scopes |
| `OIDC_GROUPS_CLAIM` / `OIDC_USERNAME_CLAIM` | `groups` / `email` | ID-token claims for groups and the username (falls back to `sub`) |
| `OIDC_ROLES_ADMIN` / `OIDC_ROLES_EDITOR` / `OIDC_ROLES_VIEWER` | (empty) | comma-separated IdP groups per role; highest wins |
| `OIDC_DEFAULT_ROLE` | (empty = refuse; the chart sets `viewer`) | role for users with no matching group: `admin`, `editor`, `viewer` or empty |
| `LOCAL_LOGIN` | (empty = form shown) | `hidden` hides the local username/password form on the login page; no effect without single sign-on; the API still accepts local logins and `/?local=1` shows the form |
| `OIDC_DISPLAY_NAME` | `SSO` | login button label ("Sign in with …") |

## Helm values

The same table is in the [chart README](https://github.com/wonjune95/sealane/blob/main/charts/sealane/README.md); the authoritative defaults are in [`charts/sealane/values.yaml`](https://github.com/wonjune95/sealane/blob/main/charts/sealane/values.yaml), validated by `values.schema.json`.

| Key | Default | Description |
|---|---|---|
| `image.repository` | `ghcr.io/wonjune95/sealane` | Image (multi-arch amd64/arm64, cosign-signed) |
| `image.tag` | `""` (= appVersion) | Image tag |
| `image.pullPolicy` | `IfNotPresent` | |
| `imagePullSecrets` | `[]` | For private registries |
| `replicaCount` | `1` | Stateless — raise for HA |
| `istioNamespace` | `istio-system` | Where istiod runs (home-page control-plane version); `""` skips the istiod-reader Role (no version shown) |
| `service.type` / `service.port` | `ClusterIP` / `8080` | |
| `accountsConfigMap` | `sealane-accounts` | Local accounts ConfigMap; rendered empty, kept on uninstall, filled by Sealane and the UI |
| `accounts` | `{}` | Seed local accounts from values (`user: "role:bcryptHash"`). Helm manages exactly these keys (changing or removing one here changes it in the cluster); keys added in the UI are kept. |
| `clustersSecret` | `sealane-clusters` | Remote kubeconfigs Secret (not managed by the chart) |
| `requestTester.enabled` | `false` | Grants `pods/exec` cluster-wide; admin-only UI feature |
| `sessionSecret.create` | `true` | Generate and keep a session-signing key with Helm `lookup`. With `false` and no `existingSecret` (install.yaml, GitOps, `helm template`) Sealane generates one itself into Secret `<release>-secret`, key `SESSION_SECRET`. |
| `sessionSecret.existingSecret` | `""` | Use your own Secret (key `session-secret`) |
| `ingress.enabled` / `className` / `hosts` / `tls` / `annotations` | `false` … | Standard Ingress |
| `httpRoute.enabled` / `parentRefs` / `hostnames` / `annotations` | `false` … | Gateway API HTTPRoute |
| `resources` | small requests/limits | |
| `serviceMonitor.enabled` | `false` | Prometheus Operator ServiceMonitor for `/metrics` |
| `oidc.enabled` | `false` | OpenID Connect single sign-on; local accounts stay |
| `oidc.issuerURL` / `clientID` / `redirectURL` | `""` | Required when enabled; rendered into the `<release>-cm` ConfigMap |
| `oidc.clientSecretRef` | `{name: "", key: clientSecret}` | Secret holding the client secret; empty = Secret `<release>-secret`, key `OIDC_CLIENT_SECRET` |
| `oidc.scopes` / `groupsClaim` / `usernameClaim` | `[openid, profile, email, groups]` / `groups` / `email` | Scopes requested and ID-token claims read |
| `oidc.roles.{admin,editor,viewer}` | `[]` | IdP groups per role; highest wins |
| `oidc.defaultRole` | `viewer` | Role for users with no matching group; empty string refuses them |
| `oidc.displayName` | `SSO` | Login button label |
| `oidc.hideLocalLogin` | `false` | Hide the local password form on the login page (`/?local=1` shows it; the API still accepts local logins) |
| `extraEnv` / `extraVolumes` / `extraVolumeMounts` | `[]` | Passed through to the pod as-is: `SHUTDOWN_DELAY` or a proxy |
| `tlsCerts` | `{}` | Extra CA certificates (PEM) rendered into the `<release>-tls-certs` ConfigMap, e.g. a private CA for the IdP (see [Private CA](authentication.md#private-ca)); keys added with kubectl are left alone |
| `podDisruptionBudget.enabled` | `true` | PodDisruptionBudget with `maxUnavailable: 1`: one pod stays up during node drains when `replicaCount` is 2 or more |
| `impersonation.enabled` / `impersonation.bindDefaults` | `false` / `true` | Writes run as the user's role identity; `bindDefaults` binds `sealane:admin` and `sealane:editor` to edit rights on the managed kinds cluster-wide (see [Impersonation mode](security.md#impersonation-mode)) |

## Probes and metrics

| Path | Purpose |
|---|---|
| `/healthz` | liveness — returns `ok` while the process runs |
| `/readyz` | readiness — pings the Kubernetes API server (2 s timeout) and returns 503 `not ready` on failure |
| `/metrics` | Prometheus metrics: `http_requests_total` (by `method`, `status`) and `http_request_duration_seconds` (by `method`), plus the standard Go and process collectors |

Logs are structured `slog` JSON (request log + write audit log). The running version is shown at the bottom of the Settings page. See [Operations](operations.md) for scraping and log fields.
