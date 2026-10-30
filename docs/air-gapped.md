# Air-gapped install

Sealane fetches nothing from the internet at runtime: the UI is embedded in the binary and the only outgoing connections are to the Kubernetes API and, with SSO, your IdP. An air-gapped install needs two things from outside: the image and `install.yaml` (or the Helm chart).

## Outside: collect and verify

```bash
V=1.17.1
# Image with both architectures; an OCI archive keeps the digest when you push it later
skopeo copy --all --preserve-digests docker://ghcr.io/wonjune95/sealane:$V oci-archive:sealane-$V.oci.tar:$V
# Manifests (and, for Helm, the chart)
curl -LO https://github.com/wonjune95/sealane/releases/download/v$V/install.yaml
curl -LO https://github.com/wonjune95/sealane/releases/download/v$V/sealane-$V.tgz
curl -LO https://github.com/wonjune95/sealane/releases/download/v$V/sbom.spdx.json   # if your review asks for one
skopeo inspect --format '{{.Digest}}' oci-archive:sealane-$V.oci.tar:$V            # note the digest
```

Verify the signatures before you carry anything in; keyless verification needs the internet. See [Verifying release signatures](security.md#verifying-release-signatures). Without skopeo, `docker pull` and `docker save` work too, but the digest changes when you push the image again inside.

To read these docs offline, build them from the repository with `use_directory_urls: false` and `theme.font: false`, so pages open from the file system and no web font is requested. Search needs the pages served over HTTP (`python3 -m http.server`).

## Inside: push and install

```bash
REG=registry.example.internal/sealane/sealane
DIGEST=sha256:...        # from skopeo inspect above
skopeo copy --all --preserve-digests oci-archive:sealane-$V.oci.tar:$V docker://$REG:$V
sed "s#ghcr.io/wonjune95/sealane:$V#$REG:$V@$DIGEST#" install.yaml > install-internal.yaml
kubectl apply -f install-internal.yaml
```

Pinning the digest makes the cluster pull exactly the image you verified, even if the tag in the internal registry is overwritten later.

## Settings install.yaml does not carry

`install.yaml` is rendered with default values. Add what your environment needs after applying it:

| Need | How | Survives `kubectl apply -f install.yaml` on upgrade |
|---|---|---|
| Registry login | `kubectl -n sealane patch deploy sealane --type merge -p '{"spec":{"template":{"spec":{"imagePullSecrets":[{"name":"regcred"}]}}}}'` | Yes |
| Private CA for the IdP | Add the certificate to the `sealane-tls-certs` ConfigMap and restart (see [Private CA](authentication.md#private-ca)) | Yes |
| Two replicas | `kubectl -n sealane scale deploy sealane --replicas=2` (the PodDisruptionBudget keeps one up during drains) | No: apply resets it to 1 |
| Real request tester | Add `pods/exec` `create` to the `sealane` ClusterRole and `kubectl -n sealane set env deploy/sealane REQUEST_TESTER=true` | Env yes, ClusterRole rule no |
| Impersonation mode | `kubectl apply -f impersonation.yaml` (a release asset, carry it in too) and `kubectl -n sealane set env deploy/sealane IMPERSONATION=true` (see [Impersonation mode](security.md#impersonation-mode)) | Yes |
| SSO | `kubectl -n sealane edit cm sealane-cm` and put the client secret in the `sealane-secret` Secret (see [Without Helm](authentication.md#without-helm-installyaml)) | Yes |

With Helm, the same settings are values (`imagePullSecrets`, `tlsCerts`, `replicaCount`, `requestTester.enabled`, `oidc.*`), and `helm upgrade` keeps them.

## Upgrades

Carry in the new image and `install.yaml`, repeat the push and `sed` steps, and apply. Accounts, SSO settings, extra CA certificates and the session key live in ConfigMaps and Secrets and are kept. Redo the rows marked "No" in the table above.
