# 시작하기

> 영문 원문: [Getting started](../getting-started.md). 내용이 다르면 영문이 기준입니다.

기본 실행 방식은 클러스터 안에 배포하는 것입니다. 미리 빌드된 이미지를 설치할 때는 Helm(또는 `kubectl`)만 있으면 됩니다.

```bash
# Helm (OCI)
helm install sealane oci://ghcr.io/wonjune95/charts/sealane --version 1.17.1 -n sealane --create-namespace
# or plain manifests
kubectl apply -f https://github.com/wonjune95/sealane/releases/latest/download/install.yaml
kubectl -n sealane port-forward svc/sealane 8080:8080   # → http://localhost:8080, log in as admin / admin
```

!!! note "Istio가 없는 클러스터"
    `install.yaml`은 istiod 버전을 읽기 위해 `istio-system` 네임스페이스가 있다고 가정합니다. Istio가 없으면 `kubectl apply`에서 `namespaces "istio-system" not found` 오류가 두 번 나오지만 무시해도 됩니다. Helm 설치는 Istio 없이도 동작합니다. `istioNamespace` 네임스페이스(기본값 `istio-system`)가 없으면 차트가 istiod-reader Role을 만들지 않고, 홈 화면에 컨트롤 플레인 버전이 나오지 않습니다. GitOps 도구(Argo CD, Flux)는 `helm template`으로 렌더링하므로 네임스페이스가 없는지 알 수 없습니다. 이때는 `istioNamespace: ""`로 설정하세요.

아래 단계는 저장소를 클론해서 직접 빌드하고 설치하는 방법입니다. Docker와 Helm만 있으면 됩니다. 이미지 빌드가 멀티 스테이지라서 로컬에 Go나 Node가 없어도 됩니다.

## 1) 이미지

릴리스 태그(`v*`)마다 GitHub Actions가 이미지를 게시합니다. 이 이미지를 그대로 쓰면 됩니다.

```bash
# prebuilt image (recommended)
ghcr.io/wonjune95/sealane:1.17.1     # or :latest
```

직접 빌드할 수도 있습니다.

```bash
docker build -t <registry>/sealane:<tag> .
docker push <registry>/sealane:<tag>
```

## 2) Helm 설치

```bash
helm install sealane ./charts/sealane -n sealane --create-namespace \
  --set image.repository=<registry>/sealane --set image.tag=<tag>
```

게시된 이미지는 멀티 아키텍처(amd64/arm64)이며 cosign 키리스 서명이 되어 있습니다. 검증 명령은 [릴리스](../releasing.md)를 참고하세요.
주요 값은 다음과 같습니다. `image.*`, `imagePullSecrets`(사설 레지스트리), `replicaCount`(무상태이므로 HA가 필요하면 늘림), `accountsConfigMap`, `clustersSecret`, `requestTester.enabled`(실제 요청 테스터, 기본값은 꺼짐이며 켜면 `pods/exec` 권한이 부여됨), `sessionSecret.*`(고정 세션 서명 키), `ingress.*`, `httpRoute.*`(Ingress 또는 Gateway API로 노출), `oidc.*`(SSO, [인증](../authentication.md#single-sign-on-oidc) 참고). 전체 표는 [설정](../configuration.md#helm-values)에 있습니다. 파드는 distroless 이미지에서 non-root(uid 65532)로 실행되며 readOnlyRootFilesystem이 켜져 있습니다.

## 3) 노출

클러스터 환경에 맞는 방식으로 노출합니다.

```bash
# with Gateway API — adjust parentRefs/hostname first
kubectl apply -f examples/httproute.yaml

# with an Ingress controller — create an Ingress backed by service sealane:8080
```

둘 중 하나를 차트가 대신 만들게 할 수도 있습니다. `httpRoute.enabled=true`(`httpRoute.parentRefs` / `hostnames`와 함께) 또는 `ingress.enabled=true`(`ingress.className` / `hosts` / `tls`와 함께)를 설정하세요.

## 4) 로그인

처음 기동하면 초기 계정(**`admin` / `admin`**)이 만들어집니다. 로그인하면 곧바로 비밀번호 변경 화면이 나옵니다. 비밀번호를 바꾸기 전에는 다른 화면으로 넘어갈 수 없습니다.

![로그인](../img/screenshot-login.png)

## 5) 계정 추가

admin으로 로그인해 **설정 → 계정 관리**에서 계정을 추가하고 역할과 비밀번호를 바꿉니다. 자기 계정 삭제, 자기 역할 변경, `admin` 삭제는 막혀 있습니다. 계정은 ConfigMap에 저장되므로 kubectl로도 관리할 수 있습니다.

```bash
go run ./hack/bcrypt-hash.go 'password'                      # generate bcrypt hash
# (without Go) htpasswd -bnBC 10 "" 'password' | tr -d ':\n'
kubectl -n sealane edit configmap sealane-accounts
# add to data:  username: "role:hash"   (role: admin | editor | viewer)
```

재시작할 필요는 없습니다. 마운트된 볼륨이 동기화되면서 1분 안팎으로 반영됩니다. [`examples/accounts-configmap.yaml`](https://github.com/wonjune95/sealane/blob/main/examples/accounts-configmap.yaml)을 참고하세요.

로컬 계정 대신 조직의 IdP로 로그인하게 하려면 [SSO(OIDC)](../authentication.md#single-sign-on-oidc)를 참고하세요.

## 6) 클러스터 추가 등록(선택 사항)

Sealane 하나로 여러 클러스터를 관리하려면 admin으로 **설정 → 클러스터 관리**에 들어가 kubeconfig를 붙여 넣거나 파일로 불러온 뒤, 추가할 클러스터의 컨텍스트를 고릅니다. 평소 쓰는 `~/.kube/config`를 그대로 넣어도 됩니다. 기본으로 Sealane이 그 자격 증명을 한 번만 써서 대상 클러스터에 권한이 제한된 `sealane-remote` 계정을 만들고, 그 계정의 토큰만 Sealane 옆의 `sealane-clusters` Secret에 저장합니다. 등록할 때 연결 테스트를 실행하고, 헤더의 클러스터 메뉴에서 클러스터를 전환합니다. [멀티 클러스터](../multi-cluster.md)를 참고하세요.

![설정 — 계정과 클러스터](../img/screenshot-settings.png)

## 설치 후 할 일

① admin 비밀번호 변경(첫 로그인 때 강제) ② 계정 추가(설정 → 계정 관리) 또는 SSO 켜기 ③ 멀티 클러스터라면 kubeconfig 등록(설정 → 클러스터 관리).

삭제는 `helm uninstall sealane -n sealane`으로 합니다. 계정 ConfigMap과 클러스터 Secret은 남으므로, 완전히 정리하려면 이 둘도 삭제하세요.
