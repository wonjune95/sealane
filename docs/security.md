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
- **Initial password** — a fresh install has no default password. At first start Sealane creates the `admin` account with a random password and stores it in Secret `sealane-initial-admin`. An install upgraded from before 1.19 whose admin still has the old `admin` / `admin` password can sign in, but the server refuses every other request until the password is changed.
- **Cookies** — the session cookie is HMAC-signed, `HttpOnly` and `SameSite=Lax`, and expires after 12 hours. The role is inside the signed value, so it cannot be edited client-side. It is `Secure` when the request came over TLS or through a proxy that sends `X-Forwarded-Proto: https`, and always with `secureCookies: true`. Those responses (all of them with `secureCookies: true`) also carry `Strict-Transport-Security`.
- **Sessions end when the account changes** — a local session carries a fingerprint of the account's password hash. Sealane reads the accounts ConfigMap through the API (cached for 3 seconds), so a session ends within seconds on every replica when its account is deleted, its role changes or its password changes. If that read fails for more than 30 seconds, Sealane uses the mounted copy of the ConfigMap, which kubelet refreshes about once a minute, and counts the failures in `sealane_account_read_errors_total`. Changing your password signs out your other sessions and keeps the current one.
- **Cross-site request protection** — browsers' cross-site `POST`, `PUT` and `DELETE` requests are refused (Go's `CrossOriginProtection`, based on `Sec-Fetch-Site` or `Origin` against `Host`). Behind a proxy that rewrites the `Host` header, list the public origin in `TRUSTED_ORIGINS` (see [Configuration](configuration.md)).
- **Server-side role checks** — every mutating request is checked against the role (403); the UI only mirrors that.
- **Image and pod** — distroless, non-root (uid 65532), `readOnlyRootFilesystem`. No runtime downloads; the frontend is embedded in the binary.
- **Narrow RBAC** — the ServiceAccount can change only the managed Istio and Gateway API kinds, listed by name (none in impersonation mode) and, in its own namespace, only its accounts ConfigMap, history ConfigMap and its own Secrets (clusters, session key, initial admin password, history objects). It reads Namespaces, Services, Endpoints and Pods cluster-wide (pods only for their labels, for mesh analysis: the Analysis page and the Service view); `requestTester.enabled` adds only `pods/exec`.
- **Pasted kubeconfigs stay in their lane** — a kubeconfig registered for another cluster may not point to files (`certificate-authority`, `client-certificate`, `client-key`, `tokenFile`) or run a credential plugin, and its server must be `https://`. Otherwise it could make Sealane read its own token or other mounted files and send them to that server.
- **Change history** — the objects before and after each change are kept in a Secret, and only editors and admins can open them. Viewers see who changed what and when. Objects that 1.15 to 1.18 kept in a ConfigMap are moved into the Secret when 1.19 starts, and the ConfigMap is deleted.
- **Login throttle** — five failed local sign-ins inside ten minutes, counted per user name and per client address as reported by your proxy (the last `X-Forwarded-For` entry, else the connection address), block that key for a minute (429 with `Retry-After`). The per-user limit holds even if a client forges the header, which also means anyone can lock a named account for a minute at a time with five wrong passwords. Without a proxy header the connection address is shared by everyone behind a port-forward or a LoadBalancer without proxy protocol, so five failures by anyone block that address for a minute; and one wrong password per minute keeps a named account blocked, because the block restarts while the five failures stay inside the window. Counters are in memory, per pod.
- **SSO** — authorization code flow with PKCE; the ID token's signature, audience, expiry and nonce are verified; IdP error details stay in the server log. See [Authentication](authentication.md#single-sign-on-oidc).

What you should do:

- **Install it in its own namespace** — Sealane may create ConfigMaps and Secrets anywhere in its namespace (see [Accepted risks](#accepted-risks)), so do not share that namespace with other workloads.
- **Change the initial password** — sign in with the generated password, change it in Settings, and delete Secret `sealane-initial-admin` if you do not want it lying around. Sealane reads it again only when no account has the admin role.
- **Serve it over HTTPS** — terminate TLS at your Ingress or Gateway. Most ingress controllers send `X-Forwarded-Proto: https`, which marks the cookies `Secure`. If yours does not, set `secureCookies: true` (`SECURE_COOKIES=true` without Helm).
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
- **The ServiceAccount cannot write.** In this mode Sealane's own ClusterRole only reads the managed kinds, so a stolen ServiceAccount token cannot change routing.
- **Unchanged:** viewers never write; reads, lists, the change history, account and cluster management and the request tester still use the ServiceAccount.
- **Without Helm:** each release attaches `impersonation.yaml`, the same RBAC rendered for namespace `sealane`, including Sealane's read-only ClusterRole. Apply it after `install.yaml` and switch the mode on. Applying `install.yaml` again (an upgrade) restores the ClusterRole that can write, so apply `impersonation.yaml` after it every time:

```bash
kubectl apply -f https://github.com/wonjune95/sealane/releases/download/v1.19.0/impersonation.yaml
kubectl -n sealane set env deploy/sealane IMPERSONATION=true
```

- **Remote clusters:** the same applies there. The stored credentials need `impersonate` on those two users and groups and the two extras, and `sealane:admin` / `sealane:editor` need bindings on that cluster; otherwise writes there fail with 403. Registering a cluster with the dedicated account option while the mode is on creates both (see [Multi-cluster](multi-cluster.md#registering-a-cluster)).

## Accepted risks

These are known and left as they are, with the reason and what limits them:

- **Namespace-wide create.** Kubernetes RBAC cannot limit `create` to object names, so Sealane's Role allows creating any ConfigMap or Secret in its own namespace (it creates its accounts, history and Secrets on first use). A compromised Sealane could create a token Secret for another ServiceAccount in that namespace. Install Sealane in a namespace of its own.
- **Password hashes in a ConfigMap.** Local accounts are bcrypt hashes in the accounts ConfigMap, readable by anyone who can read ConfigMaps in Sealane's namespace. Limit that access, or use single sign-on and keep only a break-glass local admin.
- **Account lockout.** The sign-in throttle lets anyone lock a named local account for a minute at a time (see above).
- **Credentials of other clusters.** The clusters Secret holds a token or certificate for every registered cluster. Prefer the dedicated account option, which stores a token limited to the managed kinds and can be removed from the cluster when you delete the registration.
- **SSO sessions keep their role.** Sealane cannot ask the IdP whether a user still belongs to a group, so a single sign-on session keeps the role it got at sign-in for its 12-hour lifetime. To end every session at once, rotate the session key: remove `SESSION_SECRET` from Secret `sealane-secret` (or delete `<release>-session` with Helm's `sessionSecret.create`) and restart the pods; Sealane makes a new key.
- **Stored versions are complete.** The history keeps whole objects, annotations included (`kubectl.kubernetes.io/last-applied-configuration` too, so a restore applies cleanly). Only editors and admins can open them, but put no secrets in routing resources.
- **Two writes per change.** A change's history entry and its objects are two writes. If one fails, the entry can lack its objects ("no longer kept"); `sealane_history_write_failures_total` counts it.
- **Vulnerabilities without a fix.** A release is blocked by HIGH and CRITICAL findings that have a fix. Findings without one are listed in the release log and fixed when their upstream fix ships.

## The request tester

The real request sender (**Send** in the request console) runs `curl` inside a pod you pick, through `pods/exec`. Enabling it with `requestTester.enabled=true` gives Sealane's ServiceAccount `pods/exec` create cluster-wide — effectively arbitrary command execution in any pod. That is why it ships off. When enabled it is admin-only, builds argv without a shell, validates input (scheme, method, header newlines), caps timeout and response size, and audit-logs every request with user, source pod and target URL. Route check (the simulator) needs none of this and is safe for viewers.

## Verifying release signatures

Images and charts are signed keyless with cosign (GitHub OIDC identity via Sigstore), so there is no public key to fetch:

```bash
cosign verify --certificate-identity-regexp 'https://github.com/wonjune95/sealane/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com ghcr.io/wonjune95/sealane:1.19.0
cosign verify --certificate-identity-regexp 'https://github.com/wonjune95/sealane/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com ghcr.io/wonjune95/charts/sealane:1.19.0
```

Use cosign 3.x. From 1.0.0 on, signatures are stored as OCI referrers (Sigstore bundles) next to the image and chart rather than as `.sig` tags; cosign 3.x verifies both forms, older cosign only the tags.

Each release also attaches an SBOM. See [Releasing](releasing.md) for the full verification steps.
