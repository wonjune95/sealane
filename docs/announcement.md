<!-- drafts; not published on the docs site -->
# Sealane 1.0 announcement drafts

Post from your own account. Each section links the docs site and the repository; keep the "not affiliated" line.

## 1. Istio Slack (`#general` or `#show-and-tell`)

> Sealane 1.0 is out: a routing console for Istio and the Kubernetes Gateway API. It is a single Go binary with the React UI embedded, installs no CRDs or operators, and edits VirtualService, HTTPRoute, DestinationRule and friends with dry-run validation, a diff preview and role-based accounts.
> New in 1.0: a written stability policy (Helm values, env vars and account formats stay compatible across 1.x). Since the first public release it has also gained OIDC single sign-on, multi-arch images (amd64/arm64) and signed images and charts.
> Docs: https://wonjune95.github.io/sealane/ · Repo: https://github.com/wonjune95/sealane
> `helm install sealane oci://ghcr.io/wonjune95/charts/sealane --version 1.0.0 -n sealane --create-namespace`
> `kubectl -n sealane port-forward svc/sealane 8080:8080` (log in as admin / admin, you are asked to change it)
> Third-party tool, not affiliated with or endorsed by Istio or the CNCF. Feedback and issues welcome.

## 2. r/kubernetes

**Title:** Sealane 1.0: a routing console for Istio and Gateway API (single binary, no CRDs)

**Body:**

A single typo in a VirtualService can take a service down. The usual choices are raw YAML plus `kubectl apply`, which does nothing to stop a mistake, or observability tools like Kiali, which are not built for editing configuration. Sealane sits in between: a console for editing routing config safely.

What it does:

- Browse and edit 12 Istio kinds and 7 Gateway API kinds through schema-driven forms or YAML.
- Every write is dry-run validated, shown as a diff first, and checked against `resourceVersion`, so a conflicting edit shows your copy next to the server's instead of silently overwriting it.
- A traffic flow map drawn from routing config alone, with broken paths (missing service, zero endpoints) in red.
- A request console that tells you which rule a request would hit, or (opt-in, admin only) sends it from inside a pod you pick.
- Local accounts with admin / editor / viewer roles, optional OIDC, multiple clusters from one install.

Install: `helm install sealane oci://ghcr.io/wonjune95/charts/sealane --version 1.0.0 -n sealane --create-namespace`, or `kubectl apply -f` the `install.yaml` from the latest release.

What it is not: it is not Kiali and has no metrics, tracing or topology from telemetry. It only reads and writes Kubernetes resources, as the pod's ServiceAccount (or the kubeconfigs you give it for remote clusters).

Docs: https://wonjune95.github.io/sealane/ · Repo: https://github.com/wonjune95/sealane (MIT). Third-party tool, not affiliated with Istio or the CNCF.

## 3. Show HN

**Title:** Show HN: Sealane – a safe-editing console for Istio and Gateway API routing

**Body:**

A single typo in a VirtualService applied with `kubectl apply` can take a service down. The tools I found were either good at showing the mesh (Kiali) but weak at editing it, or were a YAML editor with no guard rails, so I built Sealane to sit in between.

One constraint drove the design: it must work just by deploying it. No CRDs, no operators, no database. It reads whatever Istio and Gateway API resources already exist, sessions are signed cookies, and Kubernetes stays the only source of truth, so you can run several replicas or remove it without leaving anything behind.

Edits go through server-side dry-run, a diff preview and a `resourceVersion` check before anything is applied. There is also a flow map built from routing config and a console that answers "which rule would this request hit?" without sending traffic.

It is a single Go binary with a React UI embedded; images are multi-arch and signed with Sigstore. 1.0 adds a written compatibility policy for 1.x.

Docs: https://wonjune95.github.io/sealane/
Code: https://github.com/wonjune95/sealane

It is a third-party project, not affiliated with Istio or the CNCF. I would like to hear where it falls short.

## 4. Istio Discuss / GitHub Discussions

> Sealane 1.0.0 is released: a routing console for Istio and the Kubernetes Gateway API (single binary, no CRDs, dry-run and diff before every write). 1.0 adds a stability and support policy; OIDC sign-on and signed multi-arch images arrived in the 0.9/0.10 releases.
> Docs: https://wonjune95.github.io/sealane/ · Release notes and code: https://github.com/wonjune95/sealane
> Sealane is a third-party tool, not affiliated with Istio or the CNCF; questions and bug reports go to GitHub issues.
