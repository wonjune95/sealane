# Compatibility

| Component | Tested in CI (e2e) | Verified on a live cluster | Expected to work |
|---|---|---|---|
| Kubernetes | 1.35 (`kindest/node:v1.35.0`) | 1.34 | last three minors (1.35–1.37 as of 1.0.0); the chart accepts 1.28+ |
| Istio | 1.30.5 and 1.31.1 (minimal profile, e2e matrix) | 1.31.1 | 1.30 and 1.31 (last two minors — see [Versioning & support](versioning.md)) |
| Gateway API | v1.5.1 standard channel | v1.5.x | v1.x standard channel |
| Architectures | linux/amd64 | linux/amd64 | linux/amd64, linux/arm64 (image is multi-arch) |

Every release also runs an upgrade job: a 0.8.0 install is upgraded to the release candidate with --reset-then-reuse-values and the accounts must survive.

Sealane only reads CRDs that are present; a cluster without Gateway API or without Istio simply shows fewer resource kinds.
