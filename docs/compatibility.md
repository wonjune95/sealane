# Compatibility

| Component | Tested in CI (e2e) | Verified on a live cluster | Expected to work |
|---|---|---|---|
| Kubernetes | 1.35 (`kindest/node:v1.35.0`) | 1.34 | 1.28+ (accepted by the chart) |
| Istio | 1.31.1 (minimal profile) | 1.31.1 | 1.30 and 1.31 |
| Gateway API | v1.5.1 standard channel | v1.5.x | v1.x standard channel |
| Architectures | linux/amd64 | linux/amd64 | linux/amd64, linux/arm64 (image is multi-arch) |

Sealane only reads CRDs that are present; a cluster without Gateway API or without Istio simply shows fewer resource kinds.
Kubernetes: accepted by the chart (`kubeVersion`): 1.28+; actively tested: 1.34 (live cluster) and 1.35 (kind in CI).
