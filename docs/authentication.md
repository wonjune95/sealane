# Authentication

Sealane has two ways to sign in, and both can be on at the same time:

- **Local accounts** — usernames, roles and bcrypt hashes in a ConfigMap. Always available.
- **Single sign-on (OIDC)** — an OpenID Connect provider (Keycloak, Dex, Google, ...) authenticates the user and their groups decide the role. Optional, off by default.

Either way the result is the same: an HMAC-signed session cookie that carries the username and one of three roles. Cluster calls always run as Sealane's ServiceAccount; the role decides what the user may do in Sealane (see the [permission model](architecture.md#permission-model)).

| Role | Can |
|---|---|
| `viewer` | read everything, use route check |
| `editor` | everything a viewer can, plus create, update and delete resources |
| `admin` | everything an editor can, plus manage accounts and clusters and use the request tester (when enabled) |

## Local accounts

### The accounts ConfigMap

One key in the `sealane-accounts` ConfigMap is one account; the value is `role:bcryptHash`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: sealane-accounts
  namespace: sealane
data:
  admin: "admin:<bcrypt-hash>"
  # viewer1: "viewer:<bcrypt-hash>"
```

The ConfigMap lives in the cluster, not the pod, so accounts survive restarts and redeploys. Helm does not manage it (the name comes from `accountsConfigMap`), so upgrades and uninstalls keep it. The pod mounts it, and edits apply **within about a minute, without a restart**. The full example is [`examples/accounts-configmap.yaml`](https://github.com/wonjune95/sealane/blob/main/examples/accounts-configmap.yaml).

### Adding accounts

As admin, use **settings → account management** to add accounts, change roles and passwords, and delete accounts. To prevent lockout you cannot delete yourself, change your own role, or delete the `admin` account.

`kubectl` works too:

```bash
go run ./hack/bcrypt-hash.go 'password'                      # generate bcrypt hash
# (without Go) htpasswd -bnBC 10 "" 'password' | tr -d ':\n'
kubectl -n sealane edit configmap sealane-accounts
# add to data:  username: "role:hash"   (role: admin | editor | viewer)
```

Add a key to add an account, edit the role string to change the role, delete the key to delete the account.

### The initial admin

If the accounts ConfigMap is missing or has no `admin` key at startup, Sealane creates **`admin` / `admin`**. Logging in with that password locks the UI to a password-change screen until you change it.

### Changing your own password

Everyone with a local account, viewers included, changes their own password from the user chip in the header → settings page, after entering the current one. New passwords need at least 8 characters. The server patches only your own key in the ConfigMap.

## Single sign-on (OIDC)

### How it works

- **Authorization code flow with PKCE.** The **Sign in with …** button on the login page goes to `/api/auth/oidc/login`, which redirects to the provider. The provider sends the browser back to `/api/auth/oidc/callback`. State, nonce and the PKCE verifier travel in a short-lived signed cookie (10 minutes), so any replica can finish the flow.
- **ID token checks.** Sealane redeems the code, then verifies the ID token's signature, audience (your client ID), expiry and nonce.
- **Username.** Taken from the claim named by `usernameClaim` (default `email`). If that claim is missing or empty, the token's `sub` is used.
- **Groups to role.** The claim named by `groupsClaim` (default `groups`) must be a list of strings. `oidc.roles.admin`, `oidc.roles.editor` and `oidc.roles.viewer` list the IdP groups for each role; the **highest matching role wins** (admin, then editor, then viewer).
- **`defaultRole`.** Users with no matching group get `defaultRole` — `viewer` in the chart. Set it to `""` to refuse them instead; they see an "access denied" message.
- **Local login stays.** The username and password form remains on the login page; the initial `admin` and any local accounts keep working. Keep at least one local admin as a break-glass account in case the IdP is down.
- **Same session cookie.** An SSO login ends in the same HMAC-signed session cookie as a local login (12 hours). The role is decided at sign-in and carried in the cookie, so group changes at the IdP apply on the user's next sign-in. Signing out of Sealane clears the cookie; it does not sign the user out of the IdP.
- **No password in Sealane.** SSO users see "You signed in through …" on the settings page instead of the password form. They are not written to the accounts ConfigMap and do not appear in the account list.
- **Failure messages.** When a sign-in fails, the browser goes back to the login page with `?oidc_error=<code>` and a generic message; the details go to the server log only. The three codes are listed under [troubleshooting](#troubleshooting).

The login page learns what is available from `GET /api/auth/methods` (unauthenticated), which returns whether local login is on and, when SSO is configured, its display name.

### Helm values

Create the client at your provider with the redirect URI `https://<your-sealane-host>/api/auth/oidc/callback`, store the client secret in a Secret in Sealane's namespace, then enable `oidc`:

```bash
kubectl -n sealane create secret generic sealane-oidc --from-literal=clientSecret='<client-secret>'
```

```yaml
oidc:
  enabled: true
  displayName: Keycloak            # login button: "Sign in with Keycloak"
  issuerURL: https://keycloak.example.com/realms/main
  clientID: sealane
  clientSecretRef: { name: sealane-oidc, key: clientSecret }
  redirectURL: https://sealane.example.com/api/auth/oidc/callback
  scopes: [openid, profile, email, groups]   # default
  groupsClaim: groups                        # default
  usernameClaim: email                       # default
  roles:
    admin: [platform-admins]
    editor: [sre]
    viewer: []
  defaultRole: viewer                        # "" refuses users without a matching group
```

`issuerURL`, `clientID`, `clientSecretRef.name` and `redirectURL` are required; the chart refuses to render without them. The chart passes these as `OIDC_*` environment variables (see [Configuration](configuration.md#environment-variables)); an incomplete set or an invalid `defaultRole` stops the server at startup with `oidc config invalid` in the log.

When upgrading an existing release from 0.9, use `--reset-then-reuse-values` (Helm 3.14+) or pass your values explicitly, as described in [Upgrading](upgrading.md). With plain `--reuse-values` the new `oidc` defaults are not merged in, so `defaultRole` ends up empty and users without a matching group are refused.

### Things to know

- **Local and SSO names share one namespace.** Sealane identifies a user by name only, and an SSO user's name comes from `usernameClaim` (default `email`). If a local account and an SSO user end up with the same name, the app treats them as the same name: in change-history and audit entries, and in the "you cannot change your own role" and "you cannot delete yourself" checks. Local account names cannot contain `@` (letters, digits, `-`, `.` and `_` only), so with the default `email` claim the two cannot collide; they can when you switch `usernameClaim` to something like `preferred_username`, or when a token lacks the claim and Sealane falls back to `sub`. Avoid naming local accounts after SSO usernames.
- **Discovery is lazy.** Sealane fetches the provider's discovery document on the first SSO sign-in, not at boot. An unreachable or misconfigured IdP never blocks startup, health checks or local logins; it only makes SSO sign-ins fail with `oidc_error=idp`, and the next attempt retries discovery.
- **Multiple replicas need a shared session key.** Both the session cookie and the short-lived sign-in cookie are HMAC-signed. With more than one replica, every pod must use the same key, which `sessionSecret` provides (`sessionSecret.create=true` is the default). Without it each pod signs with its own random key: sessions break when a request lands on another pod, and a callback served by a different pod than the login fails with `oidc_error=state`.

### Provider notes

#### Keycloak

1. In your realm, create a client: client type **OpenID Connect**, client ID `sealane`, **Client authentication** on, **Standard flow** on, valid redirect URI `https://sealane.example.com/api/auth/oidc/callback`. Copy the client secret from the **Credentials** tab.
2. Add a groups claim. Create a client scope named `groups` with a **Group Membership** mapper (token claim name `groups`, **Add to ID token** on, **Full group path** off so groups arrive as `platform-admins`, not `/platform-admins`), and add that scope to the `sealane` client. Naming the scope `groups` matches the default `scopes` list.
3. Use the realm's issuer: `issuerURL: https://keycloak.example.com/realms/<realm>`.

#### Dex

To try SSO without a real IdP, deploy [`examples/dex-demo.yaml`](https://github.com/wonjune95/sealane/blob/main/examples/dex-demo.yaml). It runs Dex with the **mock connector**: every sign-in succeeds as `kilgore@kilgore.trout` with the group `authors`. The comments at the top of the file give the steps: replace the two `example.com` hosts (Dex must be reachable by the browser **and** by the Sealane pod at the same issuer URL), apply the file, then upgrade Sealane with:

```bash
helm upgrade sealane oci://ghcr.io/wonjune95/charts/sealane --version 0.10.0 -n sealane --reset-then-reuse-values \
  --set oidc.enabled=true --set oidc.displayName=Dex \
  --set oidc.issuerURL=http://dex.example.com/dex --set oidc.clientID=sealane \
  --set oidc.clientSecretRef.name=sealane-dex-client \
  --set oidc.redirectURL=http://sealane.example.com/api/auth/oidc/callback \
  --set 'oidc.roles.admin={authors}'
```

The demo is not for production. [`web/tests/oidc-dex.mjs`](https://github.com/wonjune95/sealane/blob/main/web/tests/oidc-dex.mjs) runs a real round trip against it (SSO login, settings page, sign-out); see [`web/tests/README.md`](https://github.com/wonjune95/sealane/blob/main/web/tests/README.md).

With a real Dex, groups come from the connector (for example LDAP group search or GitHub teams), and Dex includes them in the ID token only when the `groups` scope is requested — which the default `scopes` does.

#### Google

- Issuer `https://accounts.google.com`; create an OAuth client of type **Web application** with the redirect URI above.
- Google does not know a `groups` scope and its ID tokens carry no groups claim, so set `scopes: [openid, profile, email]` and keep `usernameClaim: email`.
- With no groups, role mapping falls through to `defaultRole`: either give every Google user `viewer` and keep a local `admin` account for changes, or set a higher `defaultRole` deliberately. Every Google account that completes the sign-in gets `defaultRole`, so restrict who can sign in on the Google side (for a Google Workspace organization, an **Internal** OAuth consent screen).

### Troubleshooting

Every failed callback logs a warning `oidc callback failed` with `reason` (the code below) and `err` (the underlying error). A failure before the redirect to the IdP logs `oidc login` instead. Start with:

```bash
kubectl -n sealane logs deploy/sealane | grep oidc
```

| `oidc_error` | Login page says | What to check |
|---|---|---|
| `state` | The sign-in attempt expired or was tampered with. | The sign-in took longer than 10 minutes, or the browser dropped the sign-in cookie, or replicas sign with different keys (see `sessionSecret` above). Try again. |
| `access_denied` | Your account has no role in Sealane. | The user has no group listed in `oidc.roles.*` and `defaultRole` is empty. Check the group names in the ID token (exact strings, for Keycloak without the leading `/`) and `groupsClaim`. |
| `idp` | The identity provider returned an error. | Read the `err` in the log. Common causes: `redirectURL` does not match the redirect URI registered on the IdP client **exactly** (scheme, host, port, path); `issuerURL` differs from the `issuer` that the provider's discovery document returns (a trailing slash or `http` vs `https` is enough); the Sealane pod cannot reach the issuer; wrong client secret; an unknown scope. If the user cancelled or the IdP refused the sign-in, its own `error` (for example the IdP's `access_denied`) appears in `err`. |

To compare the issuer, fetch `<issuerURL>/.well-known/openid-configuration` from inside the cluster and check its `issuer` field against `oidc.issuerURL` character by character.
