# Security Policy

## Supported versions

Only the latest minor release receives fixes. Please upgrade before reporting. See [docs/versioning.md](docs/versioning.md) for the full policy.

## Reporting a vulnerability

Do not open a public issue. Use GitHub's private vulnerability reporting on this repository ("Security" tab → "Report a vulnerability"). You will get an acknowledgement within 7 days and a fix or mitigation plan within 30 days for confirmed issues. Credit is given in the release notes unless you prefer otherwise.

## Scope notes

- Sealane runs every cluster call as its own ServiceAccount; the app role (admin/editor/viewer) is the only per-user authorization. Treat the Sealane URL as a privileged endpoint.
- The request tester (`requestTester.enabled=true`) grants `pods/exec` cluster-wide. It is off by default and admin-only.
- `--dev` disables login and refuses to start inside a cluster; never expose a dev instance.
