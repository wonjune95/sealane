# 운영

> 영문 원문: [Operations](../operations.md). 내용이 다르면 영문이 기준입니다.

## 고가용성

Sealane은 자체 상태를 저장하지 않습니다. 계정은 계정 ConfigMap에, 원격 클러스터는 클러스터 Secret에, 세션은 서명된 쿠키에 있습니다. 따라서 복제본끼리 서로 대체할 수 있고, `replicaCount`로 확장합니다. 복제본 2개와 종료 지연(`SHUTDOWN_DELAY`, 기본값 `5s`)이 있으면 롤링 업데이트 중에도 요청이 끊기지 않습니다. 이전 파드는 Service에서 빠진 뒤에도 5초 동안 요청을 처리합니다. HA가 필요하면 `replicaCount: 2`로 설정하세요.

모든 복제본은 **같은 키**로 세션에 서명해야 합니다. 그렇지 않으면 한 파드가 발급한 쿠키를 다른 파드가 거부합니다. SSO 로그인 후 다른 파드로 돌아오면 `oidc_error=state`로 실패하는 것도 같은 이유입니다.

- **Helm** — `sessionSecret.create=true`(기본값)이면 임의 키를 한 번 생성해 `<release>-session` Secret에 넣고, 업그레이드해도 그대로 둡니다. 그래서 재시작해도 세션이 유지됩니다.
- **`install.yaml`, GitOps(Argo CD, Flux), `helm template`** — 이 방식에는 `lookup`이 없어 키가 렌더링되지 않습니다. 대신 Sealane이 처음 시작할 때 키를 생성해 `sealane-secret` Secret(키 `SESSION_SECRET`)에 저장합니다. Argo CD의 `argocd-secret`과 같은 방식입니다. 모든 파드가 이 Secret을 읽으므로 재시작해도, 복제본이 여러 개여도 같은 키를 씁니다. 직접 만든 키를 쓰려면 `sessionSecret.existingSecret`(키 `session-secret`)을 설정하세요.

홈 화면의 변경 기록은 파드별 메모리에 저장되며(최근 200건) 재시작하면 초기화됩니다. 복제본이 여러 개면 각 파드는 자신이 처리한 변경만 보여 줍니다. 영구 기록은 아래의 구조화된 감사 로그입니다.

## 메트릭

`/metrics`는 Prometheus 메트릭을 제공합니다. `http_requests_total`(`method`, `status`별), `http_request_duration_seconds`(`method`별), 표준 Go 수집기와 프로세스 수집기가 포함됩니다. Prometheus Operator를 쓴다면 `serviceMonitor.enabled=true`로 설정하세요. 차트가 `http` 포트의 `/metrics`를 수집하는 `ServiceMonitor`를 만듭니다.

프로브는 `/healthz`(liveness)와 `/readyz`(readiness)입니다. `/readyz`는 Kubernetes API 서버에 2초 타임아웃으로 요청을 보내고, 실패하면 503 `not ready`를 반환합니다. 차트가 두 프로브를 모두 설정합니다.

## 로그

로그는 stdout에 JSON 한 줄씩 출력됩니다(`slog`). 눈여겨볼 메시지는 다음과 같습니다.

| `msg` | 시점 | 필드 |
|---|---|---|
| `request` | 모든 HTTP 요청 | `method`, `path`, `status`, `dur_ms` |
| `resource mutated` | Sealane을 통한 생성/수정/삭제 성공 | `audit=true`, `dangerous`, `cluster`, `user`, `verb`, `type`, `namespace`, `name` |
| `HIGH-RISK resource mutated` (warn) | 위와 같고, 고위험 리소스 종류인 경우 | 위와 같음, `dangerous=true` |
| `resource mutation failed` (error) | API 서버가 쓰기를 거부함 | 위와 같음, `err` 추가 |
| `login` | 로컬 또는 SSO 로그인 성공 | `user`, `role`(SSO는 `provider`도) |
| `login failed` (warn) | 로컬 사용자 이름 또는 비밀번호가 틀림 | `user` |
| `oidc callback failed` (warn) | SSO 로그인 실패 | `reason`(`state`, `access_denied`, `idp`), `err` |

평소 쓰는 로그 수집기로 보내고, `dangerous=true`에 알림을 걸어 두세요. dry-run은 변경으로 기록하지 않습니다.

## 업그레이드

새 `--version`으로 `helm upgrade`를 실행합니다. 릴리스별 절차(새 values 키, 이름이 바뀐 리소스)는 [업그레이드](../upgrading.md)에 있습니다. 마이너 버전을 넘어갈 때는 먼저 읽어 보세요. 업그레이드는 기존 계정이나 클러스터 Secret을 건드리지 않습니다. 차트는 계정 ConfigMap을 data 없이 렌더링하고 삭제할 때도 남겨 두며, 클러스터 Secret은 Helm이 관리하지 않습니다.

## 백업

보관할 가치가 있는 것은 모두 두 객체에 있습니다. 둘 다 Sealane 네임스페이스에 있습니다.

```bash
kubectl -n sealane get configmap sealane-accounts -o yaml > sealane-accounts.yaml   # local accounts and roles
kubectl -n sealane get secret sealane-clusters -o yaml > sealane-clusters.yaml      # remote cluster kubeconfigs (credentials — store it like one)
```

세션 Secret(`<release>-session`)은 로그인 상태를 유지하는 데만 쓰입니다. 잃어버리면 모두 로그아웃될 뿐 다른 영향은 없습니다. 라우팅 리소스 자체는 Sealane이 아니라 각 클러스터에 있습니다. SSO를 쓴다면 IdP 클라이언트와 client secret이 든 Secret은 IdP 설정의 일부로 함께 관리하세요.

## 삭제

`helm uninstall sealane -n sealane`으로 릴리스를 삭제합니다. 계정 ConfigMap과 클러스터 Secret은 남으므로, 완전히 정리하려면 이 둘도 삭제하세요.
