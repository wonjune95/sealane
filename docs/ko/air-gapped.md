# 폐쇄망 설치

> 영문 원문: [Air-gapped install](../air-gapped.md). 내용이 다르면 영문이 기준입니다.

Sealane은 실행 중에 인터넷에서 아무것도 가져오지 않습니다. UI는 바이너리에 내장되어 있고, 밖으로 나가는 연결은 Kubernetes API와 IdP(SSO를 쓰는 경우)뿐입니다. 폐쇄망 설치에 필요한 외부 반입물은 두 가지입니다. 이미지와 `install.yaml`(또는 Helm 차트)입니다.

## 외부망: 내려받고 검증하기

```bash
V=1.19.0
# Image with both architectures; an OCI archive keeps the digest when you push it later
skopeo copy --all --preserve-digests docker://ghcr.io/wonjune95/sealane:$V oci-archive:sealane-$V.oci.tar:$V
# Manifests, the chart, license notices and the signed checksums of all of them
for f in install.yaml impersonation.yaml sealane-$V.tgz sbom.spdx.json THIRD_PARTY_NOTICES \
         image-digest.txt SHA256SUMS SHA256SUMS.sigstore.json; do
  curl -fLO https://github.com/wonjune95/sealane/releases/download/v$V/$f
done
skopeo inspect --format '{{.Digest}}' oci-archive:sealane-$V.oci.tar:$V            # must match image-digest.txt
```

반입하기 전에 검증하세요. 키리스 검증에는 인터넷이 필요합니다.

```bash
ID='https://github.com/wonjune95/sealane/'; ISSUER=https://token.actions.githubusercontent.com
cosign verify-blob --bundle SHA256SUMS.sigstore.json --certificate-identity-regexp "$ID" \
  --certificate-oidc-issuer "$ISSUER" SHA256SUMS          # the checksums were made by this repository's release
sha256sum -c SHA256SUMS                                   # and the files match them
cosign verify --certificate-identity-regexp "$ID" --certificate-oidc-issuer "$ISSUER" ghcr.io/wonjune95/sealane:$V
```

`image-digest.txt`도 `SHA256SUMS`에 들어 있으므로, 서명된 체크섬이 복사한 이미지까지 고정합니다. 내부망에서는 `sha256sum -c SHA256SUMS`로 반입 중에 바뀐 것이 없는지 확인합니다. 내부에서도 서명을 확인하려면 cosign 3.x 실행 파일과, 검증한 PC의 Sigstore 신뢰 루트(cosign이 `~/.sigstore/root/tuf-repo-cdn.sigstore.dev/targets/trusted_root.json`에 보관)를 함께 반입해 실행하세요.

```bash
cosign verify-blob --trusted-root trusted_root.json --bundle SHA256SUMS.sigstore.json \
  --certificate-identity-regexp "$ID" --certificate-oidc-issuer "$ISSUER" SHA256SUMS
``` [릴리스 서명 검증](../security.md#verifying-release-signatures)을 참고하세요. skopeo가 없으면 `docker pull`과 `docker save`로도 됩니다. 다만 내부에서 이미지를 다시 푸시할 때 다이제스트가 바뀝니다.

이 문서를 오프라인으로 보려면 저장소에서 `use_directory_urls: false`와 `theme.font: false`를 지정해 빌드하세요. 그러면 파일 시스템에서 바로 페이지를 열 수 있고 웹 폰트도 요청하지 않습니다. 검색은 페이지를 HTTP로 서빙해야 동작합니다(`python3 -m http.server`).

## 내부망: 푸시하고 설치하기

모든 노드의 컨테이너 런타임이 내부 레지스트리를 신뢰해야 합니다(레지스트리 CA 인증서 또는 insecure-registry 설정). 그렇지 않으면 파드가 `ImagePullBackOff`에 머뭅니다. 설치 전에 노드 하나에서 확인하세요. 예: `crictl pull $REG:$V`(containerd), `podman pull $REG:$V`.

```bash
REG=registry.example.internal/sealane/sealane
DIGEST=sha256:...        # from skopeo inspect above
skopeo copy --all --preserve-digests oci-archive:sealane-$V.oci.tar:$V docker://$REG:$V
sed "s#ghcr.io/wonjune95/sealane:$V#$REG:$V@$DIGEST#" install.yaml > install-internal.yaml
kubectl apply -f install-internal.yaml
```

다이제스트를 고정하면 나중에 내부 레지스트리의 태그를 덮어쓰더라도 클러스터는 검증한 바로 그 이미지를 받습니다.

`admin`으로 로그인합니다. 비밀번호는 Sealane이 처음 시작할 때 만든 값입니다.

```bash
kubectl -n sealane get secret sealane-initial-admin -o jsonpath='{.data.password}' | base64 -d; echo
```

## install.yaml에 없는 설정

`install.yaml`은 기본값으로 렌더링되어 있습니다. 환경에 필요한 설정은 적용한 뒤에 추가하세요.

| 필요한 것 | 방법 | 업그레이드 때 `kubectl apply -f install.yaml` 후에도 유지 |
|---|---|---|
| 레지스트리 로그인 | `kubectl -n sealane patch deploy sealane --type merge -p '{"spec":{"template":{"spec":{"imagePullSecrets":[{"name":"regcred"}]}}}}'` | 예 |
| IdP용 사설 CA | `sealane-tls-certs` ConfigMap에 인증서를 추가하고 재시작([사설 CA](../authentication.md#private-ca) 참고) | 예 |
| 복제본 2개 | `kubectl -n sealane scale deploy sealane --replicas=2`(노드 드레인 중에는 PodDisruptionBudget이 하나를 계속 띄워 둠) | 아니요: apply하면 1로 돌아감 |
| 실제 요청 테스터 | `sealane` ClusterRole에 `pods/exec` `create`를 추가하고 `kubectl -n sealane set env deploy/sealane REQUEST_TESTER=true` 실행 | 환경 변수는 예, ClusterRole 규칙은 아니요 |
| 사용자 권한 모드(impersonation) | 릴리스에 첨부된 `impersonation.yaml`도 함께 반입해 `install.yaml` 다음에 `kubectl apply -f impersonation.yaml` 한 뒤 `kubectl -n sealane set env deploy/sealane IMPERSONATION=true` ([Impersonation mode](../security.md#impersonation-mode) 참고) | 환경 변수는 예, `impersonation.yaml`은 `install.yaml` 다음에 다시 적용 |
| TLS 프록시가 `X-Forwarded-Proto`를 보내지 않을 때 Secure 쿠키 | `kubectl -n sealane set env deploy/sealane SECURE_COOKIES=true` | 예 |
| SSO | `kubectl -n sealane edit cm sealane-cm`으로 수정하고 client secret을 `sealane-secret` Secret에 넣음([Helm 없이](../authentication.md#without-helm-installyaml) 참고) | 예 |

Helm을 쓰면 같은 설정이 values(`imagePullSecrets`, `tlsCerts`, `replicaCount`, `requestTester.enabled`, `impersonation.enabled`, `secureCookies`, `oidc.*`)로 들어가고, `helm upgrade`를 해도 유지됩니다.

두 선택 모드를 다시 끄려면:

```bash
# request tester: drop the variable; applying install.yaml removes the pods/exec rule
kubectl -n sealane set env deploy/sealane REQUEST_TESTER-
kubectl apply -f install.yaml
# impersonation: give Sealane back its writing role first, then drop the mode and its RBAC
kubectl apply -f install.yaml
kubectl -n sealane set env deploy/sealane IMPERSONATION-
kubectl delete clusterrole,clusterrolebinding sealane-impersonator sealane-writer --ignore-not-found
```

`kubectl delete -f impersonation.yaml`은 쓰지 마세요. Sealane 자체의 ClusterRole도 들어 있습니다.

## 업그레이드

새 이미지와 `install.yaml`을 반입하고, 푸시와 `sed` 단계를 반복한 뒤 적용합니다. 계정, SSO 설정, 추가 CA 인증서, 세션 키는 ConfigMap과 Secret에 있으므로 그대로 유지됩니다. 위 표에서 "아니요"로 표시된 항목은 다시 적용하고, 사용자 권한 모드를 쓴다면 `impersonation.yaml`도 다시 적용하세요. 거쳐 가는 버전의 [업그레이드](../upgrading.md) 안내도 읽어 보세요.
