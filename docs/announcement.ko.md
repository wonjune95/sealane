<!-- drafts; not published on the docs site -->
# Sealane 1.0 공개 알림 초안 (한국어)

본인 계정으로 올린다. 문서 사이트와 저장소 링크, "공식 프로젝트가 아님" 문장은 빼지 않는다.

## 1. GeekNews

**제목:** Sealane 1.0 - Istio·Gateway API 라우팅 설정을 안전하게 고치는 웹 콘솔

**본문:**

- VirtualService 오타 하나로 서비스가 내려가는 일을 막으려고 만든 도구입니다. 지금까지는 YAML을 `kubectl apply`로 바로 넣거나, 설정 편집에는 약한 Kiali 같은 관측 도구를 쓰는 수밖에 없었습니다.
- 모든 변경은 서버 측 dry-run 검증, diff 미리보기, `resourceVersion` 충돌 확인을 거친 뒤에 적용됩니다. 다른 사람이 먼저 고쳤다면 덮어쓰지 않고 두 버전을 나란히 보여 줍니다.
- 설계 원칙은 "배포만 하면 동작한다"입니다. 클러스터에 CRD나 오퍼레이터를 설치하지 않고, DB도 없이 기존 리소스를 그대로 읽습니다.
- Istio 12종, Gateway API 7종 리소스를 스키마 기반 폼이나 YAML로 편집하고, 라우팅 설정만으로 트래픽 흐름도를 그리며, 요청이 어느 규칙에 걸리는지 미리 확인할 수 있습니다.
- admin / editor / viewer 역할의 로컬 계정, 선택적 OIDC SSO, 멀티 클러스터를 지원합니다. UI는 영어 기본에 한국어 전환이 됩니다.
- Go 단일 바이너리(React 내장), amd64/arm64 이미지, Sigstore 서명. 1.0부터 1.x 동안의 호환 정책을 문서로 약속합니다.
- Istio나 CNCF의 공식 프로젝트가 아닌 서드파티 도구입니다(MIT).
- 문서: https://wonjune95.github.io/sealane/ · 저장소: https://github.com/wonjune95/sealane

## 2. Kubernetes Korea (페이스북 그룹 / 슬랙)

> Istio·Gateway API 라우팅 설정을 웹에서 안전하게 고치는 콘솔 Sealane 1.0을 공개했습니다.
> 적용 전에 dry-run 검증과 diff를 꼭 거치게 해서, VirtualService 실수로 서비스가 내려가는 일을 줄이는 것이 목표입니다.
> CRD·오퍼레이터 설치 없이 Helm 한 줄이면 됩니다: `helm install sealane oci://ghcr.io/wonjune95/charts/sealane --version 1.0.0 -n sealane --create-namespace`
> 폐쇄망에서도 외부 호출 없이 동작하고, UI에서 한국어를 고를 수 있습니다.
> 문서: https://wonjune95.github.io/sealane/ · 저장소: https://github.com/wonjune95/sealane
> Istio·CNCF와 무관한 개인 오픈소스입니다. 써 보시고 이슈나 의견 남겨 주시면 반영하겠습니다.
