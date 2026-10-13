# Sealane — Istio Dashboard를 공개 오픈소스 제품으로 (설계 스펙)

작성일: 2026-10-13 · 상태: 사용자 검토 대기 · 범위: v0.8 → v1.0 (4개 릴리스)

## 1. 목적

`istio_dashboard`(Go+React 단일 바이너리, Istio/Gateway API 라우팅 콘솔)를 "GitHub에서 모르는 사람이 찾아와 믿고 설치해 쓰는" 수준의 공개 오픈소스 프로젝트로 만든다. 사용자가 고른 목적은 세 가지: 프로젝트 완성도 자체, 생태계 진입(Artifact Hub·CNCF Landscape·Istio 생태계), 외부 사용자 확보. 고객사 납품은 목적이 아니다.

### 현재 상태 (2026-10-13)

- 공개 저장소 `github.com/wonjune95/istio_dashboard`, MIT, 태그 푸시 → GHCR 이미지 + OCI Helm 차트(v0.7.0).
- Go 약 5,000줄, TS 약 6,600줄. CI는 vet/test/build/docker build.
- 이미지는 **amd64 단일**(Apple Silicon·kind on Mac에서 즉시 실패).
- **UI가 전부 한국어**: `web/src`에 한국어 포함 줄 493, Go 사용자 노출 문자열 약 110개, Go 주석 110줄, TS 주석 46줄, Playwright 픽스처 테스트도 한국어 문자열 단언.
- 인증은 ConfigMap 로컬 계정만. `/readyz`는 항상 200인 스텁. 앱 버전 노출 없음.
- CONTRIBUTING/SECURITY/CoC/이슈 템플릿/문서 사이트 없음. `DESIGN.md`는 폐기된 v4(OIDC 토큰 패스스루) 설계라 신규 기여자를 오도.

### 비목표 (YAGNI)

CLI, Operator/CRD 기반 선언적 설정, Kustomize 오버레이, 실시간 SSE, K8s impersonation, Dex 번들, IdP 로그아웃 연동, 로컬 로그인 끄기 옵션, 문서 사이트 한국어판, 별도 GitHub 조직. 필요가 실제로 생기면 그때 추가한다.

## 2. 결정 사항

| 항목 | 결정 | 근거 |
|---|---|---|
| 이름 | **Sealane** (항로 = 라우팅 경로) | Istio 상표(Open Usage Commons 보유)를 제품명에 넣지 않는 생태계 관행(Kiali). GitHub 저장소명 충돌 3개(모두 0스타), `sealane.io` 미등록, Artifact Hub·동명 SW 없음. `github.com/sealane` 조직명은 선점되어 `wonjune95/sealane`으로 유지. |
| 유지보수 | Claude가 구현, 사용자가 방향·리뷰·GitHub 설정·외부 제출 | 사용자 답변 |
| 인증 | 로컬 계정 유지 + OIDC 선택(코드 플로우+PKCE, 그룹→역할 매핑) | 외부 조직의 첫 질문이 SSO. impersonation은 멀티클러스터와 충돌해 제외. |
| UI 언어 | 영어 기본 + 한국어 로케일, 라이브러리 없는 `t(key)` | 외부 사용자 1순위 차단 요소. 기존 한국 사용자도 유지. |
| 문서 언어 | README 영어 우선(`README.ko.md` 병행), 문서 사이트 영어 단일 | 이중 유지보수 회피 |
| 진행 | 4단계 릴리스 v0.8 → v0.9 → v0.10 → v1.0 | 매 단계가 쓸 수 있는 릴리스, 리뷰 단위 축소, 공개 신호 조기 발생 |
| 로고 | 기존 해도 마크 유지 | 항로 테마와 일치 |
| 표기 | "Sealane — routing console for Istio and Gateway API", README 비제휴 고지 유지 | 상표 가이드 범위 내의 서술적 사용 |

## 3. 릴리스 계획

| 릴리스 | 주제 | 내용 |
|---|---|---|
| v0.8.0 | Sealane | 리네임, 영어 UI + ko 로케일, 프로젝트 외형 파일, 영어 README |
| v0.9.0 | 어디서나 설치 | 멀티아치, install.yaml, 서명/SBOM, 차트 메타·스키마·Ingress/HTTPRoute, Artifact Hub, 진짜 `/readyz`, 버전 노출, 린트·스캔·Dependabot, kind+Istio e2e |
| v0.10.0 | SSO | OIDC, 안정적 SESSION_SECRET, mkdocs-material 문서 사이트 |
| v1.0.0 | 정책 + 생태계 | 버전/지원/호환 정책, GOVERNANCE/ADOPTERS, e2e Istio 2버전 매트릭스, 생태계 제출 준비물, 알림 문안 |

구현 계획(writing-plans)은 릴리스별로 하나씩 쓴다. v0.8 계획부터 시작하고, 이전 릴리스가 태그된 뒤 다음 계획을 쓴다.

순서 근거: 이름이 모든 산출물(이미지·차트·문서·install.yaml)에 박히므로 리네임이 첫째. i18n은 UI 파일 대부분을 건드리므로 OIDC UI 작업보다 먼저. 릴리스 엔지니어링과 e2e를 갖춘 뒤 유일한 기능 작업(OIDC)을 한다.

---

## 4. v0.8.0 — Sealane

### 4.1 리네임

사용자가 GitHub에서 하는 것: 저장소 이름 `istio_dashboard` → `sealane`(옛 URL 자동 리다이렉트), 설명 "Routing console for Istio and Gateway API", topics(`istio`, `gateway-api`, `kubernetes`, `service-mesh`, `dashboard`, `envoy`), Discussions 켜기, 비공개 취약점 신고 켜기.

코드·배포에서 바꾸는 것(한 PR):

| 대상 | 현재 | 변경 |
|---|---|---|
| 로컬 디렉터리 | `~/istio_dashboard` | `~/sealane` |
| Go 모듈 | `istio-dashboard` | `github.com/wonjune95/sealane` (pkg.go.dev·툴링 관행) |
| 바이너리 | `/istiod` (Dockerfile) | `/sealane` |
| 이미지 | `ghcr.io/wonjune95/istio-dashboard` | `ghcr.io/wonjune95/sealane` |
| 차트 | `deploy/helm`, name `istio-dashboard` | `charts/sealane`, name `sealane` (Artifact Hub·chart 관행 경로) |
| 예시 | `deploy/examples/` | `examples/` |
| 헬퍼 템플릿 | `istio-dashboard.*` | `sealane.*` |
| 세션 쿠키 | `istio_dash_session` | `sealane_session` (한 번 재로그인) |
| 계정 경로 | `/etc/istio-dashboard/accounts` | `/etc/sealane/accounts` |
| values 기본 | `accountsConfigMap: istio-dashboard-accounts`, `clustersSecret: istio-dashboard-clusters` | `sealane-accounts`, `sealane-clusters` |
| 데모 ns | `istio-dashboard-demo` | `sealane-demo` |
| 테스트 env | `ISTIO_DASHBOARD_TEST_URL`, `ISTIO_DASHBOARD_CHROMIUM_PATH` | `SEALANE_TEST_URL`, `SEALANE_CHROMIUM_PATH` |
| 웹 패키지 | `istio-dashboard-web` | `sealane-web` |
| 릴리스 워크플로 | 이미지·차트 이름 | 새 이름 |

차트 version/appVersion은 0.8.0으로 함께 올린다. 옛 GHCR 패키지(`istio-dashboard` 이미지·차트)는 지우지 않는다.

마이그레이션(`docs/upgrading.md`): 기존 설치는 `helm install sealane oci://.../charts/sealane -n <ns> --set accountsConfigMap=istio-dashboard-accounts --set clustersSecret=istio-dashboard-clusters`로 새 릴리스를 올리고, HTTPRoute/Ingress 백엔드를 `sealane` 서비스로 바꾼 뒤 옛 릴리스를 `helm uninstall`한다. 계정·클러스터 데이터는 CM/Secret에 있으므로 유지된다. CI/CD 클러스터의 라이브 설치(ns `istio-dashboard`, release `istio-dashboard`)는 이 절차대로 Claude가 옮긴다.

### 4.2 영어 UI + 한국어 로케일

- `web/src/i18n/index.tsx`: `LocaleProvider`, `useT()` → `t(key, vars?)`; `{name}` 치환. `en.json`/`ko.json`은 평면 키(`login.tagline`, `settings.cluster.deleted`). 라이브러리 추가 없음(약 50줄).
- 로케일 결정: `localStorage["sealane.locale"]` → `navigator.language`가 `ko`로 시작하면 `ko` → 그 외 `en`. 헤더의 테마 토글 옆에 언어 토글(`Layout.tsx`).
- 폼 도움말·예시 문구는 `en.json`/`ko.json`의 `help.*`/`example.*` 키에 두고, `fieldHelp.ts`/`fieldExample.ts`는 타입 없는 `lookup()`으로 읽는다(구현 기준으로 수정 — 원안의 `{ en, ko }` 맵은 "한국어는 ko.json에만" 불변식과 충돌).
- Go 사용자 노출 문자열(에러 메시지, 로그 메시지, `hack/bcrypt-hash.go` 안내)은 영어로 통일. 서버 에러는 로케일과 무관하게 영어로 표시한다(에러 코드 매핑은 필요 시).
- 코드 주석(Go 110줄, TS 46줄)과 차트·values·examples·워크플로 주석을 영어로.
- Playwright 픽스처 테스트(`web/tests/*.mjs`)는 영어 문자열로 갱신(테스트 기본 로케일 en).
- 가드 `hack/check-i18n.sh`(CI에서 실행): ① `git grep -P '[가-힣]'`이 `web/src/i18n/ko.json`, `README.ko.md`, `docs/superpowers/` 외에서 한 줄이라도 나오면 실패 ② en/ko 키 집합이 다르면 실패.

### 4.3 프로젝트 외형

- `README.md` = 영어(현 `README.en.md`를 Sealane 기준으로 다시 씀; 맨 위에 설치 한 줄·스크린샷·핵심 기능·링크). `README.ko.md` = 한국어. `README.en.md` 삭제.
- 추가: `CONTRIBUTING.md`(개발 환경, 테스트, PR 규칙, DCO sign-off), `CODE_OF_CONDUCT.md`(Contributor Covenant 2.1), `SECURITY.md`(GitHub 비공개 취약점 신고, 지원 버전 = 최신 minor), `ROADMAP.md`(v0.9~v1.0), `.github/ISSUE_TEMPLATE/bug.yml`·`feature.yml`, `.github/PULL_REQUEST_TEMPLATE.md`.
- 삭제: `DESIGN.md`, `docs/ui-refresh/`(디자인 작업 기록 17장). git 이력에 남는다.
- README 스크린샷 7장(`docs/*.png`)은 영어 UI를 라이브에 배포한 뒤 다시 찍어 `docs/img/`로 옮긴다.

### 4.4 검증

`go vet ./... && go test ./...`, `npm run build && npm run lint`, Playwright 2종, `hack/check-i18n.sh`, `helm lint charts/sealane && helm template`, `docker build`, 라이브 클러스터에 `sealane` 릴리스로 설치해 기존 계정으로 로그인·리소스 목록 확인. 이후 `v0.8.0` 태그.

---

## 5. v0.9.0 — 어디서나 설치

### 5.1 이미지

- Dockerfile: 빌드 스테이지에 `--platform=$BUILDPLATFORM`, Go 빌드에 `GOOS=linux GOARCH=$TARGETARCH`(크로스 컴파일, QEMU 없이). `-X main.version=$VERSION` ldflag(`ARG VERSION=dev`).
- release.yml: `docker/setup-qemu-action` + `setup-buildx-action` + `build-push-action`으로 `linux/amd64,linux/arm64` 발행. `permissions: id-token: write` 추가. `sigstore/cosign-installer` → `cosign sign --yes ghcr.io/wonjune95/sealane@<digest>`. `anchore/sbom-action`으로 SPDX SBOM을 릴리스 자산에 첨부.
- 문서의 검증 명령: `cosign verify --certificate-identity-regexp 'https://github.com/wonjune95/sealane/' --certificate-oidc-issuer https://token.actions.githubusercontent.com ghcr.io/wonjune95/sealane:<tag>`.
- 앱: `var version = "dev"`, `--version` 플래그, `/api/capabilities`에 `appVersion`, 설정 페이지 하단에 표시.

### 5.2 설치 경로

1. Helm OCI(기존): `helm install sealane oci://ghcr.io/wonjune95/charts/sealane --version <v> -n sealane --create-namespace`.
2. `kubectl apply -f https://github.com/wonjune95/sealane/releases/latest/download/install.yaml`: release.yml이 `helm template sealane charts/sealane -n sealane --set sessionSecret.create=false`로 렌더하고 `Namespace sealane`을 앞에 붙여 GitHub Release 자산으로 첨부(`softprops/action-gh-release`, `generate_release_notes: true`). 릴리스 노트 카테고리는 `.github/release.yml`(labels: `enhancement`, `bug`, `documentation`, `dependencies`). CHANGELOG 파일은 두지 않는다.
3. Artifact Hub: `Chart.yaml`에 `home`, `sources`, `maintainers`, `icon`(raw GitHub SVG URL), `keywords`, `annotations.artifacthub.io/license`·`/links`·`/images`. 사용자가 artifacthub.io에 OCI 저장소 `oci://ghcr.io/wonjune95/charts/sealane`를 등록한다(계정 필요). verified publisher는 v1.0에서.

### 5.3 차트

- `charts/sealane/README.md`(values 표, 수기), `values.schema.json`(전 키, 최상위 `additionalProperties: false`), `templates/NOTES.txt`(port-forward 안내, 초기 계정 `admin`/`admin` + 변경 강제 안내).
- 새 템플릿: `ingress.yaml`(`ingress.enabled`, `className`, `hosts`, `tls`, `annotations`), `httproute.yaml`(`httpRoute.enabled`, `parentRefs`, `hostnames`), `secret-session.yaml`.
- `secret-session.yaml`: `sessionSecret.existingSecret`이 있으면 그것을 쓰고, 없고 `sessionSecret.create: true`(기본)면 `lookup`으로 기존 값을 유지하는 랜덤 32자 Secret을 만들어 `SESSION_SECRET`으로 주입한다. 재시작·복제본 간 세션이 유지된다. install.yaml은 `sessionSecret.create=false`로 렌더(앱은 기존대로 부팅마다 랜덤). 이 values 키는 `values.schema.json`에 함께 들어간다.
- CI: `helm lint` + `helm template`.

### 5.4 `/readyz`

로컬 클러스터 discovery `ServerVersion()` 핑(타임아웃 2초). 실패 시 503. 한 함수.

### 5.5 CI 품질 게이트

`ci.yml`에 golangci-lint(기본 린터 세트, `.golangci.yml` 최소), `govulncheck ./...`, helm lint/template을 추가한다(`hack/check-i18n.sh`는 0.8에서 이미 들어감). 별도 워크플로: `codeql.yml`(go, javascript), `scorecard.yml`(OpenSSF Scorecard 배지), `.github/dependabot.yml`(gomod·npm·github-actions 주간, 그룹화). 액션은 SHA로 고정(Dependabot이 갱신). 릴리스 시 Trivy 이미지 스캔은 결과만 남기고 실패시키지 않는다. 가장 먼저 자를 것은 Scorecard, 그다음 Trivy.

### 5.6 e2e (`e2e.yml`, PR·main)

1. `docker build -t sealane:ci .` (amd64)
2. `helm/kind-action`으로 kind 클러스터
3. Gateway API CRD: `kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.<x>/standard-install.yaml` (버전은 구현 계획에서 go.mod의 gateway-api 버전에 맞춰 고정)
4. istioctl(버전 고정) `istioctl install -y --set profile=minimal`
5. `kind load docker-image sealane:ci`
6. `helm install sealane charts/sealane -n sealane --create-namespace --set image.repository=sealane --set image.tag=ci --set image.pullPolicy=Never`, `kubectl rollout status`
7. `kubectl port-forward` 백그라운드 → `hack/e2e.sh http://localhost:8080`

`hack/e2e.sh`(bash+curl+jq): `/healthz`·`/readyz` → `admin`/`admin` 로그인 → 비밀번호 변경 강제 확인 후 변경 → `/api/capabilities`(istio·gateway api 감지, appVersion) → VirtualService dry-run 생성 → 생성 → 목록에서 확인 → `/api/flowmap` → 삭제. 로컬 클러스터에서도 같은 스크립트를 쓴다. Playwright는 CI에 넣지 않는다(픽스처 기반이라 설치 회귀와 무관).

호환 매트릭스 문서: e2e가 도는 K8s(kind 노드)·Istio·Gateway API 버전과 라이브 검증 버전(K8s 1.34, Istio 1.31.1).

### 5.7 검증

CI 전부 녹색(e2e 포함). `skopeo inspect --raw`로 매니페스트 리스트에 amd64·arm64 확인. `cosign verify` 성공. `helm install --set replicaCount=abc`가 스키마 오류로 거부. install.yaml을 CI/CD 클러스터 임시 네임스페이스에 apply → 로그인까지 확인 → 삭제. 이후 `v0.9.0`.

---

## 6. v0.10.0 — SSO

### 6.1 OIDC

- 의존: `github.com/coreos/go-oidc/v3`(신규), `golang.org/x/oauth2`(이미 간접 의존 → 직접). 직접 구현하지 않는다.
- 코드: `internal/auth/oidc.go`(provider 초기화, verifier, 역할 매핑), `internal/api/oidc.go`(핸들러 2개 + methods).
- 환경변수(차트가 values에서 주입):

| env | values | 설명 |
|---|---|---|
| `OIDC_ISSUER_URL` | `oidc.issuerURL` | 비우면 OIDC 비활성 |
| `OIDC_CLIENT_ID` | `oidc.clientID` | |
| `OIDC_CLIENT_SECRET` | `oidc.clientSecretRef{name,key}` → `secretKeyRef` | 차트는 Secret을 만들지 않는다 |
| `OIDC_REDIRECT_URL` | `oidc.redirectURL` | 명시 필수(요청 Host에서 유도하지 않음) |
| `OIDC_SCOPES` | `oidc.scopes` (기본 `openid profile email groups`) | |
| `OIDC_GROUPS_CLAIM` | `oidc.groupsClaim` (기본 `groups`) | |
| `OIDC_USERNAME_CLAIM` | `oidc.usernameClaim` (기본 `email`) | |
| `OIDC_ROLES_ADMIN` / `_EDITOR` / `_VIEWER` | `oidc.roles.admin` 등 (그룹 목록; env는 쉼표 구분) | 여러 그룹에 속하면 admin > editor > viewer |
| `OIDC_DEFAULT_ROLE` | `oidc.defaultRole` (기본 `viewer`, 빈값 = 매핑 없는 사용자 거부) | |
| `OIDC_DISPLAY_NAME` | `oidc.displayName` (기본 `SSO`) | 로그인 버튼 문구 |

- 흐름: `GET /api/auth/oidc/login` → state·nonce·PKCE verifier를 10분짜리 HMAC 서명 쿠키 `sealane_oidc`(HttpOnly, SameSite=Lax)에 담고 IdP로 리다이렉트. `GET /api/auth/oidc/callback?code&state` → state 쿠키 검증 → 코드 교환(PKCE) → ID 토큰 검증(issuer·aud·nonce·만료) → username·groups 추출 → 역할 매핑 → 매핑 실패면 403 페이지 → 성공이면 기존 `sealane_session` 쿠키 발급 후 `/`로 리다이렉트. 세션 구조는 그대로라 무상태 HA 유지.
- `auth.Identity`에 `Provider`(`local`|`oidc`) 추가, 세션 페이로드 `user|role|exp|provider`. 0.10 배포 시 기존 세션 한 번 무효화(재로그인).
- UI: 비인증 `GET /api/auth/methods` → `{"local":true,"oidc":{"enabled":true,"displayName":"Keycloak"}}`. 로그인 화면에 "Sign in with <displayName>" 버튼. OIDC 사용자에겐 비밀번호 변경 UI 숨김. 로컬 로그인은 항상 유지(IdP 장애 시 잠금 방지). 계정 관리 UI는 admin이면 provider와 무관하게 사용.
- 감사 로그·변경 히스토리에 `provider` 포함.

### 6.2 SESSION_SECRET

v0.9의 `secret-session.yaml`(§5.3)이 전제다. OIDC state 쿠키와 세션 쿠키가 같은 키로 서명되므로, OIDC를 켜는 설치는 `sessionSecret.create=true`(기본) 또는 `existingSecret`이 필요하다. install.yaml로 설치한 경우 OIDC 사용 전에 `SESSION_SECRET`을 직접 넣어야 함을 문서에 명시한다.

### 6.3 테스트

- 단위: httptest로 가짜 OIDC 제공자(`/.well-known/openid-configuration`, `/jwks`, `/token`, 테스트 RSA 키로 ID 토큰 서명). 케이스: 정상 로그인→콜백→세션 쿠키와 역할, 그룹 우선순위, 잘못된 state 거부, nonce 불일치 거부, 그룹 없음 + `defaultRole=""` 거부, 만료 토큰 거부.
- 수동 1회: CI/CD 클러스터의 ArgoCD Dex(`devops-tools/argocd-dex-server`)에 static client를 등록해 실제 로그인 확인(ArgoCD 설정 변경이므로 그 시점에 사용자 동의 후 진행). 대안은 kind에 Dex를 띄우는 것.

### 6.4 문서 사이트

- `mkdocs.yml` + `mkdocs-material`, `.github/workflows/docs.yml`이 main 푸시마다 GitHub Pages(Actions 소스)로 배포. 사용자가 Pages를 켠다. URL `https://wonjune95.github.io/sealane`.
- 페이지: Home / Getting started(설치 3경로, 첫 로그인) / Architecture & security model(단일 SA, 역할, 쓰기 안전장치, 무상태) / Authentication(로컬 계정, OIDC 공통, Keycloak·Dex·Google 예시) / Multi-cluster / Features(resource editor, flow map, route check, request tester) / Configuration reference(플래그·env·values) / Operations(HA, metrics, upgrading + istio-dashboard→sealane 마이그레이션) / Compatibility / Security(하드닝, 신고) / Contributing / Releasing / FAQ.
- README는 랜딩(설치 한 줄, 스크린샷, 기능 요약, 문서 링크)으로 축소. 내용 대부분은 기존 README에서 옮긴다.

### 6.5 검증

단위 테스트 + CI 녹색, Dex로 실제 로그인 1회, `replicaCount=2`에서 로그인 후 파드 재시작해도 세션 유지 확인, 문서 사이트 빌드·배포 확인. 이후 `v0.10.0`.

---

## 7. v1.0.0 — 정책 + 생태계

새 기능 없음. v0.10.x에 약속을 얹는다.

### 7.1 정책 문서

- `docs/versioning.md`: SemVer. 1.x 안에서 Helm values·환경변수·설정 형식·ConfigMap/Secret 포맷의 비호환 변경 금지(폐기는 한 minor 전에 예고). `/api/*`는 UI 전용 내부 계약이며 공개 API가 아님을 명시. 지원: 최신 minor에만 수정 릴리스(단일 메인테이너). 호환: K8s 최근 3 minor, Istio 최근 2 minor, Gateway API v1.x — e2e를 Istio 2개 버전 매트릭스로 확장해 문서와 일치시킨다. 릴리스 주기는 고정하지 않는다.
- `GOVERNANCE.md`: 단일 메인테이너, 결정 방식(이슈/PR 공개 논의), 메인테이너 추가 기준(지속 기여 + 기존 메인테이너 승인).
- `ADOPTERS.md`: 첫 항목은 사용자의 회사(회사명 표기 가능 여부는 그때 확인).
- `ROADMAP.md`를 1.0 이후 항목으로 갱신(impersonation 모드, 에러 코드 i18n, 문서 한국어판 등 후보).

### 7.2 1.0 게이트

v0.8 → v1.0 `helm upgrade` 검증, e2e 매트릭스 녹색, 문서 사이트 전 페이지 완성, 열린 P1 버그 0. 충족 시 `v1.0.0`.

### 7.3 생태계 제출 (내용은 Claude가 준비, 제출은 사용자 계정으로)

- **Artifact Hub verified publisher**: `artifacthub-repo.yml`을 `oras`로 OCI 저장소에 푸시.
- **istio.io 통합 페이지 PR**: 수락은 메인테이너 재량, 비용은 낮으니 시도.
- **공개 알림 문안** `docs/announcement.md`: Istio Slack, r/kubernetes, Show HN, GeekNews·K8s Korea.
- **CNCF Landscape**: 조사 결과 로고는 **영어 프로젝트명이 포함된 SVG(텍스트는 패스로), 스택형, 배경색 없음**이어야 하고 프로젝트 항목은 **GitHub 약 300스타**를 기대한다. 현재 파비콘은 텍스트가 없고 파란 배경 사각형이 있어 그대로 쓸 수 없다. 따라서 Landscape PR은 스타 기준을 넘긴 뒤로 미루되, 준비물(`hack/landscape/`에 landscape.yml 항목 스니펫 + 투명 배경·워드마크 SVG, fontTools로 텍스트→패스 변환)은 v1.0에서 만들어 둔다.

### 7.4 나중으로

CNCF Sandbox 신청(외부 이슈·채택사가 생긴 뒤; CoC·라이선스·DCO·거버넌스·보안 정책 요건은 이미 충족), istio-ecosystem GitHub 조직 편입 제안(절차는 `istio/community`에서 확인 필요, 검색으로는 미확인), 별도 조직 `sealane-io` 생성.

---

## 8. 공통 사항

### 8.1 사용자가 직접 하는 GitHub/외부 작업

| 시점 | 작업 |
|---|---|
| v0.8 | 저장소 이름 변경, 설명·topics·website, Discussions 켜기, 비공개 취약점 신고 켜기, (선택) social preview |
| v0.9 | artifacthub.io 계정 + OCI 저장소 등록, (선택) main 브랜치 보호에 CI 필수 설정 |
| v0.10 | GitHub Pages 켜기(소스: GitHub Actions), Dex static client 등록 동의 |
| v1.0 | 생태계 PR·알림 제출, ADOPTERS 회사명 확인 |

### 8.2 리스크

- 리네임 후 옛 GHCR 패키지와 새 패키지가 공존한다. 옛 것은 지우지 않고 README에서만 새 이름을 안내한다.
- i18n PR은 UI 파일 대부분을 건드린다. 리네임 직후, 다른 UI 변경 전에 단독으로 머지한다.
- OIDC 도입 시 세션 페이로드가 바뀌어 한 번 전원 재로그인된다. 릴리스 노트에 명시.
- `docs/superpowers/`는 공개 저장소에 그대로 노출된다. 계획 문서로 두는 데 문제는 없지만, 원치 않으면 `.gitignore`에 추가할 수 있다.
- 단일 메인테이너의 이슈 응답 속도는 사용자 몫이다. README에 best-effort임을 적는다.

### 8.3 테스트 원칙

기존 Go 단위 테스트(fake client)와 Playwright 픽스처 테스트를 유지한다. 새로 추가하는 검사는 각 릴리스에 하나씩: `hack/check-i18n.sh`(0.8), `hack/e2e.sh`(0.9), OIDC 가짜 제공자 단위 테스트(0.10), e2e 매트릭스(1.0). 프레임워크 추가 없음.

## 9. 조사 기록

- 이름 후보 검사(2026-10-13): GitHub `in:name` 검색, `rdap.org` `.io` 조회, Artifact Hub 검색, 웹 검색. 탈락: Rutter(핀테크 회사), Rhumb(`rhumb-mcp` 개발 도구), Fairway(customerio/fairway 등), Strait·Chartroom(일반어·기존 프로젝트), Tideway(Tideways 프로파일러), Portolan(portolan-sdi 지리정보 CLI와 겹침), Meshway(조직명 선점, 항해 테마 이탈).
- Istio 상표: 2020년 Google이 Open Usage Commons로 이전. 공식 사용 가이드 문서는 검색으로 확인하지 못했으나, 서술적 "for Istio" 사용과 고유 제품명이 생태계 관행.
- CNCF Landscape 요건: SVG 로고(영어 이름 포함, 스택형, 배경색 없음), 알파벳순 `landscape.yml` 항목, 프로젝트는 약 300스타 기대.
- CNCF Sandbox: `cncf/sandbox` 저장소 이슈 폼으로 신청, TOC 투표. 사전 실사 없음.
- Artifact Hub OCI 차트: 차트당 저장소 1개, 태그는 semver, 메타데이터는 Chart.yaml·README·LICENSE에서 추출, 선택 주석 `artifacthub.io/*`.
