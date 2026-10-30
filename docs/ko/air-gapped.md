# 폐쇄망 설치

> 영문 원문: [Air-gapped install](../air-gapped.md). 내용이 다르면 영문이 기준입니다.

Sealane은 실행 중에 인터넷에서 아무것도 가져오지 않습니다. UI는 바이너리에 내장되어 있고, 밖으로 나가는 연결은 Kubernetes API와 IdP(SSO를 쓰는 경우)뿐입니다. 폐쇄망 설치에 필요한 외부 반입물은 두 가지입니다. 이미지와 `install.yaml`(또는 Helm 차트)입니다.

## 외부망: 내려받고 검증하기

```bash
V=1.17.0
# Image with both architectures; an OCI archive keeps the digest when you push it later
skopeo copy --all --preserve-digests docker://ghcr.io/wonjune95/sealane:$V oci-archive:sealane-$V.oci.tar:$V
# Manifests (and, for Helm, the chart)
curl -LO https://github.com/wonjune95/sealane/releases/download/v$V/install.yaml
curl -LO https://github.com/wonjune95/sealane/releases/download/v$V/sealane-$V.tgz
curl -LO https://github.com/wonjune95/sealane/releases/download/v$V/sbom.spdx.json   # if your review asks for one
skopeo inspect --format '{{.Digest}}' oci-archive:sealane-$V.oci.tar:$V            # note the digest
```

반입하기 전에 서명을 검증하세요. 키리스 검증에는 인터넷이 필요합니다. [릴리스 서명 검증](../security.md#verifying-release-signatures)을 참고하세요. skopeo가 없으면 `docker pull`과 `docker save`로도 됩니다. 다만 내부에서 이미지를 다시 푸시할 때 다이제스트가 바뀝니다.

이 문서를 오프라인으로 보려면 저장소에서 `use_directory_urls: false`와 `theme.font: false`를 지정해 빌드하세요. 그러면 파일 시스템에서 바로 페이지를 열 수 있고 웹 폰트도 요청하지 않습니다. 검색은 페이지를 HTTP로 서빙해야 동작합니다(`python3 -m http.server`).

## 내부망: 푸시하고 설치하기

```bash
REG=registry.example.internal/sealane/sealane
DIGEST=sha256:...        # from skopeo inspect above
skopeo copy --all --preserve-digests oci-archive:sealane-$V.oci.tar:$V docker://$REG:$V
sed "s#ghcr.io/wonjune95/sealane:$V#$REG:$V@$DIGEST#" install.yaml > install-internal.yaml
kubectl apply -f install-internal.yaml
```

다이제스트를 고정하면 나중에 내부 레지스트리의 태그를 덮어쓰더라도 클러스터는 검증한 바로 그 이미지를 받습니다.

## install.yaml에 없는 설정

`install.yaml`은 기본값으로 렌더링되어 있습니다. 환경에 필요한 설정은 적용한 뒤에 추가하세요.

| 필요한 것 | 방법 | 업그레이드 때 `kubectl apply -f install.yaml` 후에도 유지 |
|---|---|---|
| 레지스트리 로그인 | `kubectl -n sealane patch deploy sealane --type merge -p '{"spec":{"template":{"spec":{"imagePullSecrets":[{"name":"regcred"}]}}}}'` | 예 |
| IdP용 사설 CA | `sealane-tls-certs` ConfigMap에 인증서를 추가하고 재시작([사설 CA](../authentication.md#private-ca) 참고) | 예 |
| 복제본 2개 | `kubectl -n sealane scale deploy sealane --replicas=2`(노드 드레인 중에는 PodDisruptionBudget이 하나를 계속 띄워 둠) | 아니요: apply하면 1로 돌아감 |
| 실제 요청 테스터 | `sealane` ClusterRole에 `pods/exec` `create`를 추가하고 `kubectl -n sealane set env deploy/sealane REQUEST_TESTER=true` 실행 | 환경 변수는 예, ClusterRole 규칙은 아니요 |
| 사용자 권한 모드(impersonation) | 릴리스에 첨부된 `impersonation.yaml`도 함께 반입해 `kubectl apply -f impersonation.yaml` 한 뒤 `kubectl -n sealane set env deploy/sealane IMPERSONATION=true` ([Impersonation mode](../security.md#impersonation-mode) 참고) | 예 |
| SSO | `kubectl -n sealane edit cm sealane-cm`으로 수정하고 client secret을 `sealane-secret` Secret에 넣음([Helm 없이](../authentication.md#without-helm-installyaml) 참고) | 예 |

Helm을 쓰면 같은 설정이 values(`imagePullSecrets`, `tlsCerts`, `replicaCount`, `requestTester.enabled`, `oidc.*`)로 들어가고, `helm upgrade`를 해도 유지됩니다.

## 업그레이드

새 이미지와 `install.yaml`을 반입하고, 푸시와 `sed` 단계를 반복한 뒤 적용합니다. 계정, SSO 설정, 추가 CA 인증서, 세션 키는 ConfigMap과 Secret에 있으므로 그대로 유지됩니다. 위 표에서 "아니요"로 표시된 항목은 다시 적용하세요.
