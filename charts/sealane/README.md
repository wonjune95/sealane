# Sealane Helm chart

Routing console for Istio and Gateway API. Single stateless binary, no CRDs or operators.

```bash
helm install sealane oci://ghcr.io/wonjune95/charts/sealane --version 0.9.0 -n sealane --create-namespace
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
| `istioNamespace` | `istio-system` | Where istiod runs (home-page control-plane version) |
| `service.type` / `service.port` | `ClusterIP` / `8080` | |
| `accountsConfigMap` | `sealane-accounts` | Local accounts ConfigMap (not managed by the chart) |
| `clustersSecret` | `sealane-clusters` | Remote kubeconfigs Secret (not managed by the chart) |
| `requestTester.enabled` | `false` | Grants `pods/exec` cluster-wide; admin-only UI feature |
| `sessionSecret.create` | `true` | Generate and keep a session-signing key (sessions survive restarts) |
| `sessionSecret.existingSecret` | `""` | Use your own Secret (key `session-secret`) |
| `ingress.enabled` / `className` / `hosts` / `tls` / `annotations` | `false` … | Standard Ingress |
| `httpRoute.enabled` / `parentRefs` / `hostnames` / `annotations` | `false` … | Gateway API HTTPRoute |
| `resources` | small requests/limits | |
| `serviceMonitor.enabled` | `false` | Prometheus Operator ServiceMonitor for `/metrics` |

Upgrading from the Istio Dashboard name: set `accountsConfigMap=istio-dashboard-accounts` and `clustersSecret=istio-dashboard-clusters` to keep your data — see [docs/upgrading.md](https://github.com/wonjune95/sealane/blob/main/docs/upgrading.md).
