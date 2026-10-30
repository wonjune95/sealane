# 자주 묻는 질문

> 영문 원문: [FAQ](../faq.md). 내용이 다르면 영문이 기준입니다.

## Kiali와 무엇이 다른가요?

Kiali는 메시를 관측하는 도구입니다. 텔레메트리 기반 토폴로지, 메트릭, 트레이스, 상태를 보여 줍니다. Sealane은 **라우팅 설정을 안전하게 편집하는** 도구입니다. Istio와 Gateway API 리소스 모두에 대해 스키마 기반 폼, dry-run 검증, diff 미리보기, 충돌 감지, 역할 기반 계정을 제공합니다. [트래픽 흐름](../features.md#traffic-flow-map)은 설정만으로 그립니다. 그래서 메트릭 스택 없이도 트래픽이 *가야 할* 곳과 라우트가 끊긴 곳을 보여 줍니다. 두 도구는 서로를 보완합니다.

## Sealane이 CRD나 오퍼레이터를 설치하나요?

아니요. 클러스터에 이미 있는 Istio와 Gateway API CRD를 읽고, 찾은 리소스 종류만 보여 줍니다. 차트가 설치하는 것은 Deployment, Service, ServiceAccount, 관리 대상 리소스 종류에 대한 ClusterRole과 ClusterRoleBinding, 네임스페이스 범위의 Role과 RoleBinding입니다. 필요하면 세션 Secret, Ingress/HTTPRoute, ServiceMonitor도 설치합니다.

## 왜 모든 클러스터 호출을 ServiceAccount 하나로 실행하나요?

처음에는 사용자마다 Kubernetes 토큰을 그대로 전달하는 방식을 시도했습니다. 권한은 정확했지만 사용자별로 토큰을 발급하고 전달하고 갱신하는 일이 번거로웠고, 로그인 경험도 나빴습니다. 지금은 Sealane이 자체 역할(로컬 계정 또는 SSO 그룹)로 사용자를 인가하고, 클러스터와는 자신의 ServiceAccount로 통신합니다. 그래서 Kubernetes 감사 로그에는 ServiceAccount가 남고, 사용자는 Sealane의 변경 기록과 감사 로그에 남습니다. 역할별로 무엇을 바꿀 수 있는지 클러스터 RBAC가 정하게 하거나, Kubernetes 감사 로그에 사람 이름이 남아야 한다면 [사용자 권한 모드(impersonation)](../security.md#impersonation-mode)를 켜세요. 그러면 쓰기가 `sealane:admin` 또는 `sealane:editor`로 실행됩니다. [아키텍처와 보안 모델](../architecture.md#why-app-local-accounts-instead-of-token-passthrough)도 참고하세요.

## 폐쇄망 클러스터에서도 동작하나요?

네. 프런트엔드가 바이너리에 내장되어 있어 실행 중에 인터넷에서 가져오는 것이 없습니다. SSO를 쓰는 경우 IdP는 브라우저와 Sealane 파드에서만 접근할 수 있으면 됩니다. 이미지를 미러링하고, `install.yaml`에서 다이제스트로 고정한 뒤(Helm은 `image.repository` 설정), `install.yaml`에 없는 몇 가지 설정을 추가하면 됩니다. 단계별 절차는 [폐쇄망 설치](air-gapped.md)에 있습니다.

## 클러스터에 Gateway API만(또는 Istio만) 있어도 되나요?

네. Sealane은 CRD가 있는 리소스 종류만 보여 주고 나머지는 숨깁니다. Istio가 없는 클러스터라면 [시작하기](getting-started.md)의 `istioNamespace` 안내를 참고하세요. 이 경우 홈 화면에 컨트롤 플레인 버전이 나오지 않습니다. Gateway API와 Istio CRD가 모두 없으면 모든 페이지가 설치 안내로 연결됩니다.

## admin 비밀번호를 잊어버렸습니다. 어떻게 다시 로그인하나요?

계정 ConfigMap을 직접 수정하세요. 재시작은 필요 없습니다.

```bash
go run ./hack/bcrypt-hash.go 'new-password'
kubectl -n sealane edit configmap sealane-accounts   # set  admin: "admin:<new-hash>"
```

변경은 1분 안팎으로 반영됩니다. `admin` 키를 지우고 파드를 재시작하는 방법도 있습니다. 시작할 때 ConfigMap에 `admin` 키가 없으면 Sealane이 `admin` / `admin` 계정을 다시 만들고, 첫 로그인 때 비밀번호 변경을 강제합니다.

## 재시작하면 왜 다시 로그인해야 하나요?

세션은 서명된 쿠키이므로 서명 키가 파드보다 오래 유지되어야 합니다. Helm 차트는 키를 `<release>-session` Secret에 보관합니다(`sessionSecret.create=true`). `install.yaml`과 GitOps 설치에서는 Sealane이 처음 시작할 때 키를 생성해 `sealane-secret` Secret에 넣습니다. 재시작 후 로그아웃되는 경우는 두 가지뿐입니다. 파드가 그 Secret을 읽거나 만들 수 없을 때(로그에 `session secret: using a per-boot key`가 남습니다), 그리고 Deployment에서 `SESSION_SECRET_NAME`이 제거됐을 때입니다. [운영](../operations.md#high-availability)을 참고하세요.

## ARM에서도 동작하나요?

네. 이미지는 멀티 아키텍처(linux/amd64, linux/arm64)입니다. CI의 end-to-end 테스트는 amd64에서 실행합니다. [호환성](../compatibility.md)을 참고하세요.
