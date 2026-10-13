# Sealane

[English](README.md) | **한국어**

![Kubernetes](https://img.shields.io/badge/Kubernetes-native-326CE5?logo=kubernetes&logoColor=white)
![Istio](https://img.shields.io/badge/Istio-%2FGateway%20API-466BB0?logo=istio&logoColor=white)
![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![React](https://img.shields.io/badge/React-18%20%2B%20TS-61DAFB?logo=react&logoColor=black)
![License](https://img.shields.io/badge/License-MIT-yellow)
[![ci](https://github.com/wonjune95/sealane/actions/workflows/ci.yml/badge.svg)](https://github.com/wonjune95/sealane/actions/workflows/ci.yml)
[![e2e](https://github.com/wonjune95/sealane/actions/workflows/e2e.yml/badge.svg)](https://github.com/wonjune95/sealane/actions/workflows/e2e.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/wonjune95/sealane/badge)](https://scorecard.dev/viewer/?uri=github.com/wonjune95/sealane)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/sealane)](https://artifacthub.io/packages/search?repo=sealane)

> [!NOTE]
> Istio용 서드파티 도구입니다. Istio 및 CNCF의 공식 프로젝트가 아니며 제휴·보증 관계가 없습니다.

> Istio · Gateway API 라우팅/보안/텔레메트리 설정을 **계정·역할 기반으로 안전하게** 보고·만들고·고치는 단일 바이너리 웹 콘솔.

```bash
# Helm (OCI)
helm install sealane oci://ghcr.io/wonjune95/charts/sealane --version 0.10.0 -n sealane --create-namespace
# or plain manifests
kubectl apply -f https://github.com/wonjune95/sealane/releases/latest/download/install.yaml
kubectl -n sealane port-forward svc/sealane 8080:8080   # → http://localhost:8080, 초기 계정 admin / admin
```

`install.yaml`은 istiod 버전 조회용으로 `istio-system` 네임스페이스를 전제한다. Istio가 없는 클러스터에서 `kubectl apply`가 내는 `namespaces "istio-system" not found` 오류 두 건은 무시해도 된다. Helm 설치는 Istio 없이도 된다. `istioNamespace` 네임스페이스가 없으면 차트가 istiod-reader Role을 건너뛴다(Argo CD·Flux 같은 GitOps 도구는 `helm template`으로 렌더링해 이를 감지하지 못하므로 `istioNamespace: ""`로 설정) — [Getting started](https://wonjune95.github.io/sealane/getting-started/) 참고.

![Sealane 홈 화면](docs/img/screenshot.png)

> 이전 이름 **Istio Dashboard**에서 올리는 경우 [docs/upgrading.md](docs/upgrading.md)를 참고한다 — 계정과 클러스터 등록은 그대로 이어진다.

---

## 요약 (TL;DR)

- **자체 계정 · 역할 인증** — 계정은 ConfigMap 하나로 관리(사용자 = `역할:bcrypt해시`), 로그인하면 HMAC 서명 세션 쿠키. 클러스터 작업은 파드 ServiceAccount **단일 신원**으로 수행하고, 누가 무엇을 할 수 있는지는 앱 역할(`admin`/`editor`/`viewer`)이 결정한다. 설치 시 admin 계정이 없으면 **초기 계정 `admin`/`admin`을 자동 생성**하고, 첫 로그인 시 **비밀번호 변경을 강제**한다.
- **SSO (선택)** — OpenID Connect 로그인(Keycloak, Dex, Google 등). IdP 그룹을 같은 세 역할에 매핑하고, 로컬 계정도 그대로 쓸 수 있다.
- **쓰기 안전 우선** — 모든 쓰기는 `dry-run` 선검증 → `kubectl diff` 스타일 미리보기 → 적용. `resourceVersion` 낙관적 락으로 409 충돌을 감지해 *내 수정본 vs 서버 최신본*을 대조하고, 고위험 kind는 이름 타이핑 확인을 요구한다. 대시보드를 거친 모든 변경은 홈의 **변경 히스토리**에 계정명과 함께 기록된다.
- **무상태 HA · 에어갭** — Go `embed.FS`에 React SPA를 내장한 단일 바이너리. 서버가 세션 저장소를 갖지 않아(서명 쿠키) N개 복제본으로 수평 확장되고, 외부 CDN 의존이 0이라 폐쇄망에서 즉시 구동된다.
- **제네릭 리소스 엔진** — 특정 CRD에 종속되지 않는 dynamic client 기반. Istio 12종 + Gateway API 7종을 하나의 CRUD 파이프라인으로 다룬다.
- **트래픽 흐름도** — 라우팅 설정만으로 인그레스/이그레스 경로를 그래프로 그린다. 노드를 클릭하면 그 경로만 부각되고, 끊긴 경로(서비스 없음·엔드포인트 0)는 빨간 엣지로 드러난다.
- **요청 콘솔** — 요청 하나를 작성해 두 가지로 검증한다: **경로 확인**은 트래픽 없이 어느 룰에 매치되는지 계산하고(불매치 사유까지), **보내기**는 고른 파드 안에서 실제 요청을 보내 사이드카를 통과한 응답을 보여준다(opt-in, admin 전용).
- **멀티클러스터** — kubeconfig를 붙여넣는 것만으로 원격 클러스터를 등록하고(설정 페이지, admin), 헤더 드롭다운으로 전환한다. 자격증명은 대시보드가 있는 클러스터의 Secret에만 저장되고, 대상 클러스터에는 아무것도 설치하지 않는다.
- **영어 UI + 한국어 로케일** — 헤더에서 언어를 전환한다.

---

## 왜 만들었나

Istio 라우팅은 `VirtualService` 하나의 오타가 서비스 전체 장애로 직결된다. 그런데 이걸 다루는 방법은 보통 두 갈래다 — 날 YAML을 `kubectl apply` 하거나(휴먼 에러에 무방비), Kiali처럼 관측에 특화된 도구를 쓰거나(설정 편집은 약함). 이 프로젝트는 그 사이의 빈칸, 즉 **"안전하게 설정을 편집하는 콘솔"** 을 채운다.

핵심 제약은 하나였다 — **"대시보드를 띄우는 것만으로 동작해야 한다."** 클러스터에 별도 CRD/Operator를 설치하지 않고, 붙는 순간 기존 리소스를 읽어 렌더링한다. 이를 위해 K8s를 유일한 진실의 원천으로 두고, 대시보드 자신은 상태를 갖지 않도록 설계했다.

---

## 스크린샷

트래픽 흐름도는 라우팅 설정만으로 인그레스·이그레스 경로를 그리고, 끊긴 경로를 빨간색으로 표시한다.

![트래픽 흐름도 — 경로 탐색과 선택한 노드 상세](docs/img/screenshot-flowmap.png)

요청 콘솔은 요청이 어느 룰에 매치되는지 계산하거나, 고른 파드 안에서 실제로 보내 본다.

![요청 콘솔 — 파드 안에서 보낸 실제 요청의 응답](docs/img/screenshot-requesttest.png)

리소스는 CRD 스키마로 만든 폼으로 편집하고, YAML 보기와 동기화된다.

![리소스 편집 폼](docs/img/screenshot-form.png)

admin은 설정 페이지에서 계정과 원격 클러스터를 관리한다.

![설정 페이지 — 계정·클러스터 관리](docs/img/screenshot-settings.png)

---

## 문서

문서는 영어로 제공됩니다: **https://wonjune95.github.io/sealane/**

- [Getting started](https://wonjune95.github.io/sealane/getting-started/) — 설치, 노출, 첫 로그인, 계정
- [Authentication](https://wonjune95.github.io/sealane/authentication/) — 로컬 계정과 OIDC SSO
- [Architecture & security model](https://wonjune95.github.io/sealane/architecture/) — 요청 흐름, 설계 결정, 권한 모델
- [Features](https://wonjune95.github.io/sealane/features/) — 리소스 엔진, 쓰기 안전장치, 흐름도, 요청 콘솔
- [Configuration](https://wonjune95.github.io/sealane/configuration/) — 플래그, 환경변수, Helm values
- [FAQ](https://wonjune95.github.io/sealane/faq/)
- [Versioning & support](https://wonjune95.github.io/sealane/versioning/) · [Governance & adopters](https://wonjune95.github.io/sealane/governance/) — 1.x 호환 약속, 지원 범위, 프로젝트 운영 방식

## 개발

```bash
go run ./cmd/server --dev          # 백엔드 :8080, 로컬 kubeconfig 사용 (인클러스터에선 부팅 거부)
cd web && npm run dev              # 프론트 HMR: Vite :5173 → /api 프록시 → :8080

go test ./internal/...             # 단위 테스트
go vet ./cmd/... ./internal/...
./hack/check-i18n.sh               # 로케일 파일 밖 한글 금지, en/ko 키 일치
helm lint charts/sealane
cd web && npm run lint && npm run test:cluster-selection && npm run test:flow-map && npm run test:login-sso
./hack/e2e.sh http://localhost:8080   # 새로 설치한 인스턴스에 대한 API 스모크 테스트 (admin/admin)
```

전체 체크리스트와 문서 빌드 방법은 [CONTRIBUTING.md](CONTRIBUTING.md)와 [contributing 페이지](https://wonjune95.github.io/sealane/contributing/)를 참고한다.

## 호환성

CI에서 kind + Istio 1.31.1 + Gateway API v1.5.1(linux/amd64)로 검증하며, 이미지는 멀티아치(amd64/arm64)다.
Kubernetes 1.28 이상이 필요하며, 전체 표는 [docs/compatibility.md](docs/compatibility.md)에 있다.

## 라이선스

[MIT](LICENSE)
