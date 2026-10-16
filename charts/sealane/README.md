# Sealane Helm chart

Routing console for Istio and Gateway API. Single stateless binary, no CRDs or operators.

```bash
helm install sealane oci://ghcr.io/wonjune95/charts/sealane --version 1.8.0 -n sealane --create-namespace
kubectl -n sealane port-forward svc/sealane 8080:8080   # http://localhost:8080 — admin / admin, then change the password
```

## Values

| Key | Default | Description |
|---|---|---|
| `image.repository` | `ghcr.io/wonjune95/sealane` | Image (multi-arch amd64/arm64, cosign-signed) |
| `image.tag` | `""` (= appVersion) | Image tag |
| `image.pullPolicy` | `IfNotPresent` | |
| `imagePullSecrets` | `[]` | For private registries |
| `replicaCount` | `1` | Stateless — raise for HA |
| `istioNamespace` | `istio-system` | Where istiod runs (home-page control-plane version); `""` skips the istiod-reader Role (no version shown) |
| `service.type` / `service.port` | `ClusterIP` / `8080` | |
| `accountsConfigMap` | `sealane-accounts` | Local accounts ConfigMap (not managed by the chart) |
| `clustersSecret` | `sealane-clusters` | Remote kubeconfigs Secret (not managed by the chart) |
| `requestTester.enabled` | `false` | Grants `pods/exec` cluster-wide; admin-only UI feature |
| `sessionSecret.create` | `true` | Generate and keep a session-signing key (sessions survive restarts). GitOps tools (Argo CD, Flux) and plain "helm template" cannot use lookup and would render a new key on every sync — point existingSecret at a Secret you manage instead. |
| `sessionSecret.existingSecret` | `""` | Use your own Secret (key `session-secret`) |
| `ingress.enabled` / `className` / `hosts` / `tls` / `annotations` | `false` … | Standard Ingress |
| `httpRoute.enabled` / `parentRefs` / `hostnames` / `annotations` | `false` … | Gateway API HTTPRoute |
| `resources` | small requests/limits | |
| `serviceMonitor.enabled` | `false` | Prometheus Operator ServiceMonitor for `/metrics` |
| `oidc.enabled` | `false` | OpenID Connect single sign-on; local accounts stay |
| `oidc.issuerURL` / `clientID` / `clientSecretRef` / `redirectURL` | `""` | Required when enabled (`clientSecretRef.name` = Secret, `key` default `clientSecret`) |
| `oidc.scopes` / `groupsClaim` / `usernameClaim` | `[openid, profile, email, groups]` / `groups` / `email` | Scopes requested and ID-token claims read |
| `oidc.roles.{admin,editor,viewer}` | `[]` | IdP groups per role; highest wins |
| `oidc.defaultRole` | `viewer` | Role for users with no matching group; empty string refuses them |
| `oidc.displayName` | `SSO` | Login button label |
| `oidc.hideLocalLogin` | `false` | Hide the username/password form when SSO is on; the API still accepts local logins; `/?local=1` shows the form |

Upgrading from the Istio Dashboard name: set `accountsConfigMap=istio-dashboard-accounts` and `clustersSecret=istio-dashboard-clusters` to keep your data — see [docs/upgrading.md](https://github.com/wonjune95/sealane/blob/main/docs/upgrading.md).

Single sign-on example (Keycloak):

```yaml
oidc:
  enabled: true
  displayName: Keycloak
  issuerURL: https://keycloak.example.com/realms/main
  clientID: sealane
  clientSecretRef: { name: sealane-oidc, key: clientSecret }
  redirectURL: https://sealane.example.com/api/auth/oidc/callback
  roles: { admin: [platform-admins], editor: [sre] }
```
