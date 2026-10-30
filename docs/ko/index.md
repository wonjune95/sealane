# Sealane

> 영문 원문: [Sealane](../index.md). 내용이 다르면 영문이 기준입니다.

**Sealane은 Istio와 Gateway API를 위한 라우팅 콘솔입니다.** 단일 바이너리로 동작하는 웹 UI입니다. `VirtualService`, `HTTPRoute`, `DestinationRule`, `Gateway`, `AuthorizationPolicy` 등 Istio와 Gateway API 리소스를 스키마 기반 폼이나 YAML로 조회하고 만들고 고칠 수 있습니다. dry-run 검증, diff 미리보기, 역할 기반 계정을 제공하며, 클러스터에 CRD나 오퍼레이터를 설치하지 않습니다.

!!! note "참고"
    Sealane은 Istio용 서드파티 도구입니다. Istio나 CNCF의 공식 프로젝트가 아니며, 이들과 제휴하거나 보증을 받은 관계도 아닙니다.

```bash
# Helm (OCI)
helm install sealane oci://ghcr.io/wonjune95/charts/sealane --version 1.18.3 -n sealane --create-namespace
# or plain manifests
kubectl apply -f https://github.com/wonjune95/sealane/releases/latest/download/install.yaml
kubectl -n sealane port-forward svc/sealane 8080:8080   # → http://localhost:8080, log in as admin / admin
```

다음 단계: [시작하기](getting-started.md)에서 설치, UI 노출, 첫 로그인, 계정 추가를 차례로 안내합니다.

![Sealane 홈](../img/screenshot.png)

## 요약

- **로컬 계정과 역할** — 사용자는 ConfigMap 하나에 저장합니다(`user: "role:bcryptHash"`). 로그인하면 HMAC으로 서명한 세션 쿠키를 발급합니다. 클러스터 호출은 파드의 ServiceAccount로 실행하고, 누가 무엇을 바꿀 수 있는지는 앱 역할(`admin` / `editor` / `viewer`)이 정합니다. 처음 기동하면 `admin`/`admin` 계정을 만들고 비밀번호 변경을 강제합니다.
- **SSO(선택 사항)** — Keycloak, Dex, Google 등 OpenID Connect를 지원하는 제공자라면 어디로든 로그인할 수 있습니다. IdP 그룹은 같은 세 역할에 매핑합니다. 로컬 계정도 함께 쓸 수 있습니다. [인증](authentication.md)을 참고하세요.
- **안전장치를 거치는 쓰기** — 모든 쓰기는 dry-run으로 검증하고, `kubectl diff` 형식으로 변경 내용을 미리 보여 주며, `resourceVersion`으로 충돌을 확인합니다. 그래서 409가 나면 *내 사본과 서버 버전*을 나란히 보여 줍니다. 고위험 리소스 종류는 리소스 이름을 직접 입력해야 적용됩니다. UI로 한 모든 변경은 계정 이름, 이전 버전과 함께 변경 기록에 남아서 비교하고 복원할 수 있습니다.
- **무상태, 폐쇄망 지원** — React를 Go 바이너리에 내장했습니다. 세션은 서명된 쿠키라서 복제본을 원하는 만큼 수평 확장할 수 있습니다. 실행 중에 인터넷에서 가져오는 것은 없습니다.
- **범용 리소스 엔진** — dynamic client 기반입니다. Istio 12종과 Gateway API 7종이 하나의 CRUD 파이프라인을 공유하며, 클러스터에 있는 CRD만 화면에 나옵니다.
- **트래픽 흐름** — 라우팅 설정만으로 인그레스/이그레스 경로를 그립니다. 끊긴 경로(서비스 없음, 엔드포인트 0개)는 빨간 선으로 표시합니다.
- **요청 콘솔** — *경로 확인*은 트래픽을 보내지 않고 요청이 어느 규칙에 걸릴지 계산합니다. *보내기*(별도로 켜야 하며 admin 전용)는 직접 고른 파드 안에서 요청을 실행하므로 실제 사이드카 동작을 볼 수 있습니다.
- **멀티 클러스터** — kubeconfig를 붙여 넣으면 원격 클러스터가 등록됩니다. 자격 증명은 호스트 클러스터의 Secret에만 저장하고, 대상 클러스터에는 아무것도 설치하지 않습니다.
- **영어 UI와 한국어 로캘** — 헤더에서 전환합니다.

## 만든 이유

`VirtualService` 오타 하나로 서비스 전체가 내려갈 수 있습니다. 그런데 흔히 쓰는 방법은 두 가지뿐입니다. YAML을 `kubectl apply`로 바로 넣는 방법은 사람의 실수를 막아 주지 못합니다. Kiali 같은 관측 중심 도구는 설정 편집에 약합니다. 이 프로젝트는 그 사이의 빈자리, 즉 **설정을 안전하게 고치는 콘솔**을 채웁니다.

설계를 이끈 제약은 하나입니다. **"배포만 하면 동작해야 한다."** 클러스터에 CRD나 오퍼레이터를 추가하지 않고, 연결하는 즉시 기존 리소스를 읽어 화면에 보여 줍니다. 기준 데이터는 오직 Kubernetes에 있고, Sealane 자체는 아무 상태도 갖지 않습니다.

## 다음으로 읽을 문서

- [시작하기](getting-started.md) — 설치, 노출, 로그인, 계정 추가.
- [아키텍처와 보안 모델](../architecture.md) — 요청 흐름, 설계 결정, 권한 모델.
- [인증](authentication.md) — 로컬 계정과 OIDC SSO.
- [기능](../features.md) — 리소스 엔진, 쓰기 안전장치, 트래픽 흐름, 요청 콘솔.
- [설정](../configuration.md) — 플래그, 환경 변수, Helm 값.
- [버전 관리와 지원](../versioning.md) — 1.x 동안 유지되는 것, 수정을 받는 버전, 호환성 정책.
- [자주 묻는 질문](faq.md)
