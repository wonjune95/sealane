# Contributing

Sealane is a small project with a single maintainer; issues and pull requests are welcome and answered on a best-effort basis. The full guide is [`CONTRIBUTING.md`](https://github.com/wonjune95/sealane/blob/main/CONTRIBUTING.md); this page summarizes it.

## Development setup

Requirements: Go 1.26+, Node.js 20+, a kubectl context (kind or Docker Desktop is enough).

```bash
go run ./cmd/server --dev          # backend on :8080, uses your local kubeconfig (refuses to start in-cluster)
cd web && npm ci && npm run dev    # frontend with HMR on :5173, proxies /api to :8080
```

## Checks before you open a PR

```bash
go vet ./cmd/... ./internal/... && go test ./internal/...
cd web && npm run build && npm run lint
cd web && npm run test:cluster-selection && npm run test:flow-map && npm run test:login-sso   # browser checks, need `npm run dev` running and Playwright Chromium
./hack/check-i18n.sh               # no Korean outside locale files, en/ko keys match
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run ./cmd/... ./internal/...
helm lint charts/sealane

./hack/e2e.sh http://localhost:8080   # API smoke test against a fresh install (admin/admin)

# single binary without a container
cd web && npm ci && npm run build  # frontend build → internal/assets/dist
CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/server ./cmd/server
```

Browser tests replace API responses with fixtures; `SEALANE_TEST_URL` points them at the frontend (e.g. `http://localhost:5173`). `web/tests/screenshots.mjs` regenerates the screenshots and `web/tests/oidc-dex.mjs` runs a real SSO round trip against the Dex demo; neither is part of CI. The `ui-tests` CI job runs the fixture-based checks (`npm run test:ui`) on every push to `main` and on pull requests. See [`web/tests/README.md`](https://github.com/wonjune95/sealane/blob/main/web/tests/README.md).

## Documentation

This site is built with MkDocs Material from `docs/` and `mkdocs.yml`, and published to GitHub Pages by the `docs` workflow on every push to `main` that touches them. To preview locally:

```bash
pip install -r docs/requirements.txt && mkdocs serve   # http://127.0.0.1:8000
mkdocs build --strict                                  # what CI runs: broken links fail the build
```

The site is English only. Link to other pages with relative `.md` paths and to repository files with full GitHub URLs.

## UI strings and translations

Every user-visible string goes through `t('key')` from `web/src/i18n`. Add the English text to `web/src/i18n/en.json` and the Korean text to `web/src/i18n/ko.json` with the same key (`area.item`, `{name}` for placeholders). CI fails if the key sets differ or if Korean text appears anywhere else. Server-side messages (Go) are English only.

## Commits and pull requests

- Commit messages in English, imperative mood ("Add OIDC login", not "Added").
- Keep PRs focused. Describe what changed and how you tested it; the PR template asks for both.
- New behavior needs a test where one is practical (Go unit test with the fake client, or a Playwright fixture check).

For security problems, follow the [security policy](security.md) instead of opening a public issue. Releases follow [Releasing](releasing.md). Version numbers and what stays stable in 1.x are in [Versioning & support](versioning.md).

## Project layout

```
sealane/
├─ cmd/server/main.go          # entrypoint: ServeMux, probes/metrics, graceful shutdown
├─ internal/
│  ├─ api/                     # JSON handlers (resources CRUD, login/OIDC/accounts/clusters, capabilities,
│  │                           #  flow map, route simulator, request tester, change history)
│  ├─ auth/                    # local accounts (bcrypt), HMAC sessions, OIDC client and role mapping
│  ├─ k8s/                     # per-cluster client factory, kind registry, discovery, lookups, pod exec
│  ├─ assets/                  # built React (dist) embed + SPA fallback
│  └─ observability/           # Prometheus metrics
├─ web/                        # React 18 + TS + Vite + Tailwind + rjsf + CodeMirror
│  ├─ src/i18n/                # en.json · ko.json (UI strings)
│  └─ tests/                   # browser checks (cluster selection, flow map, SSO login, Dex round trip)
├─ hack/
│  ├─ bcrypt-hash.go           # password hash helper
│  ├─ check-i18n.sh            # CI guard: no Korean outside locale files, en/ko keys match
│  └─ e2e.sh                   # API smoke test against a fresh install
├─ charts/sealane/             # Helm chart: deployment / service / rbac / values
├─ examples/                   # accounts-configmap.yaml · httproute.yaml · demo-mesh.yaml · dex-demo.yaml
├─ docs/                       # this site (mkdocs.yml at the root), img/ (screenshots)
├─ .github/workflows/          # ci · e2e · codeql · scorecard · release (tag → image + chart) · docs (site)
├─ Dockerfile  go.mod  mkdocs.yml
```

## Tech stack

- **Backend** — Go 1.26, `net/http` (1.22 ServeMux), `client-go` dynamic client, `istio.io/client-go`, `sigs.k8s.io/gateway-api`, `coreos/go-oidc`
- **Frontend** — React 18 · TypeScript · Vite · Tailwind · TanStack Query · React Router · react-jsonschema-form · CodeMirror
- **Deploy** — multi-stage Docker (distroless, non-root) · Helm
- **Docs** — MkDocs Material, GitHub Pages
