# Contributing to Sealane

Thanks for your interest. Sealane is a small project with a single maintainer; issues and pull requests are welcome and answered on a best-effort basis.

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
```

## Documentation

The documentation site (https://wonjune95.github.io/sealane/) is built with MkDocs Material from `docs/` and `mkdocs.yml`, and published by the `docs` workflow on pushes to `main`. Preview and check it locally:

```bash
pip install -r docs/requirements.txt && mkdocs serve   # http://127.0.0.1:8000
mkdocs build --strict                                  # CI runs this; broken links fail it
```

The site is English only (Korean lives in `README.ko.md` and the UI locale). Link between pages with relative `.md` paths and to repository files with full GitHub URLs.

## UI strings and translations

Every user-visible string goes through `t('key')` from `web/src/i18n`. Add the English text to `web/src/i18n/en.json` and the Korean text to `web/src/i18n/ko.json` with the same key (`area.item`, `{name}` for placeholders). CI fails if the key sets differ or if Korean text appears anywhere else. Server-side messages (Go) are English only.

## Commits and pull requests

- Commit messages in English, imperative mood ("Add OIDC login", not "Added").
- Keep PRs focused. Describe what changed and how you tested it; the PR template asks for both.
- New behavior needs a test where one is practical (Go unit test with the fake client, or a Playwright fixture check).

## Releases

See [docs/releasing.md](docs/releasing.md).

## Reporting bugs and proposing features

Use the issue templates. For security problems, see [SECURITY.md](SECURITY.md) instead of opening a public issue.
