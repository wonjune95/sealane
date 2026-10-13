# Upgrading

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

3. Remove the old release: `helm uninstall istio-dashboard -n <namespace>`. The ConfigMap and Secret are not managed by Helm and stay in place.

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

0.9 adds new values keys (`sessionSecret`, `ingress`, `httpRoute`), so `helm upgrade --reuse-values` from 0.8.0 fails with a nil-pointer template error. Reset to the new chart defaults and then reapply your previous values (Helm 3.14+), or pass your values explicitly:

```bash
helm upgrade sealane oci://ghcr.io/wonjune95/charts/sealane --version 0.9.0 -n <namespace> --reset-then-reuse-values
```

0.9 also creates a `<release>-session` Secret by default so sessions survive restarts. Set `sessionSecret.create=false` to opt out (GitOps users should set `sessionSecret.existingSecret` instead).
