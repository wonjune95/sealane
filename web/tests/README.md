# Browser regression checks

Start the frontend with `npm run dev`, install Chromium with `npx playwright install chromium` if needed, and run the following commands from `web`:

```bash
npm run test:cluster-selection
npm run test:flow-map
```

The checks replace API responses with fixtures. They verify that a removed cluster selection recovers before capabilities requests, login still works, valid registrations retain their selection, and connection or list failures offer recovery. No Kubernetes API or actual login credentials are used.

The flow map checks cover mixed ingress/egress parents, missing gateways, search and namespace filters retaining related paths, problem paths, keyboard selection, scaled drag, layout reset, 320–1440px layouts, reduced motion, and empty/error/retry states.

Set `SEALANE_TEST_URL` to change the frontend URL. Set `SEALANE_CHROMIUM_PATH` to use an existing Chromium executable.

`node tests/screenshots.mjs` regenerates the README screenshots against a running instance (`SEALANE_URL`, `SEALANE_USER`, `SEALANE_PASS`); deploy `examples/demo-mesh.yaml` first so the request console has a target.
