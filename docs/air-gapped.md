# Air-gapped install

Sealane fetches nothing from the internet at runtime: the UI is embedded in the binary and the only outgoing connections are to the Kubernetes API and, with SSO, your IdP. An air-gapped install needs two things from outside: the image and `install.yaml` (or the Helm chart).

## Outside: collect and verify

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

Verify before you carry anything in; keyless verification needs the internet:

```bash
ID='https://github.com/wonjune95/sealane/'; ISSUER=https://token.actions.githubusercontent.com
cosign verify-blob --bundle SHA256SUMS.sigstore.json --certificate-identity-regexp "$ID" \
  --certificate-oidc-issuer "$ISSUER" SHA256SUMS          # the checksums were made by this repository's release
sha256sum -c SHA256SUMS                                   # and the files match them
cosign verify --certificate-identity-regexp "$ID" --certificate-oidc-issuer "$ISSUER" ghcr.io/wonjune95/sealane:$V
```

`image-digest.txt` is in `SHA256SUMS`, so the signed checksums also pin the image you copied. Inside, `sha256sum -c SHA256SUMS` shows nothing changed on the way. To check the signature inside as well, carry Sigstore's trust root from the machine that verified (cosign keeps it under `~/.sigstore/root/`) and run `cosign verify-blob` with `--offline --trusted-root trusted_root.json` and the same identity flags. See [Verifying release signatures](security.md#verifying-release-signatures). Without skopeo, `docker pull` and `docker save` work too, but the digest changes when you push the image again inside.

To read these docs offline, build them from the repository with `use_directory_urls: false` and `theme.font: false`, so pages open from the file system and no web font is requested. Search needs the pages served over HTTP (`python3 -m http.server`).

## Inside: push and install

Every node's container runtime must trust your internal registry (its CA certificate, or an insecure-registry setting), or the pod stays in `ImagePullBackOff`. Check from one node before installing, for example `crictl pull $REG:$V` (containerd) or `podman pull $REG:$V`.

```bash
REG=registry.example.internal/sealane/sealane
DIGEST=sha256:...        # from skopeo inspect above
skopeo copy --all --preserve-digests oci-archive:sealane-$V.oci.tar:$V docker://$REG:$V
sed "s#ghcr.io/wonjune95/sealane:$V#$REG:$V@$DIGEST#" install.yaml > install-internal.yaml
kubectl apply -f install-internal.yaml
```

Pinning the digest makes the cluster pull exactly the image you verified, even if the tag in the internal registry is overwritten later.

Sign in as `admin` with the password Sealane generated at first start:

```bash
kubectl -n sealane get secret sealane-initial-admin -o jsonpath='{.data.password}' | base64 -d; echo
```

## Settings install.yaml does not carry

`install.yaml` is rendered with default values. Add what your environment needs after applying it:

| Need | How | Survives `kubectl apply -f install.yaml` on upgrade |
|---|---|---|
| Registry login | `kubectl -n sealane patch deploy sealane --type merge -p '{"spec":{"template":{"spec":{"imagePullSecrets":[{"name":"regcred"}]}}}}'` | Yes |
| Private CA for the IdP | Add the certificate to the `sealane-tls-certs` ConfigMap and restart (see [Private CA](authentication.md#private-ca)) | Yes |
| Two replicas | `kubectl -n sealane scale deploy sealane --replicas=2` (the PodDisruptionBudget keeps one up during drains) | No: apply resets it to 1 |
| Real request tester | Add `pods/exec` `create` to the `sealane` ClusterRole and `kubectl -n sealane set env deploy/sealane REQUEST_TESTER=true` | Env yes, ClusterRole rule no |
| Impersonation mode | `kubectl apply -f impersonation.yaml` (a release asset, carry it in too) after `install.yaml`, and `kubectl -n sealane set env deploy/sealane IMPERSONATION=true` (see [Impersonation mode](security.md#impersonation-mode)) | Env yes; apply `impersonation.yaml` again after `install.yaml` |
| Secure cookies behind a TLS proxy without `X-Forwarded-Proto` | `kubectl -n sealane set env deploy/sealane SECURE_COOKIES=true` | Yes |
| SSO | `kubectl -n sealane edit cm sealane-cm` and put the client secret in the `sealane-secret` Secret (see [Without Helm](authentication.md#without-helm-installyaml)) | Yes |

With Helm, the same settings are values (`imagePullSecrets`, `tlsCerts`, `replicaCount`, `requestTester.enabled`, `impersonation.enabled`, `secureCookies`, `oidc.*`), and `helm upgrade` keeps them.

To turn the two optional modes off again:

```bash
# request tester: drop the variable; applying install.yaml removes the pods/exec rule
kubectl -n sealane set env deploy/sealane REQUEST_TESTER-
kubectl apply -f install.yaml
# impersonation: give Sealane back its writing role first, then drop the mode and its RBAC
kubectl apply -f install.yaml
kubectl -n sealane set env deploy/sealane IMPERSONATION-
kubectl delete clusterrole,clusterrolebinding sealane-impersonator sealane-writer --ignore-not-found
```

Do not `kubectl delete -f impersonation.yaml`: it also carries Sealane's own ClusterRole.

## Upgrades

Carry in the new image and `install.yaml`, repeat the push and `sed` steps, and apply. Accounts, SSO settings, extra CA certificates and the session key live in ConfigMaps and Secrets and are kept. Redo the rows marked "No" in the table above, and apply `impersonation.yaml` again if you use that mode. Read [Upgrading](upgrading.md) for the versions you cross.
