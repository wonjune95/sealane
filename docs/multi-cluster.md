# Multi-cluster

One Sealane install can manage several clusters. The cluster Sealane runs in is always available as `local`; remote clusters are added by pasting a kubeconfig. Nothing is installed on the target clusters.

## Registering a cluster

As admin, go to **settings → cluster management**, enter a name and paste the target cluster's kubeconfig. A connectivity test runs on registration and the result is shown next to the cluster. The cluster is saved even if the test fails (it may just be briefly unreachable), so check the status after registering.

The settings page lists every cluster with its **connection status and Kubernetes version**. Deleting a cluster removes its kubeconfig.

The API behind it is admin-only: `GET /api/clusters`, `PUT /api/clusters/{name}` (register or replace, with the connectivity test) and `DELETE /api/clusters/{name}`.

![Settings — accounts & clusters](img/screenshot-settings.png)

## Where credentials live

Kubeconfigs are stored in the `sealane-clusters` Secret in Sealane's namespace (the name comes from the `clustersSecret` value), one key per cluster name. The Secret is not managed by Helm, so upgrades and `helm uninstall` keep it; delete it yourself for a full cleanup. Sealane's ServiceAccount can read and patch only that Secret (it may also create Secrets in its namespace, because Kubernetes RBAC cannot limit `create` by name).

You can edit it with `kubectl` as well; a key is a cluster name and its value is the kubeconfig.

## Switching clusters

The header dropdown switches the current cluster, and the sidebar always shows which one is selected. Every API call takes a `?cluster=` parameter (default `local`). The CRD catalog and schema caches are kept per cluster, so clusters with different CRDs installed (Istio only, Gateway API only, different versions) are handled safely; each cluster shows only the kinds it has.

## Limits

- `local` is reserved and cannot be modified or deleted. Names start with a letter or digit and contain only letters, digits, `-`, `.` and `_` (at most 64 characters).
- On a remote cluster Sealane can do exactly what the pasted kubeconfig's credentials allow; the chart's RBAC applies only to the local cluster. Use a dedicated ServiceAccount token on the target with the permissions you want Sealane to have.
- The image is distroless and contains no cloud CLIs, so kubeconfigs that rely on an `exec` credential plugin (for example `aws eks get-token` or `gke-gcloud-auth-plugin`) cannot authenticate. Use a token or client-certificate kubeconfig.
- App roles are global: an `editor` can write on every registered cluster.
