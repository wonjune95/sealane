# FAQ

## How is this different from Kiali?

Kiali is built for observing a mesh: topology from telemetry, metrics, traces and health. Sealane is built for **editing routing configuration safely**: schema-driven forms, dry-run validation, diff preview, conflict detection and role-based accounts, for both Istio and Gateway API resources. Its [traffic flow map](features.md#traffic-flow-map) is drawn from configuration alone, so it shows where traffic *should* go (and where a route is broken) without any metrics stack. The two complement each other.

## Does Sealane install CRDs or an operator?

No. It reads the Istio and Gateway API CRDs that are already in the cluster and shows only the kinds it finds. The chart installs a Deployment, a Service, a ServiceAccount, a ClusterRole and a ClusterRoleBinding for the managed kinds, namespaced Roles and RoleBindings, and optionally a session Secret, Ingress/HTTPRoute and ServiceMonitor.

## Why does every cluster call run as one ServiceAccount?

Passing each user's Kubernetes token through was tried first. It was accurate but issuing, delivering and renewing tokens per user was painful, and the login experience was poor. Sealane now authorizes users with its own roles (local accounts or SSO groups) and talks to the cluster as its ServiceAccount. Kubernetes audit logs therefore show the ServiceAccount; Sealane's change history and audit log record the user. If you need cluster RBAC to decide what each role may change, or the Kubernetes audit log to name the person, turn on [impersonation mode](security.md#impersonation-mode): writes then run as `sealane:admin` or `sealane:editor`. See [Architecture & security model](architecture.md#why-app-local-accounts-instead-of-token-passthrough).

## Does it work in an air-gapped cluster?

Yes. The frontend is embedded in the binary and nothing is fetched from the internet at runtime; with SSO the IdP only has to be reachable from the browser and the Sealane pod. Mirror the image, pin it by digest in `install.yaml` (or set `image.repository` for Helm) and add the few settings `install.yaml` does not carry. Step by step: [Air-gapped install](air-gapped.md).

## My cluster has only Gateway API (or only Istio). Is that fine?

Yes. Sealane shows the kinds whose CRDs exist and hides the rest. On a cluster without Istio, see the `istioNamespace` note in [Getting started](getting-started.md); the home page then shows no control-plane version. With neither Gateway API nor Istio CRDs, every page leads to an install guide.

## I lost the admin password. How do I get back in?

If another admin can sign in, they set a new password for `admin` under **Settings → Accounts**. Otherwise edit the accounts ConfigMap directly, no restart needed:

```bash
go run ./hack/bcrypt-hash.go 'new-password'
kubectl -n sealane edit configmap sealane-accounts   # set  admin: "admin:<new-hash>"
```

The change applies within seconds and ends the sessions that used the old password. Alternatively, have Sealane generate a new one: remove the `admin` key and the old initial password, then restart. With no admin account at startup, Sealane creates `admin` again with a random password in Secret `sealane-initial-admin`:

```bash
kubectl -n sealane patch configmap sealane-accounts --type json -p '[{"op":"remove","path":"/data/admin"}]'
kubectl -n sealane delete secret sealane-initial-admin --ignore-not-found
kubectl -n sealane rollout restart deploy/sealane && kubectl -n sealane rollout status deploy/sealane
kubectl -n sealane get secret sealane-initial-admin -o jsonpath='{.data.password}' | base64 -d; echo
```

This works only when no other account has the admin role; otherwise one of those admins resets the password.

## Why do I have to log in again after a restart?

Sessions are signed cookies, and the signing key must outlive the pod. The Helm chart keeps one in the `<release>-session` Secret (`sessionSecret.create=true`); `install.yaml` and GitOps installs get one that Sealane generates into the `sealane-secret` Secret on first start. You are signed out after a restart only when the pod cannot read or create that Secret (the log says `session secret: using a per-boot key`) or when `SESSION_SECRET_NAME` was removed from the Deployment. See [Operations](operations.md#high-availability).

## Does it run on ARM?

Yes. The image is multi-arch (linux/amd64 and linux/arm64). CI runs the end-to-end test on amd64; see [Compatibility](compatibility.md).
