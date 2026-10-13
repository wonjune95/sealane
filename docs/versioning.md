# Versioning and support

## Version numbers

Sealane follows [Semantic Versioning](https://semver.org/): `vMAJOR.MINOR.PATCH`. The image, the Helm chart and `install.yaml` share one number (the chart's `appVersion` equals its `version`).

## What is stable in 1.x

| Surface | Promise |
|---|---|
| Helm values keys and their meaning | No breaking change |
| Environment variables and flags | No breaking change |
| Accounts ConfigMap (`user: "role:bcryptHash"`) and clusters Secret format | No breaking change; existing data keeps working after an upgrade |
| Session cookie | A format change never costs more than one re-login and is announced in the release notes |
| `install.yaml` object names | No change, so `kubectl apply` upgrades in place |

Breaking changes to any of these wait for 2.0. Deprecations are announced one minor release ahead in the release notes and in [`ROADMAP.md`](https://github.com/wonjune95/sealane/blob/main/ROADMAP.md).

## Not a public API

`/api/*` exists for the bundled UI. It may change in any minor release without notice. Automate with `kubectl`, not with Sealane's endpoints.

## Support

Sealane has a single maintainer. Fixes ship as patch releases of the latest minor only; there are no backports. To report a vulnerability, follow the [security policy](security.md).

## Compatibility policy

Sealane supports the last three Kubernetes minors, the last two Istio minors and the Gateway API v1.x standard channel: bugs reported against these versions are in scope for a fix. [Compatibility](compatibility.md) lists the exact versions that CI and a live cluster run; the e2e matrix in [`.github/workflows/e2e.yml`](https://github.com/wonjune95/sealane/blob/main/.github/workflows/e2e.yml) covers both Istio minors. Older versions that the chart still accepts (Kubernetes 1.28+) probably work but are not tested.

## Release cadence

None fixed. A release ships when `main` is green and something is worth shipping.
