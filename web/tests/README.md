# Browser regression checks

Start the frontend with `npm run dev`, install Chromium with `npx playwright install chromium` if needed, and run the following commands from `web`:

```bash
npm run test:cluster-selection
npm run test:flow-map
npm run test:login-sso
npm run test:list-sort
npm run test:vs-form
npm run test:httproute-form
npm run test:dr-form
npm run test:routecheck
npm run test:services
npm run test:editor-focus
npm run test:service-create
npm run test:managed-by
npm run test:cluster-menu
```

`npm run test:ui` runs every fixture-based check; CI runs it on each push (dev server on `127.0.0.1:5173`, `SEALANE_TEST_URL` set).

The checks replace API responses with fixtures. They verify that a removed cluster selection recovers before capabilities requests, login still works, valid registrations retain their selection, and connection or list failures offer recovery. No Kubernetes API or actual login credentials are used.

`npm run test:login-sso` covers the SSO button and the `oidc_error` messages on the login page, fixture-based.

`node tests/analysis.mjs` covers the Mesh analysis page (severity filter, search, skipped-pods banner, resource links), fixture-based.

`npm run test:list-sort` covers sortable columns (Name, Age by duration) and the 50-row "Show more" paging on the resource list, fixture-based.

`npm run test:vs-form` covers the curated VirtualService form: a new VS built through the form (hosts, gateways, URI match added with "+ URI", weighted subsets; no weight input for a lone route) checked against the YAML tab, the subset suggestions per destination host, the Form tab lock for unsupported fields (fault), a lossless edit of an existing VS, typing a header name through an existing one without collapsing rows, and the uri row's × dropping only the uri, fixture-based.

`npm run test:httproute-form` covers the curated HTTPRoute form's Hostnames "+ Add": the new row stays editable and its value reaches the YAML tab, fixture-based.

`npm run test:dr-form` covers the curated DestinationRule form: a new DR built through the form (host with the missing-Service badge, labelled subsets, TLS mode) checked against the YAML tab, the Form tab lock for unsupported fields (connectionPool), a lossless edit of an existing DR, typing a label key through an existing one, and unset selects dropping tls and trafficPolicy, fixture-based.

`npm run test:routecheck` covers the request console: a route-check destination whose Service does not expose the request port shows the red port pill, and an Envoy 503 with no upstream shows the no-cluster hint, fixture-based.

`npm run test:services` covers the Services list counts: the request carries `mesh=true`, the Routes, Rules, Policies and Findings columns (a dash for zero, a red pill for findings) and sorting by Findings, fixture-based (`SEALANE_SCREENSHOT=<path>` also saves a screenshot).

`npm run test:editor-focus` covers opening a finding's field: `findFocusLine` on js-yaml-shaped text, `?focus=<path>&value=<v>` switching a form-capable VirtualService to the YAML tab with the offending line highlighted and the banner dismissible, and the not-found banner for a path absent from the YAML, fixture-based (`SEALANE_SCREENSHOT=<path>` also saves a screenshot).

`npm run test:service-create` covers creating from the Service page: a viewer sees no create buttons, an editor's "New VirtualService/DestinationRule for this service" open `?ns=<ns>&host=<name>` on the Form tab with name, namespace and host(s) prefilled, and a lone `ns` leaves the template unchanged, fixture-based (`SEALANE_SCREENSHOT=<path>` also saves a screenshot).

The flow map checks cover mixed ingress/egress parents, missing gateways, search and namespace filters retaining related paths, problem paths, keyboard selection, scaled drag, layout reset, 320–1440px layouts, reduced motion, and empty/error/retry states.

Set `SEALANE_TEST_URL` to change the frontend URL. Set `SEALANE_CHROMIUM_PATH` to use an existing Chromium executable.

`node tests/screenshots.mjs` regenerates the README screenshots against a running instance (`SEALANE_URL`, `SEALANE_USER`, `SEALANE_PASS`); deploy `examples/demo-mesh.yaml` first so the request console has a target.

`node tests/oidc-dex.mjs` runs a real OIDC round trip (SSO login, settings page, sign-out) against a running instance with `oidc.enabled=true` and the Dex demo from `examples/dex-demo.yaml` (`SEALANE_URL`, `SEALANE_SSO_NAME`, default `Dex`). The cluster needs Gateway API or Istio CRDs, otherwise every page redirects to the install guide. It is not part of CI.

`npm run test:managed-by` covers the owner badge (Argo CD, Helm) on resource list rows and its tooltip, search by tool name, and the editor warning on managed resources only, fixture-based.

`npm run test:cluster-menu` covers the header cluster menu: no native select, current cluster checked and focused on open, Escape and outside click close it, arrow keys wrap, Enter switches the cluster (stored, app reloads), **Manage clusters…** opens Settings for admins and is absent for viewers, fixture-based.
