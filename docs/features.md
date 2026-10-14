# Features

## Generic resource engine
A single CRUD path (`/api/resources/{type}`) built on the dynamic client (unstructured) covers everything below. Only CRDs actually installed on the cluster are exposed (`/api/capabilities`, `/api/resourceTypes`).

| Category | Kinds |
|---|---|
| **Traffic** (Istio) | VirtualService · DestinationRule · Gateway · ServiceEntry · Sidecar · WorkloadEntry · WorkloadGroup · EnvoyFilter |
| **Security** (Istio) | AuthorizationPolicy · PeerAuthentication · RequestAuthentication |
| **Telemetry** (Istio) | Telemetry |
| **Gateway API** | GatewayClass · Gateway · HTTPRoute · GRPCRoute · TCPRoute · TLSRoute · ReferenceGrant |

**Status in lists** — resource lists show a status pill. Gateway API kinds read their conditions (`Accepted`, `ResolvedRefs`, `Programmed`; for routes, the conditions under `status.parents`): any of them `False` shows an error with its reason. Istio kinds read `status.validationMessages` (worst level wins, with the message code); istiod writes those only when its analysis is enabled (`PILOT_ENABLE_ANALYSIS=true`), otherwise Istio kinds show no pill.

## Multiple write safeguards
1. **Dry-run first** — `DryRun=All` catches admission-webhook and schema errors before saving.
2. **Diff preview** — line-level diff between the current and proposed object, `kubectl diff`-style.
3. **409 conflict control** — on `resourceVersion` mismatch, never blindly overwrite; show *my edit vs server latest* and let the user decide.
4. **High-risk confirmation** — kinds with a large blast radius require typing the resource name to delete/update.

## Traffic flow map
Draws the traffic path purely from routing configuration — no metrics dependency, so it works on any cluster immediately.

- **Ingress** — External → Gateway → Route (hosts) → Service (endpoint counts). Backends that don't exist or have zero endpoints get red edges + warning pills, so dead-end configs are caught at a glance.
- **Egress** — Mesh → egress Gateway → external hosts; ServiceEntries not referenced by any route are shown as direct egress paths. External backends get a badge with the ServiceEntry that covers them (wildcard host matching included).
- **Navigation** — switch between ingress and egress, search by name/host/kind, filter by namespace or problem paths. Matching nodes retain their upstream and downstream connections. Missing gateway references appear as warning nodes.
- **Interaction** — select a node with a click or keyboard to highlight its paths. The details panel shows its status, hosts, port and resource link. Mouse drag, zoom, fit and layout reset are available. On mobile, horizontal scrolling stays inside the graph.
- **Refresh** — configuration refreshes every 30 seconds, with a manual refresh button. Failed queries show an error and retry action. Animated connections do not represent actual traffic volume or latency.

![Traffic flow map — path navigation and selected node details](img/screenshot-flowmap.png)

## Request console — route check (simulator)
Compose a request Postman-style (method, URL, headers, body) and verify it two ways. **Route check** computes which rule the request matches and where it goes — **without sending any request**. No traffic, no side effects, and no extra permissions (viewers can use it).

- **Shows the evaluation** — not just the winning rule, but **every rule that did not match and why** (which condition failed). That answers "why didn't it take this rule?" directly.
- **Verifies the destination** — service existence and endpoint count, plus whether the `subset` is actually defined in a DestinationRule (a typo there silently blackholes traffic).
- **Models precedence correctly** — Istio takes the first match in declaration order; Gateway API follows the spec (path type → path length → method → header count). PathPrefix segment boundaries are respected (`/api` does not match `/apifoo`).

![Route check — matched rule and destination](img/screenshot-routecheck.png)

## Send a real request (opt-in)
The same request can be **sent from inside a pod you pick under Source**. It goes through that pod's sidecar, so routing, authorization and mTLS are exercised for real — not "this is how it should route" but "this is how it routed".

- **Responses read in Istio terms** — `403` + `RBAC: access denied` → denied by an AuthorizationPolicy; `503` + `upstream connect error` → mTLS mismatch or no healthy endpoint; presence of Envoy headers → whether the sidecar was actually traversed. Upstream service time (`x-envoy-upstream-service-time`) is shown alongside.
- **Three locks** — (1) Helm opt-in (`requestTester.enabled=true`, off by default), (2) admin only, (3) argv built without a shell, plus input validation (scheme, method, header newlines) and timeout/response-size caps. Every test request is audit-logged with user, source pod and target URL.
- **The cost of enabling it** — the ServiceAccount gains `pods/exec` create (it already reads pods), which is effectively arbitrary command execution in any pod. That's why it ships off. If the target container has no `curl` (distroless), the UI says so.

To try it, apply [`examples/demo-mesh.yaml`](https://github.com/wonjune95/sealane/blob/main/examples/demo-mesh.yaml) (`kubectl apply -f examples/demo-mesh.yaml`). It brings up sidecar-injected `echo` v1/v2, a curl client and an ingress Gateway route for the flow map, so you can watch the `x-user: beta` canary split both ways (clean up with `kubectl delete namespace sealane-demo`).

![Request console — a real response sent from inside a pod](img/screenshot-requesttest.png)

## Mesh analysis
**Mesh analysis** (`/analysis`, API `/api/analyze?ns=`) runs cross-resource checks that explain why traffic would not flow. It reads configuration, Endpoints and pod labels (no metrics or traffic), refreshes every 30 seconds and filters by namespace (references are still resolved cluster-wide).

| Code | Severity | On | When |
|---|---|---|---|
| `vs-host-missing` | error | VirtualService | a route destination resolves to an in-cluster Service that does not exist |
| `vs-subset-undefined` | error | VirtualService | a destination names a subset no DestinationRule defines for that Service |
| `vs-gateway-missing` | error | VirtualService | `spec.gateways` names an Istio Gateway that does not exist (`mesh` ignored) |
| `vs-host-conflict` | warning | VirtualService | another VirtualService bound to the same gateway claims the same host |
| `backend-no-endpoints` | warning | VirtualService / HTTPRoute | the backend Service exists but has zero ready endpoints (ExternalName Services are exempt) |
| `httproute-backend-missing` | error | HTTPRoute | a `backendRefs` Service does not exist |
| `httproute-not-accepted` | error | HTTPRoute | a parent condition `Accepted` or `ResolvedRefs` is `False` |
| `gateway-not-programmed` | error | Gateway (API) | condition `Accepted` or `Programmed` is `False` |
| `dr-host-missing` | warning | DestinationRule | `spec.host` resolves in-cluster but the Service is missing and no ServiceEntry covers it |
| `dr-subset-no-pods` | warning | DestinationRule | Service selector + subset labels match no pod in the Service's namespace |
| `gateway-selector-no-pods` | warning | Gateway (Istio) | `spec.selector` matches no pod in any namespace |

The two pod checks need `pods` list (in the chart's ClusterRole from 1.1.0). Without it they are skipped and the page says so (`"skipped": ["pods"]`); only pod labels are read. If Services cannot be listed, the existence checks are skipped; a kind whose CRD is not installed is skipped silently.

## Service view
**Services** (`/services`, then one Service) shows everything that applies to a Service (API `/api/services/{ns}/{name}/mesh`): its selector, ports and ready endpoints; VirtualServices whose hosts name it or that route to it (a bare `*` host never counts); DestinationRules for its host, wildcards included; HTTPRoutes with it as a backend; the gateways those routes bind; the security policies (AuthorizationPolicy, PeerAuthentication, RequestAuthentication) that apply; and the analysis findings that concern it.

Policies are marked by scope: **workload** (the policy selector matches the Service selector, or a `targetRef` names the Service), **namespace** (no selector, same namespace) or **mesh** (no selector, in the Istio namespace: `istioNamespace`, default `istio-system`). Workload scope is a heuristic: it compares the policy selector with the Service selector, not with pod labels, so a policy that selects pods by a label the Service selector lacks is not shown.

## Role-aware UI + change history
- The UI checks the signed-in user's role up front (local account or SSO): for viewers, forms and apply/delete buttons turn **read-only**. The server enforces the same rule (403), so the UI can't be bypassed.
- Sign-in: local accounts (throttled after repeated failures) and optional SSO, whose `login` log line records the IdP groups; `oidc.hideLocalLogin` leaves only the SSO button. See [Authentication](authentication.md).
- The home page's **change history** panel records create/update/delete operations made through Sealane with account and timestamp (in-memory, last 200 — resets on restart; the structured audit log is the durable record). Home cards also show the actual istiod control-plane version (e.g. `1.30.2`).

## Schema-driven forms + YAML dual editing
- `react-jsonschema-form` reads the CRD OpenAPI schema and generates input forms — safe editing without knowing YAML.
- **Required/recommended markers** — every field gets a `*` (required) · `(recommended)` · `(optional)` badge. On top of the CRD schema's `required`, a curated list compensates for Istio schemas not declaring top-level requireds (VS `hosts`, DR `host`, Gateway `selector`/`servers`, SE `hosts`/`ports`).
- **Reference-resolving forms** — `host`/`backendRefs` suggest Services, `subset` suggests DestinationRule subsets, `gateways`/`parentRefs` suggest Gateways from the live cluster, with existence badges (✓/⚠) and jump links. Free-text is preserved so external hosts and cross-namespace references still work.
- Form ↔ YAML stay in sync. If the object uses advanced fields the form can't express, the form tab locks with a "YAML only" notice → no field loss.

## UI
**Workspace navigation** — the sidebar lists the main screens (cluster overview, all resources, traffic flow, mesh analysis, services, request console) and the installed resource kinds grouped by category. A **single search box** narrows a long list of kinds, and each one carries its count.

- **Cluster switcher** — the header picker shows the selected cluster with a connection dot, each cluster's status, and a shortcut to cluster management.
- **Home shortcuts** — cards for traffic flow, the request console, mesh analysis (with live error and warning counts) and services.
- **Light/dark** — the theme syncs across tabs and carries into the YAML editor. The accent color lives in one CSS variable (`--accent`), so a one-line change reskins the app.
- **Responsive** — no horizontal scrolling down to 320px; navigation collapses into a drawer on narrow screens.
- **Accessibility** — confirm dialogs trap keyboard focus and close on Escape. High-risk resources still require typing the name to proceed.
- The YAML editor shows indent guides so indentation mistakes are visible.

![Resource edit form](img/screenshot-form.png)
