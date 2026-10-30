# Security policy

## Supported versions

Only the latest minor release receives fixes. Please upgrade before reporting. See [Versioning & support](versioning.md) for the full policy.

## Reporting a vulnerability

Do not open a public issue. Use GitHub's private vulnerability reporting on this repository ("Security" tab → "Report a vulnerability"). You will get an acknowledgement within 7 days and a fix or mitigation plan within 30 days for confirmed issues. Credit is given in the release notes unless you prefer otherwise.

## Scope notes

- Sealane runs every cluster call as its own ServiceAccount; the app role (admin/editor/viewer) is the only per-user authorization. Treat the Sealane URL as a privileged endpoint.
- The request tester (`requestTester.enabled=true`) grants `pods/exec` cluster-wide. It is off by default and admin-only.
- `--dev` disables login and refuses to start inside a cluster; never expose a dev instance.

## Hardening

What Sealane does out of the box:

- **Security headers** — a Content Security Policy (`default-src 'self'`, `frame-ancestors 'none'`, `form-action 'self'`) and related headers on every response.
- **Cookies** — the session cookie is HMAC-signed, `HttpOnly` and `SameSite=Lax`, and expires after 12 hours. The role is inside the signed value, so it cannot be edited client-side.
- **Server-side role checks** — every mutating request is checked against the role (403); the UI only mirrors that.
- **Image and pod** — distroless, non-root (uid 65532), `readOnlyRootFilesystem`. No runtime downloads; the frontend is embedded in the binary.
- **Narrow RBAC** — the ServiceAccount can change only the managed Istio and Gateway API kinds and patch only its own accounts ConfigMap and clusters Secret. It reads Namespaces, Services, Endpoints and Pods cluster-wide (pods only for their labels, for mesh analysis: the Analysis page and the Service view); `requestTester.enabled` adds only `pods/exec`.
- **Login throttle** — five failed local sign-ins inside ten minutes, counted per user name and per client address as reported by your proxy (the last `X-Forwarded-For` entry, else the connection address), block that key for a minute (429 with `Retry-After`). The per-user limit holds even if a client forges the header, which also means anyone can lock a named account for a minute at a time with five wrong passwords. Without a proxy header the connection address is shared by everyone behind a port-forward or a LoadBalancer without proxy protocol, so five failures by anyone block that address for a minute; and one wrong password per minute keeps a named account blocked, because the block restarts while the five failures stay inside the window. Counters are in memory, per pod.
- **SSO** — authorization code flow with PKCE; the ID token's signature, audience, expiry and nonce are verified; IdP error details stay in the server log. See [Authentication](authentication.md#single-sign-on-oidc).

What you should do:

- **Change the initial password** — the UI forces it on first login with `admin` / `admin`; do it before exposing the service.
- **Serve it over HTTPS** — terminate TLS at your Ingress or Gateway. The cookies do not set `Secure` themselves, so plain HTTP sends them in the clear.
- **Keep a stable session key** — see [Operations](operations.md#high-availability).
- **Restrict who reaches it** — for SSO, map only the groups that need access and consider `defaultRole: ""`. `/metrics`, `/healthz` and `/readyz` are unauthenticated; if that matters, keep them off your public route.
- **Mind the clusters Secret** — it holds credentials for every registered cluster; on a remote cluster Sealane can do whatever that kubeconfig allows.

## Impersonation mode

By default every cluster call runs as Sealane's ServiceAccount and Sealane's roles decide who may write. With `impersonation.enabled=true` (`IMPERSONATION=true` without Helm), resource writes, their dry-runs and the editor's permission check run as the signed-in user's role instead, through Kubernetes user impersonation:

| Sealane role | Kubernetes user and group | Extra on every request |
|---|---|---|
| `admin` | `sealane:admin` | `sealane-user` = the account or SSO user name, `sealane-provider` = `local` or `oidc` |
| `editor` | `sealane:editor` | same |

- **Cluster RBAC decides.** What admins and editors may change is whatever you bind to `sealane:admin` and `sealane:editor`. The chart binds both to edit rights on the managed kinds in every namespace (`impersonation.bindDefaults`, default `true`), which matches the mode off. Set it to `false` and write your own bindings to narrow it, for example editors only in their team's namespaces:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: sealane-editors
  namespace: team-a
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: sealane-writer        # or your own role; rendered only with bindDefaults=true
subjects:
  - kind: Group
    apiGroup: rbac.authorization.k8s.io
    name: sealane:editor
```

- **The API server's audit log names the person.** Requests carry the impersonated user `sealane:<role>` and the user extras above, so Kubernetes audit events show who made a change made through Sealane, not only the ServiceAccount.
- **Bounded.** The ServiceAccount may impersonate exactly the users and groups `sealane:admin` and `sealane:editor` (plus the two extras), not arbitrary identities, so a compromised Sealane cannot become `system:masters`.
- **The editor follows RBAC.** The permission check behind read-only forms asks the API server (`SelfSubjectAccessReview`) as the impersonated identity, so an editor sees a read-only editor where RBAC would refuse the write.
- **Unchanged:** viewers never write; reads, lists, the change history, account and cluster management and the request tester still use the ServiceAccount.
- **Without Helm:** each release attaches `impersonation.yaml`, the same RBAC rendered for namespace `sealane`. Apply it next to `install.yaml` and switch the mode on; both survive the next `kubectl apply -f install.yaml`:

```bash
kubectl apply -f https://github.com/wonjune95/sealane/releases/download/v1.18.2/impersonation.yaml
kubectl -n sealane set env deploy/sealane IMPERSONATION=true
```

- **Remote clusters:** the same applies there. The stored credentials need `impersonate` on those two users and groups and the two extras, and `sealane:admin` / `sealane:editor` need bindings on that cluster; otherwise writes there fail with 403. Registering a cluster with the dedicated account option while the mode is on creates both (see [Multi-cluster](multi-cluster.md#registering-a-cluster)).

## The request tester

The real request sender (**Send** in the request console) runs `curl` inside a pod you pick, through `pods/exec`. Enabling it with `requestTester.enabled=true` gives Sealane's ServiceAccount `pods/exec` create cluster-wide — effectively arbitrary command execution in any pod. That is why it ships off. When enabled it is admin-only, builds argv without a shell, validates input (scheme, method, header newlines), caps timeout and response size, and audit-logs every request with user, source pod and target URL. Route check (the simulator) needs none of this and is safe for viewers.

## Verifying release signatures

Images and charts are signed keyless with cosign (GitHub OIDC identity via Sigstore), so there is no public key to fetch:

```bash
cosign verify --certificate-identity-regexp 'https://github.com/wonjune95/sealane/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com ghcr.io/wonjune95/sealane:1.18.2
cosign verify --certificate-identity-regexp 'https://github.com/wonjune95/sealane/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com ghcr.io/wonjune95/charts/sealane:1.18.2
```

Use cosign 3.x. From 1.0.0 on, signatures are stored as OCI referrers (Sigstore bundles) next to the image and chart rather than as `.sig` tags; cosign 3.x verifies both forms, older cosign only the tags.

Each release also attaches an SBOM. See [Releasing](releasing.md) for the full verification steps.
