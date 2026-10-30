# 운영

> 영문 원문: [Operations](../operations.md). 내용이 다르면 영문이 기준입니다.

## 고가용성

Sealane은 자체 상태를 저장하지 않습니다. 계정은 계정 ConfigMap에, 원격 클러스터는 클러스터 Secret에, 세션은 서명된 쿠키에 있습니다. 따라서 복제본끼리 서로 대체할 수 있고, `replicaCount`로 확장합니다. 복제본 2개와 종료 지연(`SHUTDOWN_DELAY`, 기본값 `5s`)이 있으면 롤링 업데이트 중에도 요청이 끊기지 않습니다. 이전 파드는 Service에서 빠진 뒤에도 5초 동안 요청을 처리합니다. HA가 필요하면 `replicaCount: 2`로 설정하세요.

모든 복제본은 **같은 키**로 세션에 서명해야 합니다. 그렇지 않으면 한 파드가 발급한 쿠키를 다른 파드가 거부합니다. SSO 로그인 후 다른 파드로 돌아오면 `oidc_error=state`로 실패하는 것도 같은 이유입니다.

- **Helm** — `sessionSecret.create=true`(기본값)이면 임의 키를 한 번 생성해 `<release>-session` Secret에 넣고, 업그레이드해도 그대로 둡니다. 그래서 재시작해도 세션이 유지됩니다.
- **`install.yaml`, GitOps(Argo CD, Flux), `helm template`** — 이 방식에는 `lookup`이 없어 키가 렌더링되지 않습니다. 대신 Sealane이 처음 시작할 때 키를 생성해 `sealane-secret` Secret(키 `SESSION_SECRET`)에 저장합니다. Argo CD의 `argocd-secret`과 같은 방식입니다. 모든 파드가 이 Secret을 읽으므로 재시작해도, 복제본이 여러 개여도 같은 키를 씁니다. 직접 만든 키를 쓰려면 `sessionSecret.existingSecret`(키 `session-secret`)을 설정하세요.

변경 기록은 Sealane 네임스페이스의 ConfigMap `sealane-history`와 Secret `sealane-history-objects`(Helm이면 `<release>-history…`)에 저장됩니다. 그래서 복제본이 같은 기록을 공유하고, 재시작해도 남습니다. 목록은 최근 200건을 담고, 객체 Secret은 변경 전후 버전을 약 768 KiB까지 보관하며 오래된 것부터 지웁니다. 그래서 오래된 항목은 복원할 내용이 사라질 수 있습니다. 1.15~1.18은 객체를 같은 이름의 ConfigMap에 두었습니다. 1.19는 시작할 때 이 객체를 Secret으로 옮기고 ConfigMap을 지웁니다. 어느 것을 지워도 기록만 사라집니다. 로그 시스템으로 보낼 기록은 여전히 아래의 구조화된 감사 로그입니다.

## 메트릭

`/metrics`는 Prometheus 메트릭을 제공합니다. `http_requests_total`(`method`, `status`별), `http_request_duration_seconds`(`method`별), `sealane_build_info`(`version`별, 항상 1), `sealane_history_write_failures_total`, `sealane_login_failures_total`(`reason`별), `sealane_account_read_errors_total`, 표준 Go 수집기와 프로세스 수집기가 포함됩니다. Prometheus Operator를 쓴다면 `serviceMonitor.enabled=true`로 설정하세요. 차트가 `http` 포트의 `/metrics`를 수집하는 `ServiceMonitor`를 만듭니다.

처음 걸어 둘 만한 알림은 다음과 같습니다.

```yaml
groups:
  - name: sealane
    rules:
      - alert: SealaneDown
        expr: absent(up{job="sealane"} == 1)
        for: 5m
      - alert: SealaneServerErrors          # the API server or a remote cluster is failing Sealane's calls
        expr: sum(rate(http_requests_total{job="sealane",status=~"5.."}[5m])) > 0.1
        for: 10m
      - alert: SealaneHistoryNotSaved       # changes go through, but their history is lost (RBAC, size, API errors)
        expr: increase(sealane_history_write_failures_total{job="sealane"}[15m]) > 0
      - alert: SealaneSignInFailures        # someone guessing passwords, or a broken SSO setup
        expr: sum(increase(sealane_login_failures_total{job="sealane"}[10m])) > 20
      - alert: SealaneCannotReadAccounts    # account changes then reach Sealane only through the mounted copy
        expr: increase(sealane_account_read_errors_total{job="sealane"}[10m]) > 5
```

`job`은 수집 설정에서 붙인 이름으로 바꾸세요. 변경 기록 저장에 실패하면 로그 메시지 `change history not saved`에 원인이 나옵니다.

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

## 백업과 복원

라우팅 리소스는 Sealane이 아니라 각 클러스터에 있습니다. Sealane이 보관하는 것은 모두 자기 네임스페이스에 있습니다. 아래 이름은 릴리스 이름이 `sealane`일 때이며, Helm에서는 `<release>-…` 형식을 따릅니다.

| 객체 | 담긴 것 | 없으면 |
|---|---|---|
| ConfigMap `sealane-accounts` | 로컬 계정, 역할, bcrypt 해시 | 로컬 로그인이 불가능하고, 새 비밀번호로 `admin`이 새로 만들어짐 |
| Secret `sealane-clusters` | 원격 클러스터 kubeconfig(자격 증명) | 원격 클러스터를 다시 등록해야 함 |
| ConfigMap `sealane-cm` | `OIDC_*` 등 설정(`install.yaml`, Helm은 values로 렌더링) | SSO가 꺼짐 |
| Secret `sealane-secret` | 생성된 세션 키, 여기에 넣었다면 `OIDC_CLIENT_SECRET` | 모두 다시 로그인해야 하고, client secret이 없으면 SSO가 실패함 |
| Secret `sealane-session` | Helm `sessionSecret.create`의 세션 키 | 모두 다시 로그인해야 함 |
| ConfigMap `sealane-tls-certs` | 사설 CA 인증서 | 사설 인증서를 쓰는 IdP로 SSO가 실패함 |
| ConfigMap `sealane-history`, Secret `sealane-history-objects` | 변경 기록 | 기록이 비어서 시작함 |
| ConfigMap `sealane-history-objects` | 1.15~1.18이 기록한 항목의 객체(1.19가 Secret으로 옮기기 전까지) | 그 항목을 비교하거나 복원할 수 없음 |
| Secret `sealane-initial-admin` | 처음 만든 admin 비밀번호 | 한 번도 바꾸지 않았다면 admin 비밀번호를 아무도 모름 |

직접 이름을 정한 Secret은 목록에 없습니다. `sessionSecret.existingSecret`과 `oidc.clientSecretRef.name`은 다른 자격 증명과 함께 백업하세요. Helm이라면 values도 보관하세요: `helm get values sealane -n sealane -o yaml > sealane-values.yaml`.

백업:

```bash
kubectl -n sealane get configmap sealane-accounts sealane-cm sealane-tls-certs sealane-history sealane-history-objects \
  --ignore-not-found -o yaml > sealane-configmaps.yaml
kubectl -n sealane get secret sealane-clusters sealane-secret sealane-session sealane-history-objects sealane-initial-admin \
  --ignore-not-found -o yaml > sealane-secrets.yaml   # credentials: store it encrypted
```

새 클러스터로 복원할 때는 Sealane을 설치하기 **전에** 넣어야, 처음 시작할 때 Sealane이 이 객체들을 찾습니다.

```bash
kubectl create namespace sealane
# drop the old object identity so the objects can be created again
grep -vE '^    (resourceVersion|uid|creationTimestamp):' sealane-configmaps.yaml | kubectl create -f -
grep -vE '^    (resourceVersion|uid|creationTimestamp):' sealane-secrets.yaml | kubectl create -f -
# then install as usual: helm install ... -f sealane-values.yaml, or kubectl apply -f install.yaml
```

Helm으로 설치했던 객체에는 Helm 소유 주석이 있어서, 같은 릴리스 이름과 네임스페이스로 `helm install`하면 그대로 넘겨받습니다. 복원 뒤에는 기존 계정으로 로그인하고, **설정**에서 등록된 클러스터가 *연결됨*으로 나오는지, 리소스의 **변경 기록** 탭에서 이전 항목을 현재 객체와 비교할 수 있는지 확인하세요.

실행 중인 설치에 되돌려 넣을 때(실수로 고친 것을 되돌릴 때)는 같은 세 필드를 지운 파일을 `kubectl apply -f`로 적용합니다. 계정은 몇 초 안에, 클러스터는 다음 요청부터 반영됩니다.

## 삭제

`helm uninstall sealane -n sealane`(또는 `kubectl delete -f install.yaml`)으로 Sealane을 삭제합니다. Sealane이 실행 중에 만든 것은 남습니다. 계정 ConfigMap, 클러스터·세션·초기 관리자·변경 기록 Secret, 변경 기록 ConfigMap입니다. Sealane은 전용 네임스페이스에 있으므로 `kubectl delete namespace sealane`으로 모두 지울 수 있습니다. 전용 계정으로 등록한 원격 클러스터에는 그 계정이 남으므로, 먼저 각 등록을 계정 삭제 옵션과 함께 지우세요([멀티 클러스터](../multi-cluster.md) 참고).
