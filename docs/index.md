# Sealane

**Sealane is a routing console for Istio and Gateway API.** It is a single-binary web UI that lets you browse, create and edit `VirtualService`, `HTTPRoute`, `DestinationRule`, `Gateway`, `AuthorizationPolicy` and the rest of the Istio and Gateway API resources through schema-driven forms or YAML, with dry-run validation, diff preview and role-based accounts, and no CRDs or operators installed in your cluster.

!!! note
    Sealane is a third-party tool for Istio. It is not an official Istio or CNCF project and is not affiliated with or endorsed by them.

```bash
# Helm (OCI)
helm install sealane oci://ghcr.io/wonjune95/charts/sealane --version 1.0.0 -n sealane --create-namespace
# or plain manifests
kubectl apply -f https://github.com/wonjune95/sealane/releases/latest/download/install.yaml
kubectl -n sealane port-forward svc/sealane 8080:8080   # → http://localhost:8080, log in as admin / admin
```

Next: [Getting started](getting-started.md) walks through the install, exposing the UI, the first login and adding accounts.

![Sealane home](img/screenshot.png)

## TL;DR

- **Local accounts and roles** — users live in one ConfigMap (`user: "role:bcryptHash"`); login issues an HMAC-signed session cookie. Cluster calls run as the pod's ServiceAccount; the app role (`admin` / `editor` / `viewer`) decides who may change what. A first boot creates `admin`/`admin` and forces a password change.
- **Single sign-on (optional)** — OpenID Connect login against any provider (Keycloak, Dex, Google, ...) with IdP groups mapped to the same three roles. Local accounts keep working next to it. See [Authentication](authentication.md).
- **Writes are guarded** — every write is dry-run validated, previewed as a `kubectl diff`-style change, and checked against `resourceVersion` so a 409 shows *your copy vs the server's*. High-risk kinds require typing the resource name. Every change made through the UI lands in the change history with the account name.
- **Stateless, air-gapped** — React is embedded in the Go binary; sessions are signed cookies, so N replicas scale horizontally and nothing is fetched from the internet at runtime.
- **Generic resource engine** — dynamic-client based; 12 Istio kinds and 7 Gateway API kinds share one CRUD pipeline, and only CRDs present in the cluster are shown.
- **Traffic flow map** — draws ingress/egress paths from routing config alone; broken paths (missing service, zero endpoints) show as red edges.
- **Request console** — *Check route* computes which rule a request would hit without sending traffic; *Send* (opt-in, admin only) runs the request from inside a pod you pick so you see the real sidecar behavior.
- **Multi-cluster** — paste a kubeconfig to register a remote cluster; credentials stay in the host cluster's Secret and nothing is installed on targets.
- **English UI with a Korean locale** — switch from the header.

## Why this exists

A single typo in a `VirtualService` can take down an entire service. Yet the usual options are raw YAML with `kubectl apply` (no protection against human error) or observability-focused tools like Kiali (weak at editing configuration). This project fills the gap in between: **a console for safely editing configuration**.

One constraint drove the design — **"it must work just by deploying it."** No extra CRDs or operators on the cluster; it reads and renders existing resources the moment it connects. Kubernetes is the single source of truth and Sealane itself holds no state.

## Where to go next

- [Getting started](getting-started.md) — install, expose, log in, add accounts.
- [Architecture & security model](architecture.md) — how requests flow, design decisions, the permission model.
- [Authentication](authentication.md) — local accounts and OIDC single sign-on.
- [Features](features.md) — the resource engine, write safeguards, flow map and request console.
- [Configuration](configuration.md) — flags, environment variables and Helm values.
- [Versioning & support](versioning.md) — what stays stable in 1.x, which versions get fixes, compatibility policy.
- [FAQ](faq.md)
