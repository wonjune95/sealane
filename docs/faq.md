# FAQ

## How is this different from Kiali?

Kiali is built for observing a mesh: topology from telemetry, metrics, traces and health. Sealane is built for **editing routing configuration safely**: schema-driven forms, dry-run validation, diff preview, conflict detection and role-based accounts, for both Istio and Gateway API resources. Its [traffic flow map](features.md#traffic-flow-map) is drawn from configuration alone, so it shows where traffic *should* go (and where a route is broken) without any metrics stack. The two complement each other.

## Does Sealane install CRDs or an operator?

No. It reads the Istio and Gateway API CRDs that are already in the cluster and shows only the kinds it finds. The chart installs a Deployment, a Service, a ServiceAccount, a ClusterRole and a ClusterRoleBinding for the managed kinds, namespaced Roles and RoleBindings, and optionally a session Secret, Ingress/HTTPRoute and ServiceMonitor.

## Why does every cluster call run as one ServiceAccount?

Passing each user's Kubernetes token through was tried first. It was accurate but issuing, delivering and renewing tokens per user was painful, and the login experience was poor. Sealane now authorizes users with its own roles (local accounts or SSO groups) and talks to the cluster as its ServiceAccount. Kubernetes audit logs therefore show the ServiceAccount; Sealane's change history and audit log record the user. See [Architecture & security model](architecture.md#why-app-local-accounts-instead-of-token-passthrough).

## Does it work in an air-gapped cluster?

Yes. The frontend is embedded in the binary and nothing is fetched from the internet at runtime. Mirror the image (`ghcr.io/wonjune95/sealane:1.11.0`) and the chart to your registry, then set `image.repository` and, if needed, `imagePullSecrets`. Without Helm, download `install.yaml` from the release, replace the `image:` line with your mirror (and add `imagePullSecrets` under the pod spec if the registry needs them) and `kubectl apply` it; the SBOM and the cosign signature on the release page let you verify what you mirrored. For SSO the IdP only has to be reachable from the browser and the Sealane pod.

## My cluster has only Gateway API (or only Istio). Is that fine?

Yes. Sealane shows the kinds whose CRDs exist and hides the rest. On a cluster without Istio, see the `istioNamespace` note in [Getting started](getting-started.md); the home page then shows no control-plane version. With neither Gateway API nor Istio CRDs, every page leads to an install guide.

## I lost the admin password. How do I get back in?

Edit the accounts ConfigMap directly — no restart needed:

```bash
go run ./hack/bcrypt-hash.go 'new-password'
kubectl -n sealane edit configmap sealane-accounts   # set  admin: "admin:<new-hash>"
```

The change applies within about a minute. Alternatively, delete the `admin` key and restart the pod: when the ConfigMap has no `admin` key at startup, Sealane recreates `admin` / `admin` and forces a password change on first login.

## Why do I have to log in again after a restart?

Sessions are signed cookies, and the signing key must outlive the pod. The Helm chart keeps one in the `<release>-session` Secret (`sessionSecret.create=true`); `install.yaml` and GitOps installs get one that Sealane generates into the `sealane-secret` Secret on first start. You are signed out after a restart only when the pod cannot read or create that Secret (the log says `session secret: using a per-boot key`) or when `SESSION_SECRET_NAME` was removed from the Deployment. See [Operations](operations.md#high-availability).

## Does it run on ARM?

Yes. The image is multi-arch (linux/amd64 and linux/arm64). CI runs the end-to-end test on amd64; see [Compatibility](compatibility.md).
