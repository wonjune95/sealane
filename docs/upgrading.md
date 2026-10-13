# Upgrading

## From Istio Dashboard (≤ 0.7) to Sealane (0.8)

Sealane is the new name of Istio Dashboard. Images, charts and default resource names changed; your data did not. Accounts live in the accounts ConfigMap and remote clusters in the clusters Secret, and both survive the move.

1. Install the new release next to the old one, pointing it at the existing ConfigMap and Secret:

   ```bash
   helm install sealane oci://ghcr.io/wonjune95/charts/sealane --version 0.8.0 -n <namespace> \
     --set accountsConfigMap=istio-dashboard-accounts \
     --set clustersSecret=istio-dashboard-clusters \
     --reuse-values   # drop this flag; copy any custom values (requestTester.enabled, image.*, resources) explicitly
   kubectl -n <namespace> rollout status deploy/sealane
   ```

2. Point your HTTPRoute or Ingress backend at the `sealane` Service (port 8080) and open the UI. Existing accounts work unchanged; everyone signs in again once because the session cookie was renamed.

3. Remove the old release: `helm uninstall istio-dashboard -n <namespace>`. The ConfigMap and Secret are not managed by Helm and stay in place.

If you prefer the new default names, rename the resources (`kubectl get cm istio-dashboard-accounts -o yaml | sed 's/istio-dashboard-accounts/sealane-accounts/' | kubectl apply -f -`, same for the Secret) and drop the two `--set` flags.

### What else changed in 0.8

- Image: `ghcr.io/wonjune95/sealane` (the old `istio-dashboard` package stays but will not get new tags).
- Chart: `oci://ghcr.io/wonjune95/charts/sealane`, source under `charts/sealane`.
- UI language: English by default, Korean available from the header toggle (browser language is detected).
- Accounts mount path inside the pod: `/etc/sealane/accounts`.
