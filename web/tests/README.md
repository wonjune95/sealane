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
```

The checks replace API responses with fixtures. They verify that a removed cluster selection recovers before capabilities requests, login still works, valid registrations retain their selection, and connection or list failures offer recovery. No Kubernetes API or actual login credentials are used.

`npm run test:login-sso` covers the SSO button and the `oidc_error` messages on the login page, fixture-based.

`node tests/analysis.mjs` covers the Mesh analysis page (severity filter, search, skipped-pods banner, resource links), fixture-based.

`npm run test:list-sort` covers sortable columns (Name, Age by duration) and the 50-row "Show more" paging on the resource list, fixture-based.

`npm run test:vs-form` covers the curated VirtualService form: a new VS built through the form (hosts, gateways, URI match, weighted subsets) checked against the YAML tab, the subset suggestions per destination host, the Form tab lock for unsupported fields (fault), a lossless edit of an existing VS, and typing a header name through an existing one without collapsing rows, fixture-based.

`npm run test:httproute-form` covers the curated HTTPRoute form's Hostnames "+ Add": the new row stays editable and its value reaches the YAML tab, fixture-based.

`npm run test:dr-form` covers the curated DestinationRule form: a new DR built through the form (host with the missing-Service badge, labelled subsets, TLS mode) checked against the YAML tab, the Form tab lock for unsupported fields (connectionPool), a lossless edit of an existing DR, typing a label key through an existing one, and unset selects dropping tls and trafficPolicy, fixture-based.

The flow map checks cover mixed ingress/egress parents, missing gateways, search and namespace filters retaining related paths, problem paths, keyboard selection, scaled drag, layout reset, 320–1440px layouts, reduced motion, and empty/error/retry states.

Set `SEALANE_TEST_URL` to change the frontend URL. Set `SEALANE_CHROMIUM_PATH` to use an existing Chromium executable.

`node tests/screenshots.mjs` regenerates the README screenshots against a running instance (`SEALANE_URL`, `SEALANE_USER`, `SEALANE_PASS`); deploy `examples/demo-mesh.yaml` first so the request console has a target.

`node tests/oidc-dex.mjs` runs a real OIDC round trip (SSO login, settings page, sign-out) against a running instance with `oidc.enabled=true` and the Dex demo from `examples/dex-demo.yaml` (`SEALANE_URL`, `SEALANE_SSO_NAME`, default `Dex`). The cluster needs Gateway API or Istio CRDs, otherwise every page redirects to the install guide. It is not part of CI.
