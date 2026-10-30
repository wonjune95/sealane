# 인증

> 영문 원문: [Authentication](../authentication.md). 내용이 다르면 영문이 기준입니다.

Sealane의 로그인 방식은 두 가지이며, 둘을 동시에 켤 수 있습니다.

- **로컬 계정** — 사용자 이름, 역할, bcrypt 해시를 ConfigMap에 저장합니다. 항상 사용할 수 있습니다.
- **SSO(OIDC)** — OpenID Connect 제공자(Keycloak, Dex, Google 등)가 사용자를 인증하고, 사용자가 속한 그룹으로 역할을 정합니다. 선택 기능이며 기본값은 꺼짐입니다.

어느 방식이든 결과는 같습니다. 사용자 이름과 세 역할 중 하나를 담은 HMAC 서명 세션 쿠키가 발급됩니다. 클러스터 호출은 항상 Sealane의 ServiceAccount로 실행됩니다. 역할은 사용자가 Sealane 안에서 무엇을 할 수 있는지만 정합니다([권한 모델](../architecture.md#permission-model) 참고).

| 역할 | 할 수 있는 일 |
|---|---|
| `viewer` | 모든 리소스 조회, 경로 확인 사용 |
| `editor` | viewer가 할 수 있는 모든 일에 더해 리소스 생성·수정·삭제 |
| `admin` | editor가 할 수 있는 모든 일에 더해 계정·클러스터 관리, 요청 테스터 사용(활성화한 경우) |

## 로컬 계정

### 계정 ConfigMap

`sealane-accounts` ConfigMap의 키 하나가 계정 하나입니다. 값은 `role:bcryptHash` 형식입니다.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: sealane-accounts
  namespace: sealane
data:
  admin: "admin:<bcrypt-hash>"
  # viewer1: "viewer:<bcrypt-hash>"
```

ConfigMap은 파드가 아니라 클러스터에 저장됩니다. 따라서 재시작이나 재배포 후에도 계정이 유지됩니다. 차트와 `install.yaml`은 이 ConfigMap을 빈 상태로 만들고(이름은 `accountsConfigMap` 값을 따릅니다), Sealane이 초기 `admin`을 추가합니다. 그 뒤로는 UI나 kubectl로 편집합니다. 업그레이드해도 기존 키는 그대로 남고, `helm uninstall` 후에도 ConfigMap은 지워지지 않습니다. Sealane이 API로 이 ConfigMap을 읽으므로 편집 내용은 **재시작 없이 몇 초 안에 반영되고**, 계정을 지우거나 역할·비밀번호를 바꾸면 그 전에 발급된 세션이 종료됩니다. 전체 예시는 [`examples/accounts-configmap.yaml`](https://github.com/wonjune95/sealane/blob/main/examples/accounts-configmap.yaml)에 있습니다.

GitOps로 운영하면서 UI 편집을 쓰지 않는다면, 계정을 Helm 값으로 미리 넣을 수 있습니다.

```yaml
accounts:
  admin: "admin:$2a$10$..."     # go run ./hack/bcrypt-hash.go 'password'
  viewer1: "viewer:$2a$10$..."
```

Helm은 정확히 이 키들만 관리합니다. values에서 키를 바꾸거나 지우면 다음 업그레이드 때 클러스터에서도 바뀌거나 지워집니다. UI에서 추가한 키는 건드리지 않습니다. values의 값이 바뀌지 않은 키라면 UI에서 바꾼 비밀번호도 그대로 둡니다(Helm은 두 렌더링 결과 사이에서 달라진 부분만 패치합니다).

### 계정 추가

admin은 **설정 → 계정 관리**에서 계정을 추가하고, 역할과 비밀번호를 바꾸고, 계정을 삭제할 수 있습니다. 잠김을 막기 위해 자기 자신은 삭제할 수 없고 자신의 역할도 바꿀 수 없습니다. `admin` 계정도 삭제할 수 없습니다.

`kubectl`로도 할 수 있습니다.

```bash
go run ./hack/bcrypt-hash.go 'password'                      # generate bcrypt hash
# (without Go) htpasswd -bnBC 10 "" 'password' | tr -d ':\n'
kubectl -n sealane edit configmap sealane-accounts
# add to data:  username: "role:hash"   (role: admin | editor | viewer)
```

키를 추가하면 계정이 추가됩니다. 역할 문자열을 고치면 역할이 바뀌고, 키를 지우면 계정이 삭제됩니다.

### 초기 admin

시작할 때 admin 역할을 가진 계정이 하나도 없으면, Sealane은 `admin` 계정을 임의 비밀번호로 만들고 그 비밀번호를 Secret `sealane-initial-admin`(Helm이면 `<release>-initial-admin`)의 `password` 키에 넣습니다.

```bash
kubectl -n sealane get secret sealane-initial-admin -o jsonpath='{.data.password}' | base64 -d; echo
```

처음 로그인한 뒤 비밀번호를 바꾸세요. Sealane은 관리자 계정이 없을 때만 이 Secret을 다시 읽으므로, 바꾼 뒤에는 지워도 됩니다. 1.19 이전에서 업그레이드한 설치에서 `admin` 비밀번호가 아직 `admin`이면 먼저 바꿔야 합니다. 그 전까지 서버가 다른 요청을 모두 거부합니다.

### 내 비밀번호 변경

로컬 계정이 있으면 viewer를 포함해 누구나 자기 비밀번호를 바꿀 수 있습니다. 헤더의 사용자 칩 → 설정 페이지에서 현재 비밀번호를 입력한 뒤 변경합니다. 새 비밀번호는 8자 이상이어야 합니다. 서버는 ConfigMap에서 본인의 키만 패치하고, 지금 세션은 유지하며 다른 곳의 세션은 종료합니다.

## SSO(OIDC)

### 동작 방식

- **PKCE를 쓰는 인가 코드(authorization code) 흐름.** 로그인 페이지의 **…(으)로 로그인** 버튼은 `/api/auth/oidc/login`으로 이동하고, 이 엔드포인트가 제공자로 리다이렉트합니다. 제공자는 브라우저를 `/api/auth/oidc/callback`으로 돌려보냅니다. state, nonce, PKCE verifier는 수명이 짧은(10분) 서명 쿠키에 담겨 전달됩니다. 따라서 어느 복제본이든 흐름을 마무리할 수 있습니다.
- **ID 토큰 검증.** Sealane은 인가 코드를 토큰으로 교환한 뒤 ID 토큰의 서명, audience(클라이언트 ID), 만료 시각, nonce를 검증합니다.
- **사용자 이름.** `usernameClaim`이 가리키는 클레임(기본값 `email`)에서 가져옵니다. 이 클레임이 없거나 비어 있으면 토큰의 `sub`를 씁니다.
- **그룹에서 역할로.** `groupsClaim`이 가리키는 클레임(기본값 `groups`)은 문자열 목록이어야 합니다. `oidc.roles.admin`, `oidc.roles.editor`, `oidc.roles.viewer`에 역할별 IdP 그룹을 나열합니다. 여러 역할에 해당하면 **가장 높은 역할이 적용됩니다**(admin, editor, viewer 순). 그룹은 ID 토큰에서 읽습니다. 토큰에 groups 클레임이 아예 없으면 Sealane이 제공자의 userinfo 엔드포인트에서 가져옵니다(Argo CD의 `enableUserInfoGroups`와 같은 방식). userinfo 호출이 실패해도 오류가 되지는 않습니다. 그룹 정보만 빠지고, 사용자는 `defaultRole`을 받습니다.
- **`defaultRole`.** 일치하는 그룹이 없는 사용자는 `defaultRole`을 받습니다. 차트의 기본값은 `viewer`입니다. `""`로 설정하면 이런 사용자를 거부하며, 사용자에게는 접근 거부 메시지가 표시됩니다.
- **로컬 로그인은 그대로 유지됩니다.** 로그인 페이지에 사용자 이름·비밀번호 폼이 남아 있고, 초기 `admin`과 다른 로컬 계정도 계속 동작합니다. IdP 장애에 대비해 비상용(break-glass) 로컬 admin 계정을 하나 이상 남겨 두는 것이 좋습니다.
- **세션 쿠키는 같습니다.** SSO 로그인도 로컬 로그인과 똑같은 HMAC 서명 세션 쿠키(12시간)를 발급받습니다. 역할은 로그인 시점에 정해져 쿠키에 담깁니다. 따라서 IdP에서 바꾼 그룹은 사용자가 다음에 로그인할 때 반영됩니다. Sealane에서 로그아웃하면 쿠키만 지워지고, IdP에서는 로그아웃되지 않습니다.
- **Sealane에는 비밀번호가 없습니다.** SSO 사용자의 설정 페이지에는 비밀번호 폼 대신 "…을(를) 통해 로그인했습니다"라는 문구가 표시됩니다. SSO 사용자는 계정 ConfigMap에 기록되지 않고, 계정 목록에도 나타나지 않습니다.
- **Secure 쿠키.** `OIDC_REDIRECT_URL`이 `https://`이면 세션 쿠키와 로그인 쿠키에 자동으로 `Secure` 속성이 붙습니다. 따라서 사용자도 https로 Sealane에 접속해야 합니다. 브라우저는 일반 http에서 `Secure` 쿠키를 버립니다.
- **실패 메시지.** 로그인에 실패하면 브라우저는 `?oidc_error=<code>`를 달고 로그인 페이지로 돌아가며, 일반적인 안내 메시지만 표시됩니다. 자세한 내용은 서버 로그에만 남습니다. 세 가지 코드는 [문제 해결](../authentication.md#troubleshooting)에 정리되어 있습니다.

로그인 페이지는 `GET /api/auth/methods`(인증 불필요)로 사용할 수 있는 방식을 확인합니다. 응답에는 로컬 로그인 활성화 여부와, SSO가 설정된 경우 그 표시 이름이 들어 있습니다.

### Helm 값

제공자에서 리다이렉트 URI를 `https://<your-sealane-host>/api/auth/oidc/callback`으로 지정해 클라이언트를 만듭니다. 클라이언트 시크릿은 Sealane 네임스페이스의 Secret에 저장하고, `oidc`를 활성화합니다.

```bash
kubectl -n sealane create secret generic sealane-oidc --from-literal=clientSecret='<client-secret>'
```

```yaml
oidc:
  enabled: true
  displayName: Keycloak            # login button: "Sign in with Keycloak"
  issuerURL: https://keycloak.example.com/realms/main
  clientID: sealane
  clientSecretRef: { name: sealane-oidc, key: clientSecret }
  redirectURL: https://sealane.example.com/api/auth/oidc/callback
  scopes: [openid, profile, email, groups]   # default
  groupsClaim: groups                        # default
  usernameClaim: email                       # default
  roles:
    admin: [platform-admins]
    editor: [sre]
    viewer: []
  defaultRole: viewer                        # "" refuses users without a matching group
```

`issuerURL`, `clientID`, `redirectURL`은 필수입니다. 하나라도 없으면 차트가 렌더링되지 않습니다. 값은 `<release>-cm` ConfigMap에 `OIDC_*` 키로 렌더링되고, 파드는 이 키를 환경 변수로 읽습니다(구성 문서의 [SSO 변수](../configuration.md#single-sign-on-variables) 참고). 설정이 불완전하거나 `defaultRole` 값이 잘못되면 서버가 시작 단계에서 멈추고 로그에 `oidc config invalid`를 남깁니다. `clientSecretRef.name`을 비워 두면 시크릿을 `<release>-secret` Secret의 `OIDC_CLIENT_SECRET` 키에서 읽습니다.

### Helm 없이 설치(install.yaml)

`install.yaml`에도 같은 `sealane-cm` ConfigMap이 빈 상태로 들어 있고, 모든 키가 주석으로 나열되어 있습니다(Argo CD의 `argocd-cm`과 같은 방식). 설정은 이 ConfigMap에, 클라이언트 시크릿은 `sealane-secret`에 넣은 다음 파드를 재시작합니다.

```bash
kubectl -n sealane create secret generic sealane-secret --from-literal=OIDC_CLIENT_SECRET='<client-secret>'
kubectl -n sealane edit configmap sealane-cm
#   data:
#     OIDC_ISSUER_URL: https://keycloak.example.com/realms/main
#     OIDC_CLIENT_ID: sealane
#     OIDC_REDIRECT_URL: https://sealane.example.com/api/auth/oidc/callback
#     OIDC_ROLES_ADMIN: platform-admins
#     OIDC_DEFAULT_ROLE: viewer
#     OIDC_DISPLAY_NAME: Keycloak
kubectl -n sealane rollout restart deploy/sealane
```

`OIDC_ISSUER_URL`을 설정하는 즉시 SSO가 켜집니다. 현재 설정은 `kubectl -n sealane get cm sealane-cm -o yaml`로 확인할 수 있습니다. 이 방식에서는 `OIDC_DEFAULT_ROLE`의 기본값이 빈 값이라는 점에 주의해야 합니다(일치하는 그룹이 없는 사용자는 거부됩니다). 반면 차트의 `oidc.defaultRole` 기본값은 `viewer`입니다.

0.9에서 기존 릴리스를 업그레이드할 때는 [업그레이드](../upgrading.md) 문서대로 `--reset-then-reuse-values`(Helm 3.14+)를 쓰거나 values를 명시적으로 넘깁니다. 그냥 `--reuse-values`만 쓰면 새 `oidc` 기본값이 병합되지 않습니다. 그러면 `defaultRole`이 빈 값이 되어, 일치하는 그룹이 없는 사용자가 거부됩니다.

### 알아 둘 점

- **로컬 계정과 SSO 사용자는 이름 공간을 공유합니다.** Sealane은 사용자를 이름으로만 구분합니다. SSO 사용자의 이름은 `usernameClaim`(기본값 `email`)에서 옵니다. 로컬 계정과 SSO 사용자의 이름이 같아지면 "자신의 역할 변경 불가", "자기 자신 삭제 불가" 검사가 둘을 같은 사용자로 취급합니다. 변경 이력과 감사 기록에는 이름 옆에 제공자(`local` 또는 `oidc`)가 함께 남으므로 누구의 작업인지는 구분됩니다. 로컬 계정 이름에는 `@`를 쓸 수 없습니다(영문자, 숫자, `-`, `.`, `_`만 허용). 그래서 기본 `email` 클레임을 쓰면 두 이름이 겹칠 수 없습니다. 하지만 `usernameClaim`을 `preferred_username` 같은 값으로 바꾸거나, 토큰에 클레임이 없어 `sub`로 대체되면 겹칠 수 있습니다. 로컬 계정 이름은 SSO 사용자 이름과 다르게 짓는 것이 좋습니다.
- **confidential 클라이언트만 지원합니다.** Sealane은 클라이언트 시크릿을 써서 인가 코드를 교환합니다. 따라서 IdP 클라이언트는 confidential이어야 하고 클라이언트 시크릿이 필요합니다. PKCE만 쓰는 public 클라이언트는 지원하지 않습니다.
- **`email_verified`는 확인하지 않습니다.** 사용자 이름 클레임은 표시와 감사 기록에만 쓰입니다. 역할은 groups 클레임과 역할 매핑으로 정해집니다.
- **디스커버리는 필요할 때 수행됩니다.** Sealane은 제공자의 디스커버리 문서를 부팅 시점이 아니라 첫 SSO 로그인 때 가져옵니다. IdP에 접속할 수 없거나 설정이 잘못되어도 시작, 헬스 체크, 로컬 로그인은 막히지 않습니다. SSO 로그인만 `oidc_error=idp`로 실패하고, 다음 시도에서 디스커버리를 다시 시도합니다.
- **SSO 전용 로그인 화면.** `oidc.hideLocalLogin: true`(환경 변수 `LOCAL_LOGIN=hidden`)로 설정하면 사용자 이름·비밀번호 폼을 숨기고 SSO 버튼만 보여 줍니다. 비상시에는 `/?local=1`로 접속하면 폼이 다시 나타납니다. 이 설정은 UI 옵션일 뿐 보안 경계가 아닙니다. API는 여전히 로컬 로그인을 받고, 로컬 계정도 유효합니다. SSO로만 로그인하게 하려면 로컬 계정을 삭제해야 합니다.
- **복제본이 여러 개면 세션 키를 공유해야 합니다.** 세션 쿠키와 수명이 짧은 로그인 쿠키는 모두 HMAC으로 서명됩니다. 복제본이 둘 이상이면 모든 파드가 같은 키를 써야 합니다. 차트의 `sessionSecret`(기본)을 쓰거나, 이 값이 없으면 Sealane이 첫 시작 때 `sealane-secret` Secret에 생성한 키를 씁니다. 둘 다 없을 때만 파드마다 자체 무작위 키로 서명합니다. 이때는 요청이 다른 파드로 가면 세션이 끊깁니다. 로그인을 시작한 파드와 다른 파드가 콜백을 처리하면 `oidc_error=state`로 실패합니다.

### 제공자별 참고 사항

!!! note "참고"
    아래 콘솔 절차는 각 제공자의 문서를 바탕으로 쓴 출발점입니다. 실제 Keycloak이나 Google 환경에서 검증하지는 않았습니다. 최신 제공자 문서와 대조해 확인하는 것이 좋습니다. Keycloak의 [client scopes](https://www.keycloak.org/docs/latest/server_admin/#_client_scopes)와 [protocol mappers](https://www.keycloak.org/docs/latest/server_admin/#_protocol-mappers), Google의 [OpenID Connect 가이드](https://developers.google.com/identity/openid-connect/openid-connect)를 참고합니다.

#### Keycloak

1. realm에서 클라이언트를 만듭니다. 클라이언트 유형은 **OpenID Connect**, 클라이언트 ID는 `sealane`으로 정합니다. **Client authentication**과 **Standard flow**를 켜고, 유효한 리다이렉트 URI에 `https://sealane.example.com/api/auth/oidc/callback`을 넣습니다. 클라이언트 시크릿은 **Credentials** 탭에서 복사합니다.
2. groups 클레임을 추가합니다. `groups`라는 client scope를 만들고 **Group Membership** 매퍼를 붙입니다. 매퍼는 토큰 클레임 이름 `groups`, **Add to ID token** 켬, **Full group path** 끔으로 설정합니다. **Full group path**를 꺼야 그룹이 `/platform-admins`가 아니라 `platform-admins`로 들어옵니다. 그다음 이 scope를 `sealane` 클라이언트에 추가합니다. scope 이름을 `groups`로 지으면 기본 `scopes` 목록과 그대로 맞습니다.
3. realm의 발급자(issuer)를 씁니다. `issuerURL: https://keycloak.example.com/realms/<realm>`

#### Dex

실제 IdP 없이 SSO를 시험하려면 [`examples/dex-demo.yaml`](https://github.com/wonjune95/sealane/blob/main/examples/dex-demo.yaml)을 배포합니다. 이 파일은 Dex를 **mock connector**로 실행합니다. 모든 로그인이 `authors` 그룹에 속한 `kilgore@kilgore.trout` 사용자로 성공합니다. 절차는 파일 맨 위 주석에 있습니다. 두 `example.com` 호스트를 바꾸고(Dex는 브라우저**와** Sealane 파드 양쪽에서 같은 발급자 URL로 접근할 수 있어야 합니다), 파일을 적용한 다음 Sealane을 다음과 같이 업그레이드합니다.

```bash
helm upgrade sealane oci://ghcr.io/wonjune95/charts/sealane --version 1.19.1 -n sealane --reset-then-reuse-values \
  --set oidc.enabled=true --set oidc.displayName=Dex \
  --set oidc.issuerURL=http://dex.example.com/dex --set oidc.clientID=sealane \
  --set oidc.clientSecretRef.name=sealane-dex-client \
  --set oidc.redirectURL=http://sealane.example.com/api/auth/oidc/callback \
  --set 'oidc.roles.admin={authors}'
```

이 데모는 운영 환경용이 아닙니다. [`web/tests/oidc-dex.mjs`](https://github.com/wonjune95/sealane/blob/main/web/tests/oidc-dex.mjs)가 이 데모를 상대로 실제 왕복 흐름(SSO 로그인, 설정 페이지, 로그아웃)을 실행합니다. 자세한 내용은 [`web/tests/README.md`](https://github.com/wonjune95/sealane/blob/main/web/tests/README.md)에 있습니다.

실제 Dex에서는 그룹이 커넥터(예: LDAP 그룹 검색, GitHub 팀)에서 옵니다. Dex는 `groups` scope를 요청받았을 때만 ID 토큰에 그룹을 넣습니다. 기본 `scopes`에는 이 scope가 들어 있습니다.

#### Gitea

Gitea는 별도 설정 없이 OpenID Connect 제공자로 동작합니다. `issuerURL`은 끝에 슬래시가 없는 Gitea 루트 URL입니다(디스커버리 문서가 이 형태로 알려 줍니다). `groups` 클레임에는 사용자의 조직이 `org` 형태로, 팀이 `org:team` 형태로(팀 이름은 소문자) 들어갑니다. 따라서 `roles.admin: [myorg:admins]`처럼 쓸 수 있습니다.

Sealane **전용** OAuth2 애플리케이션을 만듭니다(Site Administration → Integrations → Applications, confidential 클라이언트, 리다이렉트 URI `https://sealane.example.com/api/auth/oidc/callback`). Gitea는 사용자가 애플리케이션을 처음 승인할 때 허용한 scope를 저장해 두고, 이후 로그인에서도 그 승인을 재사용합니다. 그래서 `groups`를 요청한 적 없는 다른 클라이언트와 애플리케이션을 공유하면 그룹이 빠진 ID 토큰이 계속 발급됩니다. 1.1.1 이전에는 이 경우 모든 사용자가 `defaultRole`을 받았습니다. 1.1.1부터는 공유 애플리케이션도 동작합니다. ID 토큰에 그룹이 없으면 Sealane이 Gitea의 userinfo 엔드포인트에서 그룹을 읽기 때문입니다. 그래도 전용 애플리케이션이 더 깔끔한 구성입니다.

#### Google

- 발급자는 `https://accounts.google.com`입니다. 유형이 **Web application**인 OAuth 클라이언트를 만들고 위의 리다이렉트 URI를 등록합니다.
- Google에는 `groups` scope가 없고, ID 토큰에도 groups 클레임이 없습니다. 따라서 `scopes: [openid, profile, email]`로 설정하고 `usernameClaim: email`은 그대로 둡니다.
- 그룹이 없으므로 역할 매핑은 항상 `defaultRole`로 귀결됩니다. 모든 Google 사용자에게 `viewer`를 주고 변경 작업용 로컬 `admin` 계정을 유지하거나, 의도적으로 더 높은 `defaultRole`을 설정합니다. 로그인을 마친 모든 Google 계정이 `defaultRole`을 받습니다. 따라서 로그인할 수 있는 사용자를 Google 쪽에서 제한해야 합니다(Google Workspace 조직이라면 OAuth 동의 화면을 **Internal**로 설정).

#### 사설 CA

Sealane은 발급자의 TLS 인증서를 검증할 때 이미지의 시스템 루트 인증서와 `sealane-tls-certs` ConfigMap(Helm으로 설치했다면 `<release>-tls-certs`)의 모든 인증서를 신뢰합니다. Argo CD의 `argocd-tls-certs-cm`과 같은 방식입니다. `install.yaml`과 차트 모두 이 ConfigMap을 빈 상태로 만듭니다. CA 인증서(PEM, 키 이름은 자유)를 추가한 뒤 재시작합니다. Go는 신뢰 루트를 한 번만 읽기 때문입니다.

```bash
kubectl create configmap sealane-tls-certs --from-file=corp-ca.crt=/path/to/ca.crt --dry-run=client -o yaml |
  kubectl -n sealane patch configmap sealane-tls-certs --type merge --patch-file /dev/stdin
kubectl -n sealane rollout restart deploy/sealane
```

ConfigMap은 `kubectl apply`가 아니라 패치로 수정합니다. 패치나 `kubectl edit`로 추가한 키는 이후 `kubectl apply -f install.yaml`이나 `helm upgrade`를 해도 남습니다. 두 명령은 자신이 렌더링하는 키만 관리하기 때문입니다. Helm을 쓴다면 인증서를 렌더링 결과에 넣을 수도 있습니다.

```bash
helm upgrade sealane oci://ghcr.io/wonjune95/charts/sealane -n sealane --reset-then-reuse-values \
  --set-file 'tlsCerts.corp-ca\.crt=/path/to/ca.crt'
```

사용자는 IdP로 직접 이동하므로 브라우저도 같은 CA를 신뢰해야 합니다. 디스커버리는 첫 SSO 로그인 때 일어나므로 CA 누락도 그때 드러납니다. 로그인 페이지에는 제공자 오류가 표시되고, Sealane 로그에는 `x509: certificate signed by unknown authority`가 담긴 `oidc login` 경고가 남습니다. 1.15 이전에는 `SSL_CERT_FILE`과 `extraVolumes`로 이 작업을 했습니다. 이 방식도 여전히 동작합니다. 해당 파일을 디렉터리와 함께 추가로 읽기 때문입니다.

### 문제 해결

콜백이 실패할 때마다 `reason`(아래 코드)과 `err`(원인 오류)가 담긴 `oidc callback failed` 경고가 로그에 남습니다. IdP로 리다이렉트하기 전에 실패하면 대신 `oidc login`이 남습니다. 먼저 다음 명령으로 로그를 확인합니다.

```bash
kubectl -n sealane logs deploy/sealane | grep oidc
```

| `oidc_error` | 로그인 페이지 메시지 | 확인할 점 |
|---|---|---|
| `state` | 로그인 시도가 만료되었거나 변조되었습니다. | 로그인에 10분 넘게 걸렸거나, 브라우저가 로그인 쿠키를 버렸거나, 복제본마다 다른 키로 서명하는 경우입니다(위의 `sessionSecret` 참고). 로그인을 `redirectURL`에 적힌 호스트에서 시작하는지도 확인합니다. 다른 호스트에서 시작하면(예: `redirectURL`은 실제 호스트인데 `localhost` 포트 포워딩으로 접속한 경우) state 쿠키가 그 호스트에 설정되어 콜백으로 전달되지 않습니다. |
| `access_denied` | 이 계정에 배정된 역할이 없습니다. | 사용자가 `oidc.roles.*`에 나열된 어느 그룹에도 속하지 않고 `defaultRole`도 비어 있는 경우입니다. ID 토큰의 그룹 이름(정확한 문자열, Keycloak은 앞의 `/` 없이)과 `groupsClaim`을 확인합니다. |
| `idp` | 아이덴티티 제공자가 오류를 반환했습니다. | 로그의 `err`를 확인합니다. 흔한 원인은 다음과 같습니다. `redirectURL`이 IdP 클라이언트에 등록한 리다이렉트 URI와 **정확히** 일치하지 않음(scheme, 호스트, 포트, 경로). `issuerURL`이 제공자 디스커버리 문서의 `issuer`와 다름(끝의 슬래시 하나나 `http`/`https` 차이만으로도 달라짐). Sealane 파드가 발급자에 접근할 수 없음. 클라이언트 시크릿이 틀림. 알 수 없는 scope를 요청함. 사용자가 취소했거나 IdP가 로그인을 거부했다면 IdP의 `error`(예: IdP의 `access_denied`)가 `err`에 나타납니다. |

발급자를 비교하려면 클러스터 안에서 `<issuerURL>/.well-known/openid-configuration`을 조회합니다. 응답의 `issuer` 필드를 `oidc.issuerURL`과 한 글자씩 대조합니다.

**SSO 로그인 후 역할이 예상과 다른가요?** 서버는 로그인할 때마다 `groups`가 담긴 `login` 로그를 한 줄씩 남깁니다. 이 값이 IdP가 보낸 그룹 목록 그대로입니다. 이 목록을 `oidc.roles.*`와 비교합니다. `kubectl -n <ns> logs deploy/sealane | grep '"msg":"login"'`
