# Multi-cluster

One Sealane install can manage several clusters. The cluster Sealane runs in is always available as `local`; remote clusters are added by pasting a kubeconfig. Nothing is installed on the target clusters.

## Registering a cluster

As admin, go to **settings → cluster management** and paste a kubeconfig or load it from a file. Your own `~/.kube/config` is fine: pick the context of the cluster to add, and the name is filled in from it (change it if you like). Only that context is used; the other contexts and credentials in the file are dropped before anything is stored.

By default **Create a dedicated Sealane account on the cluster** is on, which works like `argocd cluster add`. Sealane uses the pasted credentials once to create, on the target cluster:

| Object | Name |
|---|---|
| Namespace | `sealane-remote` |
| ServiceAccount | `sealane` in `sealane-remote` |
| ClusterRole and ClusterRoleBinding | `sealane-remote`: the rights the chart gives the local ServiceAccount (request tester excluded) |
| Token Secret | `sealane-remote-token` (a long-lived ServiceAccount token) |
| With impersonation mode on | `sealane-remote-impersonator` and `sealane-remote-writer`, as the chart renders them |

It then stores a kubeconfig that logs in with that token, never the credentials you pasted. Those need the rights to create namespaces, cluster roles and bindings, typically a cluster admin. Registering again refreshes the objects. Clear the checkbox to store the chosen context as it is instead; Sealane can then do exactly what its credentials allow.

A connectivity test runs on registration and the result is shown next to the cluster. The cluster is saved even if the test fails (it may just be briefly unreachable), so check the status after registering. Deleting a cluster in Sealane removes only the stored kubeconfig; to remove the account from the target cluster as well:

```bash
kubectl delete clusterrolebinding,clusterrole,namespace -l app.kubernetes.io/managed-by=sealane
```

The settings page lists every cluster with its **connection status and Kubernetes version**. Deleting a cluster removes its kubeconfig.

The API behind it is admin-only: `GET /api/clusters`, `PUT /api/clusters/{name}` (register or replace, with the connectivity test) and `DELETE /api/clusters/{name}`.

![Settings — accounts & clusters](img/screenshot-settings.png)

## Where credentials live

Kubeconfigs are stored in the `sealane-clusters` Secret in Sealane's namespace (the name comes from the `clustersSecret` value), one key per cluster name. The Secret is not managed by Helm, so upgrades and `helm uninstall` keep it; delete it yourself for a full cleanup. Sealane's ServiceAccount can read and patch only that Secret (it may also create Secrets in its namespace, because Kubernetes RBAC cannot limit `create` by name).

You can edit it with `kubectl` as well; a key is a cluster name and its value is the kubeconfig.

## Switching clusters

The header cluster picker switches the current cluster and shows whether its API answers (on phones, the picker sits at the top of the navigation drawer). Every API call takes a `?cluster=` parameter (default `local`). The CRD catalog and schema caches are kept per cluster, so clusters with different CRDs installed (Istio only, Gateway API only, different versions) are handled safely; each cluster shows only the kinds it has.

## Limits

- `local` is reserved and cannot be modified or deleted. Names start with a letter or digit and contain only letters, digits, `-`, `.` and `_` (at most 64 characters).
- On a remote cluster Sealane can do exactly what the stored credentials allow; the chart's RBAC applies only to the local cluster. The dedicated account above gets the same rights as the local ServiceAccount. In [impersonation mode](security.md#impersonation-mode) it also needs to impersonate `sealane:admin` and `sealane:editor`, and those identities need bindings on the target: the dedicated account setup creates both when the mode is on, so register the cluster again after turning the mode on.
- The image is distroless and contains no cloud CLIs, so kubeconfigs that rely on an `exec` credential plugin (for example `aws eks get-token` or `gke-gcloud-auth-plugin`) are refused with a message saying so; certificate or key files referenced by path cannot be read either. Use a kubeconfig with a token or client certificate inline. For such clusters, create the account yourself with the objects in the table above and register a kubeconfig with its token.
- App roles are global: an `editor` can write on every registered cluster.
